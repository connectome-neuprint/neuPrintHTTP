package secure

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type loginRedirectCase struct {
	name                 string
	query                url.Values
	dsgURL               string
	wantRedirectContains string
}

func TestDsgLoginHandler_RedirectURL(t *testing.T) {
	cases := []loginRedirectCase{
		{
			name:                 "bare login defaults to /",
			query:                url.Values{},
			dsgURL:               "https://dsg.example.com",
			wantRedirectContains: "http://neuprint.example.com/",
		},
		{
			name: "redirect path is preserved",
			query: url.Values{
				"redirect": {"/results?qr=1"},
			},
			dsgURL:               "https://dsg.example.com",
			wantRedirectContains: "/results?qr=1",
		},
		{
			name: "dataset query is ignored by login",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain:v1.2.1"},
			},
			dsgURL:               "https://dsg.example.com",
			wantRedirectContains: "http://neuprint.example.com/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := dsgLoginHandler(tc.dsgURL)

			e := echo.New()
			target := "/login?" + tc.query.Encode()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Host = "neuprint.example.com"
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := handler(c); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}

			location := rec.Header().Get("Location")
			locURL, err := url.Parse(location)
			if err != nil {
				t.Fatalf("unparseable Location: %v", err)
			}
			if got := locURL.Path; got != "/api/v1/authorize" {
				t.Errorf("DSG path = %q, want /api/v1/authorize", got)
			}

			locQuery := locURL.Query()
			redirectVal := locQuery.Get("redirect")
			if redirectVal == "" {
				t.Fatal("redirect param missing from DSG URL")
			}
			if !containsSubstring(redirectVal, tc.wantRedirectContains) {
				t.Errorf("redirect=%q does not contain %q", redirectVal, tc.wantRedirectContains)
			}
			if gotDataset := locQuery.Get("dataset"); gotDataset != "" {
				t.Errorf("dataset param = %q, want absent", gotDataset)
			}
			if gotService := locQuery.Get("service"); gotService != "" {
				t.Errorf("service param = %q, want absent", gotService)
			}
		})
	}
}

func TestDsgLogoutRoutes_RedirectWithoutAuthentication(t *testing.T) {
	cases := []struct {
		name   string
		method string
		cookie string
	}{
		{name: "GET without cookie", method: http.MethodGet},
		{name: "GET with stale cookie", method: http.MethodGet, cookie: "garbage-stale-token"},
		{name: "POST without cookie", method: http.MethodPost},
		{name: "POST with stale cookie", method: http.MethodPost, cookie: "garbage-stale-token"},
	}

	const dsgURL = "https://dsg.janelia.org"
	const redirectURL = "https://neuprint.janelia.org/"
	wantLocation := dsgURL + "/api/v1/logout?redirect=" + url.QueryEscape(redirectURL)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDSG(t)
			fake.identityStatus = http.StatusUnauthorized
			e := echo.New()
			if _, err := InitializeEchoSecure(e, "", "", "", dsgURL, fake.client()); err != nil {
				t.Fatalf("InitializeEchoSecure returned error: %v", err)
			}

			req := httptest.NewRequest(tc.method, "https://neuprint.janelia.org/logout", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "dsg_token", Value: tc.cookie})
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != wantLocation {
				t.Fatalf("Location = %q, want %q", got, wantLocation)
			}
			location, err := url.Parse(rec.Header().Get("Location"))
			if err != nil {
				t.Fatalf("unparseable Location: %v", err)
			}
			if got := location.Query().Get("redirect"); got != redirectURL {
				t.Fatalf("redirect = %q, want %q", got, redirectURL)
			}
			if fake.userCalls != 0 {
				t.Fatalf("logout route made %d DSG auth calls, want 0", fake.userCalls)
			}
		})
	}
}

func TestRequireDatasetAccess_ErrorIncludesDatasetName(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decide = func(entry authorizeEntry) DSGDecision {
		return DSGDecision{Name: entry.Name, Version: entry.Version, Decision: "deny", Roles: []string{}}
	}
	client := fake.client()

	for _, dataset := range []string{"hemibrain", "vnc:v1.0", "VNC"} {
		t.Run(dataset, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("dsg_identity", &DSGIdentity{Email: "test@example.com"})
			c.Set("dsg_client", client)
			c.Set("dsg_token", "tok-user")

			err := RequireDatasetAccess(c, dataset, READ)
			if err == nil {
				t.Fatal("expected error for unauthorized dataset")
			}
			httpErr, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("expected echo.HTTPError, got %T", err)
			}
			msg, _ := httpErr.Message.(string)
			if !containsSubstring(msg, dataset) {
				t.Errorf("error message %q should contain dataset name %q", msg, dataset)
			}
		})
	}
}

func TestRequireDatasetAccess_TOSResponseIncludesURL(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decide = func(entry authorizeEntry) DSGDecision {
		return DSGDecision{
			Name:     entry.Name,
			Version:  entry.Version,
			Decision: "tos_required",
			Roles:    []string{"view"},
			TOSURL:   "https://dsg.example.com/opaque-native-tos?opaque=1",
		}
	}
	client := fake.client()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_identity", &DSGIdentity{Email: "test@example.com"})
	c.Set("dsg_client", client)
	c.Set("dsg_token", "tok-user")

	err := RequireDatasetAccess(c, "hemibrain:v1.2.1", READ)
	if err != echo.ErrForbidden {
		t.Fatalf("handler error = %v, want echo.ErrForbidden after writing TOS response", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["tos_url"] != "https://dsg.example.com/opaque-native-tos?opaque=1" {
		t.Errorf("tos_url = %q", body["tos_url"])
	}
}

func TestRequireDatasetAccess_AdminShortCircuit(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_identity", &DSGIdentity{Email: "admin@example.com", Admin: true})

	if err := RequireDatasetAccess(c, "unregistered:v1", ADMIN); err != nil {
		t.Fatalf("admin should be allowed without a native decision: %v", err)
	}
	if got := c.Get("level"); got != "admin" {
		t.Errorf("level = %v, want admin", got)
	}
}

func TestDsgDatasetAccessHandler_AdminShortCircuitSkipsAuthorize(t *testing.T) {
	fake := newFakeDSG(t)
	client := fake.client()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/dataset-access?dataset=unregistered%3Av1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_identity", &DSGIdentity{Email: "admin@example.com", Admin: true})
	c.Set("dsg_client", client)

	if err := dsgDatasetAccessHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fake.authorizeCalls != 0 {
		t.Fatalf("admin shortcut should not call authorize, got %d calls", fake.authorizeCalls)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["access"] != true || body["level"] != "admin" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestDsgDatasetAccessHandler_ReturnsNativeTOSURL(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decide = func(entry authorizeEntry) DSGDecision {
		return DSGDecision{
			Name:     entry.Name,
			Version:  entry.Version,
			Decision: "tos_required",
			Roles:    []string{"view"},
			TOSURL:   "https://dsg.example.com/opaque-native-tos?opaque=1",
		}
	}
	client := fake.client()

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodGet,
		"/dataset-access?dataset=hemibrain%3Av1.2.1&next=https%3A%2F%2Fneuprint.example.com%2F%3Fdataset%3Dhemibrain%253Av1.2.1",
		nil,
	)
	req.Host = "neuprint.example.com"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_identity", &DSGIdentity{Email: "test@example.com"})
	c.Set("dsg_client", client)
	c.Set("dsg_token", "tok-user")

	if err := dsgDatasetAccessHandler(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["access"] != false || body["tos_required"] != true {
		t.Fatalf("unexpected body: %v", body)
	}
	if body["tos_url"] != "https://dsg.example.com/opaque-native-tos?opaque=1" {
		t.Errorf("tos_url = %q", body["tos_url"])
	}
	legacyKey := "dsg" + "_" + "dataset"
	if _, ok := body[legacyKey]; ok {
		t.Errorf("response leaked legacy dataset key: %v", body)
	}
}

func TestDsgTokenHandlerProxiesBearerToken(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() {
		http.DefaultClient = oldClient
	}()

	var gotPath string
	var gotAuth string
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		return jsonHTTPResponse(http.StatusOK, map[string]string{"token": "stable-token"}), nil
	})}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/token", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-user")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := dsgTokenHandler("http://dsg.test")(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if gotPath != "/api/v1/long_lived_token" {
		t.Fatalf("proxied path = %q", gotPath)
	}
	if gotAuth != "Bearer tok-user" {
		t.Fatalf("proxied auth = %q", gotAuth)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["token"] != "stable-token" {
		t.Fatalf("token body = %v", body)
	}
}

func TestDSGProfileHandlerIncludesImageURL(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_identity", &DSGIdentity{
		Email:      "alice@example.org",
		PictureURL: "https://example.org/alice.png",
	})
	c.Set("level", "readwrite")

	if err := dsgProfileHandler(c); err != nil {
		t.Fatalf("unexpected profile handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode profile response: %v", err)
	}
	if body["Email"] != "alice@example.org" {
		t.Fatalf("expected Email alice@example.org, got %q", body["Email"])
	}
	if body["AuthLevel"] != "readwrite" {
		t.Fatalf("expected AuthLevel readwrite, got %q", body["AuthLevel"])
	}
	if body["ImageURL"] != "https://example.org/alice.png" {
		t.Fatalf("expected ImageURL to be propagated, got %q", body["ImageURL"])
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func newRotateTestClient(status int, body interface{}, calls *[]*http.Request) *DSGClient {
	dsg := NewDSGClient("http://dsg.test", 300, "neuprint")
	dsg.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		*calls = append(*calls, r)
		return jsonHTTPResponse(status, body), nil
	})})
	return dsg
}

func seedTokenCaches(dsg *DSGClient, tokens ...string) {
	for _, tok := range tokens {
		dsg.identityCache.Store(tok, &cachedIdentityEntry{data: &DSGIdentity{}, fetchedAt: time.Now()})
		dsg.decisionCache.Store(
			decisionCacheKey{token: tok, name: "hemibrain"},
			&cachedDecisionEntry{data: &DSGDecision{}, fetchedAt: time.Now()},
		)
	}
}

func tokenCached(dsg *DSGClient, tok string) (identity, decision bool) {
	_, identity = dsg.identityCache.Load(tok)
	_, decision = dsg.decisionCache.Load(decisionCacheKey{token: tok, name: "hemibrain"})
	return identity, decision
}

func TestDsgTokenRotateHandlerRequiresBearerHeader(t *testing.T) {
	var calls []*http.Request
	dsg := newRotateTestClient(http.StatusOK, map[string]string{"token": "tok-new"}, &calls)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/token/rotate", nil)
	req.AddCookie(&http.Cookie{Name: "dsg_token", Value: "tok-session"})
	rec := httptest.NewRecorder()

	err := dsgTokenRotateHandler("http://dsg.test", dsg)(e.NewContext(req, rec))
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want 400 HTTPError", err)
	}
	if len(calls) != 0 {
		t.Fatalf("DSG called %d times without a bearer header", len(calls))
	}
}

func TestDsgTokenRotateHandlerProxiesAndForgetsOldToken(t *testing.T) {
	var calls []*http.Request
	dsg := newRotateTestClient(http.StatusOK, map[string]string{"token": "tok-new"}, &calls)
	seedTokenCaches(dsg, "tok-old", "tok-other")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/token/rotate", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-old")
	rec := httptest.NewRecorder()

	if err := dsgTokenRotateHandler("http://dsg.test", dsg)(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("DSG calls = %d, want 1", len(calls))
	}
	if calls[0].Method != http.MethodPost || calls[0].URL.Path != "/api/v1/long_lived_token/rotate" {
		t.Fatalf("proxied %s %s", calls[0].Method, calls[0].URL.Path)
	}
	if got := calls[0].Header.Get("Authorization"); got != "Bearer tok-old" {
		t.Fatalf("proxied auth = %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["token"] != "tok-new" {
		t.Fatalf("body = %s (err %v)", rec.Body.String(), err)
	}
	if identity, decision := tokenCached(dsg, "tok-old"); identity || decision {
		t.Fatalf("old token still cached: identity=%v decision=%v", identity, decision)
	}
	if identity, decision := tokenCached(dsg, "tok-other"); !identity || !decision {
		t.Fatalf("unrelated token evicted: identity=%v decision=%v", identity, decision)
	}
}

func TestDsgTokenRotateHandlerRelaysFailureAndKeepsCache(t *testing.T) {
	var calls []*http.Request
	dsg := newRotateTestClient(http.StatusForbidden, map[string]string{"detail": "denied"}, &calls)
	seedTokenCaches(dsg, "tok-old")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/token/rotate", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-old")
	rec := httptest.NewRecorder()

	if err := dsgTokenRotateHandler("http://dsg.test", dsg)(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["detail"] != "denied" {
		t.Fatalf("relayed body = %s (err %v)", rec.Body.String(), err)
	}
	if identity, _ := tokenCached(dsg, "tok-old"); !identity {
		t.Fatal("token evicted although rotation failed")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func TestDsgTokenRotateHandlerTruncatedResponseIsBadGateway(t *testing.T) {
	dsg := NewDSGClient("http://dsg.test", 300, "neuprint")
	dsg.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(io.MultiReader(strings.NewReader(`{"tok`), failingReader{})),
		}, nil
	})})
	seedTokenCaches(dsg, "tok-old")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/token/rotate", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-old")
	rec := httptest.NewRecorder()

	err := dsgTokenRotateHandler("http://dsg.test", dsg)(e.NewContext(req, rec))
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadGateway {
		t.Fatalf("err = %v, want 502 HTTPError", err)
	}
	// DSG answered 200, so the rotation may have committed: forget the old token.
	if identity, decision := tokenCached(dsg, "tok-old"); identity || decision {
		t.Fatalf("old token still cached: identity=%v decision=%v", identity, decision)
	}
}

// rotateRouteServer builds the production route stack (InitializeEchoSecure,
// DSGAuthMiddleware) against a fake DSG that knows tok-old and tok-session.
func rotateRouteServer(t *testing.T, rotateCalls *int) *echo.Echo {
	t.Helper()
	dsg := NewDSGClient("http://dsg.test", 300, "neuprint")
	dsg.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/dsg/v1/user":
			switch r.Header.Get("Authorization") {
			case "Bearer tok-old", "Bearer tok-session":
				return jsonHTTPResponse(http.StatusOK, DSGIdentity{ID: 7, Email: "alice@example.org"}), nil
			}
			return jsonHTTPResponse(http.StatusUnauthorized, map[string]string{"detail": "invalid"}), nil
		case "/api/v1/long_lived_token/rotate":
			*rotateCalls++
			return jsonHTTPResponse(http.StatusOK, map[string]string{"token": "tok-new"}), nil
		}
		return jsonHTTPResponse(http.StatusNotFound, map[string]string{"detail": "not found"}), nil
	})})
	e := echo.New()
	if _, err := InitializeEchoSecure(e, "cert.pem", "key.pem", "neuprint.test", "http://dsg.test", dsg); err != nil {
		t.Fatalf("InitializeEchoSecure: %v", err)
	}
	return e
}

func TestTokenRotateRouteThroughAuthMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		prepare    func(*http.Request)
		wantStatus int
		wantCalls  int
	}{
		{"no credentials", func(*http.Request) {}, http.StatusUnauthorized, 0},
		{"invalid bearer", func(r *http.Request) {
			r.Header.Set(echo.HeaderAuthorization, "Bearer tok-dead")
		}, http.StatusUnauthorized, 0},
		{"cookie only", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "dsg_token", Value: "tok-session"})
		}, http.StatusBadRequest, 0},
		{"bearer", func(r *http.Request) {
			r.Header.Set(echo.HeaderAuthorization, "Bearer tok-old")
		}, http.StatusOK, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			e := rotateRouteServer(t, &calls)
			req := httptest.NewRequest(http.MethodPost, "/token/rotate", nil)
			req.Header.Set(echo.HeaderXForwardedProto, "https")
			tc.prepare(req)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if calls != tc.wantCalls {
				t.Fatalf("DSG rotate calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

// gatedDSG answers identity and authorize lookups only when released, so a
// test can revoke a token while a lookup for it is in flight.
func gatedDSG() (*DSGClient, chan struct{}, chan struct{}) {
	arrived := make(chan struct{}, 4)
	release := make(chan struct{})
	dsg := NewDSGClient("http://dsg.test", 300, "neuprint")
	dsg.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		arrived <- struct{}{}
		<-release
		switch r.URL.Path {
		case "/api/dsg/v1/user":
			return jsonHTTPResponse(http.StatusOK, DSGIdentity{ID: 7, Email: "alice@example.org"}), nil
		case "/api/dsg/v1/authorize":
			var req authorizeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				return nil, err
			}
			resp := authorizeResponse{}
			for _, entry := range req.Entries {
				resp.Entries = append(resp.Entries, DSGDecision{Name: entry.Name, Decision: "allow", Roles: []string{"view"}})
			}
			return jsonHTTPResponse(http.StatusOK, resp), nil
		}
		return jsonHTTPResponse(http.StatusNotFound, map[string]string{}), nil
	})})
	return dsg, arrived, release
}

func TestForgetTokenBlocksInFlightIdentityRecache(t *testing.T) {
	dsg, arrived, release := gatedDSG()
	done := make(chan *DSGIdentity)
	go func() {
		identity, _ := dsg.Identity("tok-old")
		done <- identity
	}()
	<-arrived
	dsg.ForgetToken("tok-old")
	close(release)
	if identity := <-done; identity == nil {
		t.Fatal("in-flight lookup should still answer its own request")
	}
	if _, cached := dsg.identityCache.Load("tok-old"); cached {
		t.Fatal("revoked token re-cached by a lookup that was in flight")
	}
	if _, err := dsg.Identity("tok-other"); err != nil {
		t.Fatalf("Identity(tok-other): %v", err)
	}
	if _, cached := dsg.identityCache.Load("tok-other"); !cached {
		t.Fatal("unrelated token not cached")
	}
}

func TestForgetTokenBlocksInFlightDecisionRecache(t *testing.T) {
	dsg, arrived, release := gatedDSG()
	done := make(chan error)
	go func() {
		_, err := dsg.AuthorizeDatasets("tok-old", []string{"hemibrain"}, "", false)
		done <- err
	}()
	<-arrived
	dsg.ForgetToken("tok-old")
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("AuthorizeDatasets: %v", err)
	}
	if _, decision := tokenCached(dsg, "tok-old"); decision {
		t.Fatal("revoked token's decision re-cached by a lookup that was in flight")
	}
}
