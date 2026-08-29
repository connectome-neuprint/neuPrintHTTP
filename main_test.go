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

	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"github.com/connectome-neuprint/neuPrintHTTP/secure"
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

func (f *mainDSGFixture) client() *secure.DSGClient {
	client := secure.NewDSGClient("http://dsg.test", 300, "neuprint")
	client.SetHTTPClient(&http.Client{Transport: mainRoundTripFunc(func(r *http.Request) (*http.Response, error) {
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
	})})
	return client
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

func setupAPIForTest(t *testing.T, client *secure.DSGClient, disableAuth bool) *echo.Echo {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("help"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "closed.json"), []byte(`{"layer":"closed"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	group := e.Group("/api")
	group.Use(secure.DSGOptionalAuthMiddleware(client, disableAuth, ""))
	store := &storage.NoStore{Datasets: []string{"closed", "public", "tos"}}
	registerBaseAPIRoutes(group, config.Config{SwaggerDir: tmpDir, NgDir: tmpDir, DisableAuth: disableAuth}, store)
	if err := api.SetupRoutes(e, group, store, secure.DSGAdminMiddleware()); err != nil {
		t.Fatalf("SetupRoutes: %v", err)
	}
	return e
}

func TestAnonymousPublicAndTOSReadRoutes(t *testing.T) {
	fixture := &mainDSGFixture{anonymousDecision: map[string]string{
		"public": "allow",
		"tos":    "tos_required",
	}}
	e := setupAPIForTest(t, fixture.client(), false)
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
			req := httptest.NewRequest(http.MethodPost, "/api/custom/custom", body)
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
	e := setupAPIForTest(t, fixture.client(), false)
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
	e := setupAPIForTest(t, fixture.client(), false)
	for _, path := range []string{
		"/api/skeletons/skeleton/closed/1",
		"/api/roimeshes/mesh/closed/roi",
		"/api/cached/roiconnectivity?dataset=closed",
		"/api/npexplorer/nglayers/closed.json",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
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
			e := setupAPIForTest(t, fixture.client(), tc.disableAuth)
			body := strings.NewReader(`{"dataset":"closed","cypher":"RETURN 1"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/raw/cypher/cypher", body)
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
		wantPublic        bool
		wantAuthorize     int
	}{
		{"at least one public dataset", []string{"closed", "public"}, map[string]string{"public": "allow"}, false, true, 1},
		{"zero public datasets", []string{"closed"}, nil, false, false, 1},
		{"disable auth", []string{"closed"}, nil, true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &mainDSGFixture{anonymousDecision: tc.anonymousDecision}
			e := echo.New()
			group := e.Group("/api")
			group.Use(secure.DSGOptionalAuthMiddleware(fixture.client(), tc.disableAuth, ""))
			registerBaseAPIRoutes(group, config.Config{DisableAuth: tc.disableAuth}, &storage.NoStore{Datasets: tc.datasets})
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/serverinfo", nil))
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
	e := echo.New()
	group := e.Group("/api")
	group.Use(secure.DSGOptionalAuthMiddleware((&mainDSGFixture{}).client(), true, ""))
	registerBaseAPIRoutes(group, options, &storage.NoStore{})

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/serverinfo", nil))
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
