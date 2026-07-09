package secure

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
	userCalls      int
	authorizeCalls int
	bodies         []authorizeRequest
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
			var req authorizeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				return nil, err
			}
			f.bodies = append(f.bodies, req)
			resp := authorizeResponse{Entries: make([]DSGDecision, 0, len(req.Entries))}
			for _, entry := range req.Entries {
				resp.Entries = append(resp.Entries, f.decide(entry))
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
