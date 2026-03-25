package secure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
)

// loginRedirectCase drives a single call to dsgLoginHandler and checks the
// resulting Location header for the expected redirect and DSG query params.
type loginRedirectCase struct {
	name string
	// Query string sent to /login (keys: redirect, dataset)
	query url.Values
	// DSG config
	dsgURL      string
	serviceName string
	// Checks against the Location header returned by the handler
	wantRedirectContains string // substring that must appear inside the redirect= value sent to DSG
	wantDatasetParam     string // top-level &dataset= value sent to DSG (empty ⇒ absent)
	wantServiceParam     string // top-level &service= value sent to DSG (empty ⇒ absent)
}

func TestDsgLoginHandler_RedirectURL(t *testing.T) {
	cases := []loginRedirectCase{
		// ---- basic: no dataset, no redirect ----
		{
			name:                 "bare login defaults to /",
			query:                url.Values{},
			dsgURL:               "https://dsg.example.com",
			wantRedirectContains: "http://neuprint.example.com/",
		},
		// ---- dataset injected into redirect path ----
		{
			name: "dataset folded into bare redirect",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "dataset=hemibrain",
			wantDatasetParam:     "hemibrain",
			wantServiceParam:     "neuprint",
		},
		// ---- dataset with version colon ----
		{
			name: "dataset with colon version is preserved",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain:v1.2.1"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "hemibrain",
			wantDatasetParam:     "hemibrain:v1.2.1",
			wantServiceParam:     "neuprint",
		},
		// ---- redirect already carries dataset → no duplication ----
		{
			name: "no duplication when redirect already has dataset",
			query: url.Values{
				"redirect": {"/?dataset=hemibrain"},
				"dataset":  {"hemibrain"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "dataset=hemibrain",
			wantDatasetParam:     "hemibrain",
			wantServiceParam:     "neuprint",
		},
		// ---- redirect with existing query params (tab, etc.) ----
		{
			name: "existing query params preserved alongside dataset",
			query: url.Values{
				"redirect": {"/?tab=graph"},
				"dataset":  {"manc:v1.0"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "tab=graph",
			wantDatasetParam:     "manc:v1.0",
			wantServiceParam:     "neuprint",
		},
		// ---- no service name configured ----
		{
			name: "service param omitted when serviceName is empty",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain"},
			},
			dsgURL:           "https://dsg.example.com",
			serviceName:      "",
			wantDatasetParam: "hemibrain",
		},
		// ---- no dataset → pure login, no service/dataset params ----
		{
			name: "pure login omits service and dataset",
			query: url.Values{
				"redirect": {"/results"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "/results",
		},
		// ---- redirect path is empty string ----
		{
			name: "empty redirect string defaults to /",
			query: url.Values{
				"redirect": {""},
				"dataset":  {"optic-lobe:v1.0"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "dataset=optic-lobe",
			wantDatasetParam:     "optic-lobe:v1.0",
			wantServiceParam:     "neuprint",
		},
		// ---- deep path with fragment-like suffix ----
		{
			name: "deep redirect path preserved",
			query: url.Values{
				"redirect": {"/results?qr=1&tab=graph"},
				"dataset":  {"hemibrain"},
			},
			dsgURL:               "https://dsg.example.com",
			serviceName:          "neuprint",
			wantRedirectContains: "qr=1",
			wantDatasetParam:     "hemibrain",
			wantServiceParam:     "neuprint",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := dsgLoginHandler(tc.dsgURL, tc.serviceName)

			e := echo.New()
			target := "/login?" + tc.query.Encode()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Host = "neuprint.example.com"
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler(c)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}

			location := rec.Header().Get("Location")
			if location == "" {
				t.Fatal("missing Location header")
			}

			locURL, err := url.Parse(location)
			if err != nil {
				t.Fatalf("unparseable Location: %v", err)
			}

			// The Location points at DSG's /api/v1/authorize
			if got := locURL.Path; got != "/api/v1/authorize" {
				t.Errorf("DSG path = %q, want /api/v1/authorize", got)
			}

			locQuery := locURL.Query()

			// ---- check redirect= value ----
			redirectVal := locQuery.Get("redirect")
			if redirectVal == "" {
				t.Fatal("redirect param missing from DSG URL")
			}
			if tc.wantRedirectContains != "" {
				if !containsSubstring(redirectVal, tc.wantRedirectContains) {
					t.Errorf("redirect=%q does not contain %q", redirectVal, tc.wantRedirectContains)
				}
			}

			// ---- check dataset= top-level param ----
			gotDataset := locQuery.Get("dataset")
			if gotDataset != tc.wantDatasetParam {
				t.Errorf("dataset param = %q, want %q", gotDataset, tc.wantDatasetParam)
			}

			// ---- check service= top-level param ----
			gotService := locQuery.Get("service")
			if gotService != tc.wantServiceParam {
				t.Errorf("service param = %q, want %q", gotService, tc.wantServiceParam)
			}
		})
	}
}

// TestDsgLoginHandler_DatasetInRedirectValue specifically verifies that when a
// dataset is provided, the *redirect URL itself* (not just the DSG query
// string) contains dataset= so the user lands on the correct page after TOS.
func TestDsgLoginHandler_DatasetInRedirectValue(t *testing.T) {
	cases := []struct {
		name     string
		query    url.Values
		wantInRedirect   bool   // expect dataset= inside the redirect URL
		wantDatasetValue string // expected value of dataset inside redirect URL
	}{
		{
			name: "dataset appended to bare /",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain"},
			},
			wantInRedirect:   true,
			wantDatasetValue: "hemibrain",
		},
		{
			name: "dataset not duplicated when already present",
			query: url.Values{
				"redirect": {"/?dataset=hemibrain"},
				"dataset":  {"hemibrain"},
			},
			wantInRedirect:   true,
			wantDatasetValue: "hemibrain",
		},
		{
			name: "colon-versioned dataset survives round-trip encoding",
			query: url.Values{
				"redirect": {"/"},
				"dataset":  {"hemibrain:v1.2.1"},
			},
			wantInRedirect:   true,
			wantDatasetValue: "hemibrain:v1.2.1",
		},
		{
			name: "no dataset param → nothing injected",
			query: url.Values{
				"redirect": {"/foo?bar=1"},
			},
			wantInRedirect: false,
		},
		{
			name: "multiple existing params preserved",
			query: url.Values{
				"redirect": {"/?tab=graph&qr=1"},
				"dataset":  {"manc:v1.0"},
			},
			wantInRedirect:   true,
			wantDatasetValue: "manc:v1.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := dsgLoginHandler("https://dsg.example.com", "neuprint")

			e := echo.New()
			target := "/login?" + tc.query.Encode()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Host = "neuprint.example.com"
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := handler(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}

			location := rec.Header().Get("Location")
			locURL, err := url.Parse(location)
			if err != nil {
				t.Fatalf("bad Location: %v", err)
			}

			// Extract the redirect= value and parse it as a URL
			redirectRaw := locURL.Query().Get("redirect")
			redirectURL, err := url.Parse(redirectRaw)
			if err != nil {
				t.Fatalf("redirect value is not a valid URL: %v", err)
			}

			gotDataset := redirectURL.Query().Get("dataset")
			if tc.wantInRedirect {
				if gotDataset != tc.wantDatasetValue {
					t.Errorf("dataset inside redirect URL = %q, want %q\n  full redirect = %s",
						gotDataset, tc.wantDatasetValue, redirectRaw)
				}
				// Verify no duplication: dataset should appear exactly once
				vals := redirectURL.Query()["dataset"]
				if len(vals) > 1 {
					t.Errorf("dataset duplicated in redirect URL: %v", vals)
				}
			} else {
				if gotDataset != "" {
					t.Errorf("did not expect dataset in redirect URL, got %q", gotDataset)
				}
			}
		})
	}
}

// --- RequireDatasetAccess and dsgDatasetAccessHandler error message tests ---

func TestRequireDatasetAccess_ErrorIncludesDatasetName(t *testing.T) {
	client := NewDSGClient("http://dsg.test", 300, "neuprint", nil)
	user := &DSGUserCache{
		Email:         "test@example.com",
		PermissionsV2: map[string][]string{},
	}

	cases := []struct {
		name    string
		dataset string
	}{
		{"simple name", "hemibrain"},
		{"versioned name", "vnc:v1.0"},
		{"uppercase", "VNC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/custom/custom", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("dsg_user", user)
			c.Set("dsg_client", client)

			err := RequireDatasetAccess(c, tc.dataset, READ)
			if err == nil {
				t.Fatal("expected error for unauthorized dataset")
			}
			httpErr, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("expected echo.HTTPError, got %T", err)
			}
			msg, _ := httpErr.Message.(string)
			if !containsSubstring(msg, tc.dataset) {
				t.Errorf("error message %q should contain dataset name %q", msg, tc.dataset)
			}
		})
	}
}

func TestDsgDatasetAccessHandler_ErrorIncludesDatasetName(t *testing.T) {
	client := NewDSGClient("http://dsg.test", 300, "neuprint", nil)
	user := &DSGUserCache{
		Email:         "test@example.com",
		PermissionsV2: map[string][]string{},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/dataset-access?dataset=VNC", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_user", user)
	c.Set("dsg_client", client)

	err := dsgDatasetAccessHandler(c)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	msg, _ := body["message"].(string)
	if !containsSubstring(msg, "VNC") {
		t.Errorf("message %q should contain dataset name 'VNC'", msg)
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
