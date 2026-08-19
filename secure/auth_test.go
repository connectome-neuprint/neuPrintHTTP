package secure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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
