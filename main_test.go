package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"github.com/connectome-neuprint/neuPrintHTTP/internal/version"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

type mainRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mainRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type mainDSGFixture struct {
	userCalls         int
	authorizeCalls    int
	anonymousDecision map[string]string
}

func (f *mainDSGFixture) client() *http.Client {
	return &http.Client{Transport: mainRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/dsg/v1/user":
			f.userCalls++
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			identity := map[string]interface{}{"id": 1, "email": token + "@example.com", "name": token}
			if token == "global" {
				identity["admin"] = true
			}
			return mainJSONResponse(http.StatusOK, identity), nil
		case "/api/dsg/v1/authorize":
			f.authorizeCalls++
			var request struct {
				Entries []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"entries"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, err
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			entries := make([]map[string]interface{}, 0, len(request.Entries))
			for _, entry := range request.Entries {
				decision := "deny"
				roles := []string{}
				if token == "" && f.anonymousDecision[entry.Name] != "" {
					decision = f.anonymousDecision[entry.Name]
					if decision == "allow" {
						roles = []string{"view"}
					}
				}
				if token == "reader" || token == "writer" {
					decision = "allow"
					roles = []string{"view"}
				}
				if token == "writer" {
					roles = []string{"admin"}
				}
				result := map[string]interface{}{
					"name": entry.Name, "version": entry.Version, "decision": decision, "roles": roles,
				}
				if decision == "tos_required" {
					result["tos_url"] = "https://dsg.test/tos"
				}
				entries = append(entries, result)
			}
			return mainJSONResponse(http.StatusOK, map[string]interface{}{"entries": entries}), nil
		default:
			return mainJSONResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
		}
	})}
}

func mainJSONResponse(status int, body interface{}) *http.Response {
	var encoded bytes.Buffer
	_ = json.NewEncoder(&encoded).Encode(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(&encoded),
	}
}

func setupAPIForTest(t *testing.T, client *http.Client, disableAuth, allowAnonymousReads bool) *echo.Echo {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("help"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "closed.json"), []byte(`{"layer":"closed"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &storage.NoStore{Datasets: []string{"closed", "public", "tos"}}
	e, _, dsgClient, err := newServer(config.Config{
		SwaggerDir: tmpDir, NgDir: tmpDir, DisableAuth: disableAuth,
		DSGUrl: "http://dsg.test", DSGCacheTTL: 300, DSGServiceName: "neuprint",
		AllowAnonymousReads: allowAnonymousReads,
	}, store, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	dsgClient.SetHTTPClient(client)
	return e
}

func TestAnonymousPublicAndTOSReadRoutes(t *testing.T) {
	fixture := &mainDSGFixture{anonymousDecision: map[string]string{
		"public": "allow",
		"tos":    "tos_required",
	}}
	e := setupAPIForTest(t, fixture.client(), false, true)
	for _, tc := range []struct {
		name       string
		dataset    string
		wantStatus int
		wantBody   string
	}{
		{"DSG-public read", "public", http.StatusOK, "columns"},
		{"public plus TOS", "tos", http.StatusForbidden, `"tos_required":true`},
		{"closed dataset", "closed", http.StatusUnauthorized, "authentication required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReader(`{"dataset":"` + tc.dataset + `","cypher":"RETURN 1"}`)
			req := httptest.NewRequest(http.MethodPost, "https://example.com/api/custom/custom", body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, req)
			if recorder.Code != tc.wantStatus || !strings.Contains(recorder.Body.String(), tc.wantBody) {
				t.Fatalf("status=%d body=%s, want status=%d containing %q", recorder.Code, recorder.Body.String(), tc.wantStatus, tc.wantBody)
			}
		})
	}
}

func TestEveryAPIRouteHasDeclaredPolicy(t *testing.T) {
	fixture := &mainDSGFixture{}
	e := setupAPIForTest(t, fixture.client(), false, false)
	policies := api.DeclaredRoutePolicies()

	for _, route := range e.Routes() {
		if !strings.HasPrefix(route.Path, "/api") {
			continue
		}
		// Echo materializes Group.Use as method-wide /api and /api/*
		// trampoline routes. They are middleware plumbing, not application
		// endpoints registered through SetupRoutes/SetRoute/SetAdminRoute.
		if route.Path == "/api" || route.Path == "/api/*" {
			continue
		}
		key := api.RoutePolicyKey{Method: route.Method, Path: route.Path}
		if _, ok := policies[key]; !ok {
			t.Errorf("unclassified API route: %s %s", route.Method, route.Path)
		}
	}

	wantExceptions := map[api.RoutePolicyKey]bool{
		{Method: http.MethodGet, Path: "/api/serverinfo"}:            true,
		{Method: http.MethodGet, Path: "/api/vimoserver"}:            true,
		{Method: http.MethodGet, Path: "/api/version"}:               true,
		{Method: http.MethodGet, Path: "/api/available"}:             true,
		{Method: http.MethodGet, Path: "/api/help*"}:                 true,
		{Method: http.MethodGet, Path: "/api/dbmeta/datasets"}:       true,
		{Method: http.MethodGet, Path: "/api/v:ver/dbmeta/datasets"}: true,
	}
	for key, policy := range policies {
		if policy == api.NamedExceptionRoute && !wantExceptions[key] {
			t.Errorf("unnamed metadata exception: %s %s", key.Method, key.Path)
		}
	}
	for key := range wantExceptions {
		if policies[key] != api.NamedExceptionRoute {
			t.Errorf("missing named exception: %s %s", key.Method, key.Path)
		}
	}

	wantAdminPaths := []string{
		"/skeletons/skeleton/:dataset/:id",
		"/roimeshes/mesh/:dataset/:roi",
		"/raw/keyvalue/key/:instance/:key",
		"/raw/cypher/cypher",
		"/raw/cypher/transaction",
		"/raw/cypher/transaction/:id/commit",
		"/raw/cypher/transaction/:id/cypher",
		"/raw/cypher/transaction/:id/kill",
	}
	wantAdmin := make(map[api.RoutePolicyKey]bool, len(wantAdminPaths)*2)
	for _, path := range wantAdminPaths {
		wantAdmin[api.RoutePolicyKey{Method: http.MethodPost, Path: "/api" + path}] = true
		wantAdmin[api.RoutePolicyKey{Method: http.MethodPost, Path: "/api/v:ver" + path}] = true
	}
	for key, policy := range policies {
		if policy == api.AdminRoute && !wantAdmin[key] {
			t.Errorf("unexpected admin route: %s %s", key.Method, key.Path)
		}
	}
	for key := range wantAdmin {
		if policies[key] != api.AdminRoute {
			t.Errorf("missing admin route classification: %s %s", key.Method, key.Path)
		}
	}
}

func TestPreviouslyUnguardedClosedReadsAreDenied(t *testing.T) {
	fixture := &mainDSGFixture{}
	e := setupAPIForTest(t, fixture.client(), false, true)
	for _, path := range []string{
		"/api/skeletons/skeleton/closed/1",
		"/api/roimeshes/mesh/closed/roi",
		"/api/cached/roiconnectivity?dataset=closed",
		"/api/npexplorer/nglayers/closed.json",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com"+path, nil))
			if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "authentication required") {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMutationWriteContractThroughRegisteredRoute(t *testing.T) {
	for _, tc := range []struct {
		name          string
		token         string
		disableAuth   bool
		wantStatus    int
		wantUserCalls int
		wantDecisions int
	}{
		{"public view-only user denied", "reader", false, http.StatusForbidden, 1, 1},
		{"explicit dataset admin grant succeeds", "writer", false, http.StatusOK, 1, 1},
		{"global admin succeeds without decision", "global", false, http.StatusOK, 1, 0},
		{"disable auth succeeds without DSG", "", true, http.StatusOK, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &mainDSGFixture{}
			e := setupAPIForTest(t, fixture.client(), tc.disableAuth, false)
			body := strings.NewReader(`{"dataset":"closed","cypher":"RETURN 1"}`)
			req := httptest.NewRequest(http.MethodPost, "https://example.com/api/raw/cypher/cypher", body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tc.token != "" {
				req.Header.Set(echo.HeaderAuthorization, "Bearer "+tc.token)
			}
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, req)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), tc.wantStatus)
			}
			if fixture.userCalls != tc.wantUserCalls || fixture.authorizeCalls != tc.wantDecisions {
				t.Fatalf("DSG calls user=%d authorize=%d, want %d/%d", fixture.userCalls, fixture.authorizeCalls, tc.wantUserCalls, tc.wantDecisions)
			}
		})
	}
}

func TestServerInfoCapabilitySemantics(t *testing.T) {
	for _, tc := range []struct {
		name              string
		datasets          []string
		anonymousDecision map[string]string
		disableAuth       bool
		allowAnonymous    bool
		wantPublic        bool
		wantAuthorize     int
	}{
		{"anonymous reads, at least one public dataset", []string{"closed", "public"}, map[string]string{"public": "allow"}, false, true, true, 1},
		{"anonymous reads, zero public datasets", []string{"closed"}, nil, false, true, false, 1},
		{"default requires a token despite public datasets", []string{"closed", "public"}, map[string]string{"public": "allow"}, false, false, false, 0},
		{"disable auth", []string{"closed"}, nil, true, false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &mainDSGFixture{anonymousDecision: tc.anonymousDecision}
			e, _, dsgClient, err := newServer(config.Config{DisableAuth: tc.disableAuth, DSGUrl: "http://dsg.test", AllowAnonymousReads: tc.allowAnonymous}, &storage.NoStore{Datasets: tc.datasets}, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			dsgClient.SetHTTPClient(fixture.client())
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/api/serverinfo", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var info struct {
				IsPublic bool
				Version  string
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil {
				t.Fatalf("invalid serverinfo JSON: %v", err)
			}
			if info.IsPublic != tc.wantPublic || info.Version == "" {
				t.Fatalf("serverinfo=%+v, want IsPublic=%v and a version", info, tc.wantPublic)
			}
			if fixture.authorizeCalls != tc.wantAuthorize {
				t.Fatalf("authorize calls=%d, want %d", fixture.authorizeCalls, tc.wantAuthorize)
			}
		})
	}
}

func TestServerInfoAnnouncementFields(t *testing.T) {
	var options config.Config
	if err := json.Unmarshal([]byte(`{"disable-auth":true,"announcement":"Tokens have moved","announcement-id":"auth-migration-1"}`), &options); err != nil {
		t.Fatal(err)
	}
	e, _, _, err := newServer(options, &storage.NoStore{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/api/serverinfo", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var info map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info["announcement"] != "Tokens have moved" || info["announcement-id"] != "auth-migration-1" {
		t.Fatalf("announcement fields missing from serverinfo: %v", info)
	}
	if _, ok := info["IsPublic"]; !ok {
		t.Fatalf("backward-compatible IsPublic key missing: %v", info)
	}
	if _, ok := info["Version"]; !ok {
		t.Fatalf("backward-compatible Version key missing: %v", info)
	}
}

func TestLlmsTxtServedFromDisk(t *testing.T) {
	llmsPath := filepath.Join(t.TempDir(), "llms.txt")
	if err := os.WriteFile(llmsPath, []byte("# neuPrint\nfirst version\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Auth enabled with no token: llms.txt must be readable anonymously.
	e, _, _, err := newServer(config.Config{DSGUrl: "http://dsg.test", LlmsTxt: llmsPath}, &storage.NoStore{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	get := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/llms.txt", nil))
		return recorder
	}

	recorder := get()
	if recorder.Code != http.StatusOK || recorder.Body.String() != "# neuPrint\nfirst version\n" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get(echo.HeaderContentType); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content-type=%q", ct)
	}

	// Edits to the file take effect without restarting the server.
	if err := os.WriteFile(llmsPath, []byte("# neuPrint\nsecond version\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if recorder := get(); recorder.Body.String() != "# neuPrint\nsecond version\n" {
		t.Fatalf("edited file not served: body=%q", recorder.Body.String())
	}

	if err := os.Remove(llmsPath); err != nil {
		t.Fatal(err)
	}
	if recorder := get(); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing file: status=%d, want 404", recorder.Code)
	}
}

func TestLlmsTxtTakesPrecedenceOverStaticDir(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "llms.txt"), []byte("static copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	llmsPath := filepath.Join(t.TempDir(), "llms.txt")
	if err := os.WriteFile(llmsPath, []byte("configured copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	e, _, _, err := newServer(config.Config{DisableAuth: true, StaticDir: staticDir, LlmsTxt: llmsPath}, &storage.NoStore{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/llms.txt", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "configured copy" {
		t.Fatalf("status=%d body=%q, want configured copy", recorder.Code, recorder.Body.String())
	}

	// A missing file must be a real 404, not the static-dir index.html fallback.
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>explorer</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(llmsPath); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/llms.txt", nil))
	if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "explorer") {
		t.Fatalf("missing file with static-dir: status=%d body=%q, want plain 404", recorder.Code, recorder.Body.String())
	}
}

func TestLlmsTxtUnconfigured(t *testing.T) {
	e, _, _, err := newServer(config.Config{DisableAuth: true}, &storage.NoStore{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/llms.txt", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 when llms-txt is unset", recorder.Code)
	}
}

func TestRemovedPublicReadDetectionAndDisableWarning(t *testing.T) {
	for _, args := range [][]string{
		{"-public_read"}, {"--public_read"}, {"-public_read=true"}, {"--public_read=false"},
	} {
		if !flagWasProvided(args, "public_read") {
			t.Errorf("did not detect removed flag in %v", args)
		}
	}
	if flagWasProvided([]string{"-port", "11000", "config.json"}, "public_read") {
		t.Fatal("reported removed flag when absent")
	}
	var omitted config.Config
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil || omitted.DisableAuth {
		t.Fatalf("omitted disable-auth must default off: config=%+v err=%v", omitted, err)
	}
	var warning bytes.Buffer
	writeDisableAuthWarning(&warning)
	if !strings.Contains(warning.String(), "ALL AUTHORIZATION IS DISABLED") {
		t.Fatalf("warning was not loud and explicit: %q", warning.String())
	}
}

func TestSharedServerConfigurationAndRedirects(t *testing.T) {
	if _, _, _, err := newServer(config.Config{}, &storage.NoStore{}, io.Discard); err == nil {
		t.Fatal("missing DSG URL accepted with auth enabled")
	}
	e, tlsServer, client, err := newServer(config.Config{DSGUrl: "http://dsg.test", DSGCacheTTL: 1, DSGServiceName: "fixture"}, &storage.NoStore{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if tlsServer == nil || client.CacheTTL != time.Second || client.ServiceName != "fixture" {
		t.Fatal("shared constructor lost DSG/TLS configuration")
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/serverinfo", nil)
	req.Host = "example.com"
	e.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusMovedPermanently || recorder.Header().Get("Location") != "https://example.com/api/serverinfo" {
		t.Fatalf("HTTPS redirect changed: %d %s", recorder.Code, recorder.Header().Get("Location"))
	}
}

// concreteRoutePath fills an Echo route pattern's parameters so that a
// request matches that route.
func concreteRoutePath(pattern string) string {
	if strings.HasSuffix(pattern, "*") {
		return strings.TrimSuffix(pattern, "*") + "/"
	}
	parts := strings.Split(pattern, "/")
	for i, part := range parts {
		switch {
		case part == "v:ver":
			parts[i] = "v" + version.Version
		case strings.HasPrefix(part, ":"):
			parts[i] = "public"
		}
	}
	return strings.Join(parts, "/")
}

func isAPIEndpoint(path string) bool {
	return strings.HasPrefix(path, "/api") && path != "/api" && path != "/api/*"
}

func TestTokenRequiredByDefaultOnEveryDataRoute(t *testing.T) {
	fixture := &mainDSGFixture{anonymousDecision: map[string]string{"public": "allow"}}
	e := setupAPIForTest(t, fixture.client(), false, false)
	paths := []struct{ method, path string }{{http.MethodGet, "/api/no-such-route"}}
	for _, route := range e.Routes() {
		if !isAPIEndpoint(route.Path) {
			continue
		}
		if policy, _ := api.RoutePolicyFor(route.Method, route.Path); policy == api.NamedExceptionRoute {
			continue
		}
		paths = append(paths, struct{ method, path string }{route.Method, concreteRoutePath(route.Path)})
	}
	if len(paths) < 40 {
		t.Fatalf("only %d data routes found; route enumeration is broken", len(paths))
	}
	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			body := strings.NewReader(`{"dataset":"public","cypher":"RETURN 1"}`)
			req := httptest.NewRequest(p.method, "https://example.com"+p.path, body)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "requires an API token for all data") {
				t.Fatalf("status=%d body=%s, want 401 with the login message", recorder.Code, recorder.Body.String())
			}
		})
	}
	if fixture.userCalls != 0 || fixture.authorizeCalls != 0 {
		t.Fatalf("refused anonymous requests consulted DSG: user=%d authorize=%d", fixture.userCalls, fixture.authorizeCalls)
	}
}

func TestNamedExceptionsOpenWithoutTokenByDefault(t *testing.T) {
	fixture := &mainDSGFixture{anonymousDecision: map[string]string{"public": "allow"}}
	e := setupAPIForTest(t, fixture.client(), false, false)
	seen := 0
	for _, route := range e.Routes() {
		if !isAPIEndpoint(route.Path) {
			continue
		}
		if policy, _ := api.RoutePolicyFor(route.Method, route.Path); policy != api.NamedExceptionRoute {
			continue
		}
		seen++
		path := concreteRoutePath(route.Path)
		t.Run(route.Method+" "+path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(route.Method, "https://example.com"+path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s, want 200 without a token", recorder.Code, recorder.Body.String())
			}
		})
	}
	if seen != 7 {
		t.Fatalf("saw %d named-exception routes, want 7", seen)
	}

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/api/dbmeta/datasets", nil))
	var listing map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listing); err != nil {
		t.Fatalf("invalid listing JSON: %v", err)
	}
	if _, ok := listing["public"]; !ok || len(listing) != 1 {
		t.Fatalf("anonymous listing=%v, want exactly the DSG-public dataset", listing)
	}

	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.com/api/serverinfo", nil))
	var info struct{ IsPublic bool }
	if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil || info.IsPublic {
		t.Fatalf("serverinfo=%s err=%v, want IsPublic false", recorder.Body.String(), err)
	}
}

func TestTokenReadsPublicDatasetByDefault(t *testing.T) {
	fixture := &mainDSGFixture{anonymousDecision: map[string]string{"public": "allow"}}
	e := setupAPIForTest(t, fixture.client(), false, false)
	body := strings.NewReader(`{"dataset":"public","cypher":"RETURN 1"}`)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/custom/custom", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer reader")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "columns") {
		t.Fatalf("status=%d body=%s, want 200 with results", recorder.Code, recorder.Body.String())
	}
}

func TestRequestLogLineIsEscapedJSON(t *testing.T) {
	fixture := &mainDSGFixture{}
	var logOutput bytes.Buffer
	e, _, dsgClient, err := newServer(config.Config{DSGUrl: "http://dsg.test"}, &storage.NoStore{Datasets: []string{"public"}}, &logOutput)
	if err != nil {
		t.Fatal(err)
	}
	dsgClient.SetHTTPClient(fixture.client())

	cypher := "MATCH (n)\nWHERE n.name = \"a\\\\b\"\tAND n.x = 1\nRETURN n"
	agent := `evil", "user": "forged@example.com`
	requestBody, _ := json.Marshal(map[string]string{"dataset": "public", "cypher": cypher})
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/custom/custom", bytes.NewReader(requestBody))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer reader")
	req.Header.Set("User-Agent", agent)
	req.RequestURI = "/api/custom/custom" // as a server sees it; httptest keeps the absolute URL
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	lines := strings.Split(strings.TrimSuffix(logOutput.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("one request produced %d log lines: %q", len(lines), logOutput.String())
	}
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, lines[0])
	}
	for key, want := range map[string]string{
		"user":    "reader@example.com",
		"agent":   agent,
		"debug":   cypher,
		"dataset": "public",
		"uri":     "/api/custom/custom",
	} {
		if record[key] != want {
			t.Errorf("log %s=%q, want %q", key, record[key], want)
		}
	}
}
