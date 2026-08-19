package secure

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type fakeDSG struct {
	identity       DSGIdentity
	identityStatus int
	decide         func(authorizeEntry) DSGDecision
	decideRequest  func(authorizeEntry, string) DSGDecision
	userCalls      int
	authorizeCalls int
	bodies         []authorizeRequest
	authorizations []string
	authPresent    []bool
}

func newFakeDSG(t *testing.T) *fakeDSG {
	t.Helper()
	f := &fakeDSG{
		identity: DSGIdentity{
			ID:         7,
			Email:      "test@example.com",
			Name:       "Test User",
			PictureURL: "https://example.com/test.png",
		},
		identityStatus: http.StatusOK,
	}
	f.decide = func(entry authorizeEntry) DSGDecision {
		return DSGDecision{
			Name:     entry.Name,
			Version:  entry.Version,
			Decision: "deny",
			Roles:    []string{},
		}
	}
	f.decideRequest = func(entry authorizeEntry, _ string) DSGDecision {
		return f.decide(entry)
	}
	return f
}

func (f *fakeDSG) client() *DSGClient {
	client := NewDSGClient("http://dsg.test", 300, "neuprint")
	client.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/dsg/v1/user":
			f.userCalls++
			if f.identityStatus != http.StatusOK {
				return jsonHTTPResponse(f.identityStatus, map[string]string{"error": "identity"}), nil
			}
			return jsonHTTPResponse(http.StatusOK, f.identity), nil
		case "/api/dsg/v1/authorize":
			f.authorizeCalls++
			authorization, present := r.Header["Authorization"]
			f.authPresent = append(f.authPresent, present)
			if len(authorization) == 0 {
				f.authorizations = append(f.authorizations, "")
			} else {
				f.authorizations = append(f.authorizations, authorization[0])
			}
			var req authorizeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				return nil, err
			}
			f.bodies = append(f.bodies, req)
			resp := authorizeResponse{Entries: make([]DSGDecision, 0, len(req.Entries))}
			for _, entry := range req.Entries {
				resp.Entries = append(resp.Entries, f.decideRequest(entry, r.Header.Get("Authorization")))
			}
			return jsonHTTPResponse(http.StatusOK, resp), nil
		default:
			return jsonHTTPResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
		}
	})})
	return client
}

func jsonHTTPResponse(status int, body interface{}) *http.Response {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(&buf),
	}
}

func TestDSGIdentityCacheAndFresh(t *testing.T) {
	fake := newFakeDSG(t)
	client := fake.client()

	first, err := client.Identity("tok-user")
	if err != nil {
		t.Fatalf("Identity returned error: %v", err)
	}
	second, err := client.Identity("tok-user")
	if err != nil {
		t.Fatalf("cached Identity returned error: %v", err)
	}
	if first.Email != second.Email || fake.userCalls != 1 {
		t.Fatalf("identity cache miss: calls=%d first=%v second=%v", fake.userCalls, first, second)
	}

	if _, err := client.IdentityFresh("tok-user"); err != nil {
		t.Fatalf("IdentityFresh returned error: %v", err)
	}
	if fake.userCalls != 2 {
		t.Fatalf("IdentityFresh should bypass cache, calls=%d", fake.userCalls)
	}
}

func TestDSGIdentityInvalidToken(t *testing.T) {
	fake := newFakeDSG(t)
	fake.identityStatus = http.StatusUnauthorized
	client := fake.client()

	identity, err := client.Identity("bad-token")
	if err != nil {
		t.Fatalf("Identity returned error: %v", err)
	}
	if identity != nil {
		t.Fatalf("invalid token identity = %v, want nil", identity)
	}
}

func TestDSGDecisionLevelMapping(t *testing.T) {
	cases := []struct {
		name     string
		decision DSGDecision
		want     AuthorizationLevel
	}{
		{"allow view", DSGDecision{Decision: "allow", Roles: []string{"view"}}, READ},
		{"allow edit", DSGDecision{Decision: "allow", Roles: []string{"view", "edit"}}, READWRITE},
		{"allow manage", DSGDecision{Decision: "allow", Roles: []string{"manage"}}, READWRITE},
		{"allow admin", DSGDecision{Decision: "allow", Roles: []string{"admin"}}, ADMIN},
		{"allow empty roles floors read", DSGDecision{Decision: "allow"}, READ},
		{"tos required", DSGDecision{Decision: "tos_required", Roles: []string{"view"}}, NOAUTH},
		{"deny", DSGDecision{Decision: "deny"}, NOAUTH},
		{"service eval", DSGDecision{Decision: "service_eval", Roles: []string{"view"}}, NOAUTH},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.decision.Level(); got != tc.want {
				t.Fatalf("Level() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDSGDecisionCacheForceFreshAndTOSInvalidation(t *testing.T) {
	fake := newFakeDSG(t)
	counts := map[string]int{}
	fake.decide = func(entry authorizeEntry) DSGDecision {
		key := entry.Name + ":" + entry.Version
		counts[key]++
		if entry.Name == "flip" && counts[key] == 1 {
			return DSGDecision{
				Name:     entry.Name,
				Version:  entry.Version,
				Decision: "tos_required",
				Roles:    []string{"view"},
				TOSURL:   "https://dsg.example.com/tos",
			}
		}
		return DSGDecision{
			Name:     entry.Name,
			Version:  entry.Version,
			Decision: "allow",
			Roles:    []string{"view"},
		}
	}
	client := fake.client()

	first, err := client.DatasetDecision("tok-user", "cached:v1", "/", false)
	if err != nil || first.Level() != READ {
		t.Fatalf("first cached decision = %v, err=%v", first, err)
	}
	second, err := client.DatasetDecision("tok-user", "cached:v1", "/", false)
	if err != nil || second.Level() != READ {
		t.Fatalf("second cached decision = %v, err=%v", second, err)
	}
	if counts["cached:v1"] != 1 {
		t.Fatalf("allow decision should be cached, calls=%d", counts["cached:v1"])
	}
	if _, err := client.DatasetDecision("tok-user", "cached:v1", "/", true); err != nil {
		t.Fatalf("force-fresh decision returned error: %v", err)
	}
	if counts["cached:v1"] != 2 {
		t.Fatalf("force-fresh should bypass cache, calls=%d", counts["cached:v1"])
	}

	tos, err := client.DatasetDecision("tok-user", "flip:v1", "/", false)
	if err != nil || !tos.TOSRequired() {
		t.Fatalf("first flip decision = %v, err=%v", tos, err)
	}
	allowed, err := client.DatasetDecision("tok-user", "flip:v1", "/", false)
	if err != nil || allowed.Level() != READ {
		t.Fatalf("TOS decision should not be cached; got %v, err=%v", allowed, err)
	}
	if counts["flip:v1"] != 2 {
		t.Fatalf("TOS decision should be refetched immediately, calls=%d", counts["flip:v1"])
	}
}

func TestDSGAuthorizeNameSplitAndBatch(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decide = func(entry authorizeEntry) DSGDecision {
		return DSGDecision{
			Name:     entry.Name,
			Version:  entry.Version,
			Decision: "allow",
			Roles:    []string{"view"},
		}
	}
	client := fake.client()

	decisions, err := client.AuthorizeDatasets(
		"tok-user", []string{"hemibrain:v1.2.1", "vnc"}, "https://neuprint.example.com/", false,
	)
	if err != nil {
		t.Fatalf("AuthorizeDatasets returned error: %v", err)
	}
	if len(decisions) != 2 || fake.authorizeCalls != 1 {
		t.Fatalf("expected one batch with two decisions, calls=%d decisions=%v", fake.authorizeCalls, decisions)
	}
	entries := fake.bodies[0].Entries
	if entries[0].Name != "hemibrain" || entries[0].Version != "v1.2.1" {
		t.Fatalf("versioned entry = %+v", entries[0])
	}
	if entries[1].Name != "vnc" || entries[1].Version != "" {
		t.Fatalf("name-only entry = %+v", entries[1])
	}
}

func TestAnonymousDatasetDecisionWireContractAndCacheSeparation(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decideRequest = func(entry authorizeEntry, authorization string) DSGDecision {
		roles := []string{"view"}
		if authorization != "" {
			roles = []string{"admin"}
		}
		return DSGDecision{Name: entry.Name, Version: entry.Version, Decision: "allow", Roles: roles}
	}
	client := fake.client()

	anonymous, err := client.AnonymousDatasetDecision("public:v1", "/")
	if err != nil || anonymous.Level() != READ {
		t.Fatalf("anonymous decision = %v, err=%v", anonymous, err)
	}
	authenticated, err := client.DatasetDecision("tok-user", "public:v1", "/", false)
	if err != nil || authenticated.Level() != ADMIN {
		t.Fatalf("authenticated decision = %v, err=%v", authenticated, err)
	}
	if fake.authorizeCalls != 2 {
		t.Fatalf("anonymous and authenticated caches must be separate, calls=%d", fake.authorizeCalls)
	}
	if fake.authPresent[0] || fake.authorizations[0] != "" {
		t.Fatalf("anonymous authorize sent Authorization header: present=%v value=%q", fake.authPresent[0], fake.authorizations[0])
	}
	if !fake.authPresent[1] || fake.authorizations[1] != "Bearer tok-user" {
		t.Fatalf("authenticated authorize header = present=%v value=%q", fake.authPresent[1], fake.authorizations[1])
	}

	if _, err := client.AnonymousDatasetDecision("public:v1", "/"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DatasetDecision("tok-user", "public:v1", "/", false); err != nil {
		t.Fatal(err)
	}
	if fake.authorizeCalls != 2 {
		t.Fatalf("repeat decisions should use their independent caches, calls=%d", fake.authorizeCalls)
	}
}

func TestAnonymousDecisionCacheTTLAndTOS(t *testing.T) {
	if got := NewDSGClient("http://dsg.test", 0, "neuprint").CacheTTL; got != 300*time.Second {
		t.Fatalf("default cache TTL = %v, want 300s", got)
	}
	t.Run("public-to-closed revocation waits for configured TTL", func(t *testing.T) {
		fake := newFakeDSG(t)
		fake.decide = func(entry authorizeEntry) DSGDecision {
			decision := "deny"
			roles := []string{}
			if fake.authorizeCalls == 1 {
				decision = "allow"
				roles = []string{"view"}
			}
			return DSGDecision{Name: entry.Name, Decision: decision, Roles: roles}
		}
		client := fake.client()
		client.CacheTTL = 20 * time.Millisecond

		first, err := client.AnonymousDatasetDecision("public", "/")
		if err != nil || first.Level() != READ {
			t.Fatalf("initial public decision = %v, err=%v", first, err)
		}
		beforeExpiry, err := client.AnonymousDatasetDecision("public", "/")
		if err != nil || beforeExpiry.Level() != READ || fake.authorizeCalls != 1 {
			t.Fatalf("before expiry = %v, calls=%d, err=%v", beforeExpiry, fake.authorizeCalls, err)
		}
		time.Sleep(30 * time.Millisecond)
		afterExpiry, err := client.AnonymousDatasetDecision("public", "/")
		if err != nil || afterExpiry.Level() != NOAUTH || fake.authorizeCalls != 2 {
			t.Fatalf("after expiry = %v, calls=%d, err=%v", afterExpiry, fake.authorizeCalls, err)
		}
	})

	t.Run("tos-required is never cached", func(t *testing.T) {
		fake := newFakeDSG(t)
		fake.decide = func(entry authorizeEntry) DSGDecision {
			return DSGDecision{Name: entry.Name, Decision: "tos_required", TOSURL: "https://dsg.test/tos"}
		}
		client := fake.client()
		for range 2 {
			decision, err := client.AnonymousDatasetDecision("tos-public", "/")
			if err != nil || !decision.TOSRequired() {
				t.Fatalf("TOS decision = %v, err=%v", decision, err)
			}
		}
		if fake.authorizeCalls != 2 {
			t.Fatalf("anonymous TOS decision was cached, calls=%d", fake.authorizeCalls)
		}
	})
}

func TestDSGOptionalAuthCredentialStateMatrix(t *testing.T) {
	type setupRequest func(*http.Request)
	tests := []struct {
		name       string
		setup      setupRequest
		status     int
		identified bool
		userCalls  int
	}{
		{"no credential", func(*http.Request) {}, http.StatusOK, false, 0},
		{"good header", func(r *http.Request) { r.Header.Set("Authorization", "Bearer good") }, http.StatusOK, true, 1},
		{"bad header token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer bad") }, http.StatusUnauthorized, false, 1},
		{"empty header", func(r *http.Request) { r.Header["Authorization"] = []string{""} }, http.StatusUnauthorized, false, 0},
		{"empty bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") }, http.StatusUnauthorized, false, 0},
		{"unsupported scheme", func(r *http.Request) { r.Header.Set("Authorization", "Basic good") }, http.StatusUnauthorized, false, 0},
		{"bad header beats good cookie", func(r *http.Request) {
			r.Header.Set("Authorization", "Basic bad")
			r.AddCookie(&http.Cookie{Name: "dsg_token", Value: "good"})
		}, http.StatusUnauthorized, false, 0},
		{"good cookie", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "dsg_token", Value: "good"}) }, http.StatusOK, true, 1},
		{"bad cookie", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "dsg_token", Value: "bad"}) }, http.StatusUnauthorized, false, 1},
		{"empty cookie", func(r *http.Request) { r.Header.Set("Cookie", "dsg_token=") }, http.StatusUnauthorized, false, 0},
		{"good query", func(r *http.Request) { r.URL.RawQuery = "dsg_token=good" }, http.StatusOK, true, 1},
		{"bad query", func(r *http.Request) { r.URL.RawQuery = "dsg_token=bad" }, http.StatusUnauthorized, false, 1},
		{"empty query", func(r *http.Request) { r.URL.RawQuery = "dsg_token=" }, http.StatusUnauthorized, false, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDSG(t)
			fake.decide = func(entry authorizeEntry) DSGDecision {
				return DSGDecision{Name: entry.Name, Decision: "allow", Roles: []string{"view"}}
			}
			if tc.name == "bad header token" || tc.name == "bad cookie" || tc.name == "bad query" {
				fake.identityStatus = http.StatusUnauthorized
			}
			client := fake.client()
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			tc.setup(req)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			called := false
			err := DSGOptionalAuthMiddleware(client, false)(func(c echo.Context) error {
				called = true
				_, identified := c.Get("dsg_identity").(*DSGIdentity)
				if identified != tc.identified {
					t.Fatalf("identified=%v, want %v", identified, tc.identified)
				}
				if c.Get("dsg_client") != client {
					t.Fatal("dsg_client was not installed")
				}
				if identified {
					if c.Get("email") != "test@example.com" {
						t.Fatalf("authenticated email was not installed for logging: %v", c.Get("email"))
					}
					if err := RequireDatasetAccess(c, "permitted", READ); err != nil {
						t.Fatalf("valid transported credential could not query permitted dataset: %v", err)
					}
				}
				return c.NoContent(http.StatusOK)
			})(c)
			if tc.status == http.StatusOK {
				if err != nil || !called || rec.Code != tc.status {
					t.Fatalf("err=%v called=%v status=%d", err, called, rec.Code)
				}
			} else {
				assertHTTPErrorCode(t, err, tc.status)
				if err.(*echo.HTTPError).Message != "invalid or expired token" {
					t.Fatalf("invalid credential message=%v", err.(*echo.HTTPError).Message)
				}
				if called {
					t.Fatal("invalid credential reached handler")
				}
			}
			if fake.userCalls != tc.userCalls {
				t.Fatalf("user calls=%d, want %d", fake.userCalls, tc.userCalls)
			}
		})
	}
}

func TestDSGOptionalAuthDisableAuthAndDefaultOff(t *testing.T) {
	t.Run("disable auth injects synthetic global admin with zero DSG calls", func(t *testing.T) {
		fake := newFakeDSG(t)
		client := fake.client()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/raw/keyvalue/key/x/y", nil)
		req.Header.Set("Authorization", "Basic malformed-is-ignored-in-disable-mode")
		c := e.NewContext(req, httptest.NewRecorder())

		err := DSGOptionalAuthMiddleware(client, true)(func(c echo.Context) error {
			identity := c.Get("dsg_identity").(*DSGIdentity)
			if !identity.Admin || identity.Email != "disable-auth@localhost" {
				t.Fatalf("synthetic identity = %+v", identity)
			}
			return RequireDatasetAccess(c, "closed", ADMIN)
		})(c)
		if err != nil {
			t.Fatalf("disable-auth admin guard failed: %v", err)
		}
		if fake.userCalls != 0 || fake.authorizeCalls != 0 {
			t.Fatalf("disable-auth made DSG calls: user=%d authorize=%d", fake.userCalls, fake.authorizeCalls)
		}
	})

	t.Run("default mode has no anonymous write bypass", func(t *testing.T) {
		fake := newFakeDSG(t)
		fake.decide = func(entry authorizeEntry) DSGDecision {
			return DSGDecision{Name: entry.Name, Decision: "allow", Roles: []string{"view"}}
		}
		client := fake.client()
		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/mutation", nil), httptest.NewRecorder())
		err := DSGOptionalAuthMiddleware(client, false)(func(c echo.Context) error {
			return RequireDatasetAccess(c, "public", ADMIN)
		})(c)
		assertHTTPErrorCode(t, err, http.StatusUnauthorized)
		if fake.authorizeCalls != 1 {
			t.Fatalf("default path bypassed DSG, calls=%d", fake.authorizeCalls)
		}
	})
}

func TestCredentialedFailuresNeverFallbackToAnonymous(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport roundTripFunc
	}{
		{"unreachable", func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") }},
		{"timeout", func(*http.Request) (*http.Response, error) { return nil, contextDeadlineExceeded{} }},
		{"malformed", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("not-json")), Header: make(http.Header)}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewDSGClient("http://dsg.test", 300, "neuprint")
			client.SetHTTPClient(&http.Client{Transport: tc.transport})
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", "Bearer credentialed")
			c := e.NewContext(req, httptest.NewRecorder())
			err := DSGOptionalAuthMiddleware(client, false)(func(echo.Context) error {
				t.Fatal("credentialed DSG failure reached anonymous handler")
				return nil
			})(c)
			assertHTTPErrorCode(t, err, http.StatusBadGateway)
		})
	}
}

type contextDeadlineExceeded struct{}

func (contextDeadlineExceeded) Error() string   { return "deadline exceeded" }
func (contextDeadlineExceeded) Timeout() bool   { return true }
func (contextDeadlineExceeded) Temporary() bool { return true }

func TestAuthenticatedDenyDoesNotRetryAnonymous(t *testing.T) {
	fake := newFakeDSG(t)
	fake.decideRequest = func(entry authorizeEntry, authorization string) DSGDecision {
		if authorization == "" {
			return DSGDecision{Name: entry.Name, Decision: "allow", Roles: []string{"view"}}
		}
		return DSGDecision{Name: entry.Name, Decision: "deny"}
	}
	client := fake.client()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer credentialed")
	c := e.NewContext(req, httptest.NewRecorder())
	err := DSGOptionalAuthMiddleware(client, false)(func(c echo.Context) error {
		return RequireDatasetAccess(c, "closed", READ)
	})(c)
	assertHTTPErrorCode(t, err, http.StatusForbidden)
	if fake.authorizeCalls != 1 || fake.authorizations[0] != "Bearer credentialed" {
		t.Fatalf("authenticated deny retried anonymously: calls=%d headers=%v", fake.authorizeCalls, fake.authorizations)
	}
}

func TestCredentialedMalformedAuthorizeIsBadGatewayWithoutFallback(t *testing.T) {
	authorizeCalls := 0
	client := NewDSGClient("http://dsg.test", 300, "neuprint")
	client.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/dsg/v1/user":
			return jsonHTTPResponse(http.StatusOK, DSGIdentity{ID: 1, Email: "user@example.com"}), nil
		case "/api/dsg/v1/authorize":
			authorizeCalls++
			if r.Header.Get("Authorization") != "Bearer credentialed" {
				t.Fatalf("credentialed authorize downgraded: header=%q", r.Header.Get("Authorization"))
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("not-json")), Header: make(http.Header)}, nil
		default:
			return jsonHTTPResponse(http.StatusNotFound, nil), nil
		}
	})})
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer credentialed")
	c := e.NewContext(req, httptest.NewRecorder())
	err := DSGOptionalAuthMiddleware(client, false)(func(c echo.Context) error {
		return RequireDatasetAccess(c, "dataset", READ)
	})(c)
	assertHTTPErrorCode(t, err, http.StatusBadGateway)
	if authorizeCalls != 1 {
		t.Fatalf("authorize calls=%d, want one credentialed attempt", authorizeCalls)
	}
}

func TestAnonymousDatasetAccessDecisionMatrix(t *testing.T) {
	for _, tc := range []struct {
		name        string
		decision    DSGDecision
		level       AuthorizationLevel
		wantCode    int
		wantTOSBody bool
		wantLevel   string
	}{
		{"public read allowed", DSGDecision{Decision: "allow", Roles: []string{"view"}}, READ, 0, false, "readwrite"},
		{"closed read requires authentication", DSGDecision{Decision: "deny"}, READ, http.StatusUnauthorized, false, ""},
		{"service eval fails closed", DSGDecision{Decision: "service_eval", Roles: []string{"view"}}, READ, http.StatusUnauthorized, false, ""},
		{"public view cannot mutate", DSGDecision{Decision: "allow", Roles: []string{"view"}}, ADMIN, http.StatusUnauthorized, false, ""},
		{"public TOS returns existing JSON", DSGDecision{Decision: "tos_required", TOSURL: "https://dsg.test/tos"}, READ, http.StatusForbidden, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDSG(t)
			fake.decide = func(entry authorizeEntry) DSGDecision {
				decision := tc.decision
				decision.Name = entry.Name
				decision.Version = entry.Version
				return decision
			}
			e := echo.New()
			recorder := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/test", nil), recorder)
			c.Set("dsg_client", fake.client())
			err := RequireDatasetAccess(c, "dataset:v1", tc.level)
			if tc.wantCode == 0 {
				if err != nil || c.Get("level") != tc.wantLevel {
					t.Fatalf("err=%v level=%v", err, c.Get("level"))
				}
			} else if tc.wantTOSBody {
				if err != echo.ErrForbidden || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"tos_required":true`) {
					t.Fatalf("err=%v status=%d body=%s", err, recorder.Code, recorder.Body.String())
				}
			} else {
				assertHTTPErrorCode(t, err, tc.wantCode)
				if httpErr := err.(*echo.HTTPError); httpErr.Message != "authentication required" {
					t.Fatalf("anonymous denial message=%v", httpErr.Message)
				}
			}
		})
	}
}

func TestAnonymousAuthorizeFailuresAreBadGateway(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport roundTripFunc
	}{
		{"unreachable", func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") }},
		{"timeout", func(*http.Request) (*http.Response, error) { return nil, contextDeadlineExceeded{} }},
		{"malformed", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString("not-json")), Header: make(http.Header)}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewDSGClient("http://dsg.test", 300, "neuprint")
			client.SetHTTPClient(&http.Client{Transport: tc.transport})
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/test", nil), httptest.NewRecorder())
			c.Set("dsg_client", client)
			assertHTTPErrorCode(t, RequireDatasetAccess(c, "dataset", READ), http.StatusBadGateway)
		})
	}
}

func TestWriteContract(t *testing.T) {
	for _, tc := range []struct {
		name          string
		identity      DSGIdentity
		roles         []string
		wantCode      int
		wantAuthCalls int
	}{
		{"public view-only user denied", DSGIdentity{Email: "reader@example.com"}, []string{"view"}, http.StatusForbidden, 1},
		{"explicit dataset admin grant succeeds", DSGIdentity{Email: "writer@example.com"}, []string{"admin"}, 0, 1},
		{"global admin exempt", DSGIdentity{Email: "global@example.com", Admin: true}, nil, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDSG(t)
			fake.decide = func(entry authorizeEntry) DSGDecision {
				return DSGDecision{Name: entry.Name, Decision: "allow", Roles: tc.roles}
			}
			client := fake.client()
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodPost, "/api/mutation", nil), httptest.NewRecorder())
			c.Set("dsg_identity", &tc.identity)
			c.Set("dsg_client", client)
			c.Set("dsg_token", "writer-token")
			err := RequireDatasetAccess(c, "public", ADMIN)
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("write unexpectedly denied: %v", err)
				}
			} else {
				assertHTTPErrorCode(t, err, tc.wantCode)
			}
			if fake.authorizeCalls != tc.wantAuthCalls {
				t.Fatalf("authorize calls=%d, want %d", fake.authorizeCalls, tc.wantAuthCalls)
			}
		})
	}
}

func assertHTTPErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP %d error", code)
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != code {
		t.Fatalf("err=%v, want HTTP %d", err, code)
	}
}

func TestDSGAuthMiddleware(t *testing.T) {
	fake := newFakeDSG(t)
	client := fake.client()
	middleware := DSGAuthMiddleware(client)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-user")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := middleware(func(c echo.Context) error {
		if c.Get("dsg_identity").(*DSGIdentity).Email != "test@example.com" {
			t.Fatalf("identity not stored on context")
		}
		if c.Get("dsg_token") != "tok-user" {
			t.Fatalf("token not stored on context")
		}
		if c.Get("email") != "test@example.com" {
			t.Fatalf("email not stored on context")
		}
		return c.NoContent(http.StatusNoContent)
	})

	if err := handler(c); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestDSGAuthMiddlewareUnavailable(t *testing.T) {
	client := NewDSGClient("http://dsg.test", 300, "neuprint")
	client.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})})
	middleware := DSGAuthMiddleware(client)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer tok-user")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := middleware(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})(c)
	if err == nil {
		t.Fatal("expected bad gateway error")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadGateway {
		t.Fatalf("err = %v, want 502 HTTPError", err)
	}
}
