package dbmeta

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/connectome-neuprint/neuPrintHTTP/secure"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

// mockStoreImpl implements the storage.Store interface for testing
type mockStoreImpl struct{}

func (m *mockStoreImpl) GetDataset(dataset string) (storage.Cypher, error) {
	return nil, nil
}

func (m *mockStoreImpl) FindStore(storeType, storeName string) (storage.SimpleStore, error) {
	return m, nil
}

func (m *mockStoreImpl) GetMain(datasets ...string) storage.Cypher {
	return nil
}

func (m *mockStoreImpl) GetStores() []storage.SimpleStore {
	return []storage.SimpleStore{m}
}

func (m *mockStoreImpl) GetInstances() map[string]storage.SimpleStore {
	return map[string]storage.SimpleStore{"test": m}
}

func (m *mockStoreImpl) GetTypes() map[string][]storage.SimpleStore {
	return map[string][]storage.SimpleStore{"test": {m}}
}

// Implement SimpleStore interface
func (m *mockStoreImpl) GetVersion() (string, error) {
	return "1.0.0", nil
}

func (m *mockStoreImpl) GetDatabase() (string, string, error) {
	return "test", "1.0.0", nil
}

func (m *mockStoreImpl) GetDatasets() (map[string]interface{}, error) {
	// Return test datasets with mixed hidden status
	return map[string]interface{}{
		"visible_dataset": map[string]interface{}{
			"last-mod":    "2024-01-01",
			"uuid":        "abc123",
			"ROIs":        []string{"roi1", "roi2"},
			"hidden":      false,
			"description": "Visible dataset",
		},
		"hidden_dataset": map[string]interface{}{
			"last-mod":    "2024-01-02",
			"uuid":        "def456",
			"ROIs":        []string{"roi3", "roi4"},
			"hidden":      true,
			"description": "Hidden dataset",
		},
		"no_hidden_field": map[string]interface{}{
			"last-mod":    "2024-01-03",
			"uuid":        "ghi789",
			"ROIs":        []string{"roi5"},
			"description": "Dataset without hidden field",
		},
	}, nil
}

func (m *mockStoreImpl) GetType() string {
	return "test"
}

func (m *mockStoreImpl) GetInstance() string {
	return "test"
}

// Test getDatasets without hidden parameter (should exclude hidden datasets)
func TestGetDatasets_WithoutHiddenParam(t *testing.T) {
	// Create Echo instance
	e := echo.New()

	// Create mock store
	mockStore := &mockStoreImpl{}

	// Create API instance
	api := storeAPI{Store: mockStore}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Handle the request
	if err := api.getDatasets(c); err != nil {
		t.Fatalf("Error handling request: %v", err)
	}

	// Check response status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Error parsing response: %v", err)
	}

	// Should contain visible_dataset and no_hidden_field, but not hidden_dataset
	if _, exists := result["visible_dataset"]; !exists {
		t.Error("Expected visible_dataset to be included")
	}
	if _, exists := result["no_hidden_field"]; !exists {
		t.Error("Expected no_hidden_field dataset to be included")
	}
	if _, exists := result["hidden_dataset"]; exists {
		t.Error("Expected hidden_dataset to be excluded")
	}

	// Should have exactly 2 datasets
	if len(result) != 2 {
		t.Errorf("Expected 2 datasets, got %d", len(result))
	}
}

// Test getDatasets with hidden=true (should include all datasets)
func TestGetDatasets_WithHiddenTrue(t *testing.T) {
	// Create Echo instance
	e := echo.New()

	// Create mock store
	mockStore := &mockStoreImpl{}

	// Create API instance
	api := storeAPI{Store: mockStore}

	// Create request with hidden=true parameter
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets?hidden=true", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Handle the request
	if err := api.getDatasets(c); err != nil {
		t.Fatalf("Error handling request: %v", err)
	}

	// Check response status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Error parsing response: %v", err)
	}

	// Should contain all datasets
	if _, exists := result["visible_dataset"]; !exists {
		t.Error("Expected visible_dataset to be included")
	}
	if _, exists := result["hidden_dataset"]; !exists {
		t.Error("Expected hidden_dataset to be included")
	}
	if _, exists := result["no_hidden_field"]; !exists {
		t.Error("Expected no_hidden_field dataset to be included")
	}

	// Should have exactly 3 datasets
	if len(result) != 3 {
		t.Errorf("Expected 3 datasets, got %d", len(result))
	}
}

// Test getDatasets with hidden=false (should exclude hidden datasets)
func TestGetDatasets_WithHiddenFalse(t *testing.T) {
	// Create Echo instance
	e := echo.New()

	// Create mock store
	mockStore := &mockStoreImpl{}

	// Create API instance
	api := storeAPI{Store: mockStore}

	// Create request with hidden=false parameter
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets?hidden=false", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Handle the request
	if err := api.getDatasets(c); err != nil {
		t.Fatalf("Error handling request: %v", err)
	}

	// Check response status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("Error parsing response: %v", err)
	}

	// Should contain visible_dataset and no_hidden_field, but not hidden_dataset
	if _, exists := result["visible_dataset"]; !exists {
		t.Error("Expected visible_dataset to be included")
	}
	if _, exists := result["no_hidden_field"]; !exists {
		t.Error("Expected no_hidden_field dataset to be included")
	}
	if _, exists := result["hidden_dataset"]; exists {
		t.Error("Expected hidden_dataset to be excluded")
	}

	// Should have exactly 2 datasets
	if len(result) != 2 {
		t.Errorf("Expected 2 datasets, got %d", len(result))
	}
}

// mockStoreWithBadHiddenField implements a store that returns invalid hidden field type
type mockStoreWithBadHiddenField struct{}

func (m *mockStoreWithBadHiddenField) GetDataset(dataset string) (storage.Cypher, error) {
	return nil, nil
}

func (m *mockStoreWithBadHiddenField) FindStore(storeType, storeName string) (storage.SimpleStore, error) {
	return m, nil
}

func (m *mockStoreWithBadHiddenField) GetMain(datasets ...string) storage.Cypher {
	return nil
}

func (m *mockStoreWithBadHiddenField) GetStores() []storage.SimpleStore {
	return []storage.SimpleStore{m}
}

func (m *mockStoreWithBadHiddenField) GetInstances() map[string]storage.SimpleStore {
	return map[string]storage.SimpleStore{"test": m}
}

func (m *mockStoreWithBadHiddenField) GetTypes() map[string][]storage.SimpleStore {
	return map[string][]storage.SimpleStore{"test": {m}}
}

func (m *mockStoreWithBadHiddenField) GetVersion() (string, error) {
	return "1.0.0", nil
}

func (m *mockStoreWithBadHiddenField) GetDatabase() (string, string, error) {
	return "test", "1.0.0", nil
}

func (m *mockStoreWithBadHiddenField) GetDatasets() (map[string]interface{}, error) {
	// Return dataset with invalid hidden field type
	return map[string]interface{}{
		"bad_hidden_dataset": map[string]interface{}{
			"last-mod": "2024-01-01",
			"uuid":     "abc123",
			"ROIs":     []string{"roi1"},
			"hidden":   "not_a_boolean", // This should cause an error
		},
	}, nil
}

func (m *mockStoreWithBadHiddenField) GetType() string {
	return "test"
}

func (m *mockStoreWithBadHiddenField) GetInstance() string {
	return "test"
}

// Test graceful handling of invalid hidden field type
func TestGetDatasets_BadHiddenFieldType(t *testing.T) {
	// Create Echo instance
	e := echo.New()

	// Create mock store with bad hidden field
	mockStore := &mockStoreWithBadHiddenField{}

	// Create API instance
	api := storeAPI{Store: mockStore}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Handle the request - should succeed with warning logged
	err := api.getDatasets(c)
	if err != nil {
		t.Errorf("Expected no error for invalid hidden field type, got: %v", err)
	}

	// Check that response is successful
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response and verify dataset is included (default behavior)
	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, exists := response["bad_hidden_dataset"]; !exists {
		t.Error("Expected bad_hidden_dataset to be included in response")
	}
}

// mockStoreWithBadDatasetInfo implements a store that returns non-map dataset info
type mockStoreWithBadDatasetInfo struct{}

func (m *mockStoreWithBadDatasetInfo) GetDataset(dataset string) (storage.Cypher, error) {
	return nil, nil
}

func (m *mockStoreWithBadDatasetInfo) FindStore(storeType, storeName string) (storage.SimpleStore, error) {
	return m, nil
}

func (m *mockStoreWithBadDatasetInfo) GetMain(datasets ...string) storage.Cypher {
	return nil
}

func (m *mockStoreWithBadDatasetInfo) GetStores() []storage.SimpleStore {
	return []storage.SimpleStore{m}
}

func (m *mockStoreWithBadDatasetInfo) GetInstances() map[string]storage.SimpleStore {
	return map[string]storage.SimpleStore{"test": m}
}

func (m *mockStoreWithBadDatasetInfo) GetTypes() map[string][]storage.SimpleStore {
	return map[string][]storage.SimpleStore{"test": {m}}
}

func (m *mockStoreWithBadDatasetInfo) GetVersion() (string, error) {
	return "1.0.0", nil
}

func (m *mockStoreWithBadDatasetInfo) GetDatabase() (string, string, error) {
	return "test", "1.0.0", nil
}

func (m *mockStoreWithBadDatasetInfo) GetDatasets() (map[string]interface{}, error) {
	// Return dataset info that's not a map
	return map[string]interface{}{
		"bad_dataset": "this_is_not_a_map",
	}, nil
}

func (m *mockStoreWithBadDatasetInfo) GetType() string {
	return "test"
}

func (m *mockStoreWithBadDatasetInfo) GetInstance() string {
	return "test"
}

// --- DSG decision filtering tests ---

// mockDSGStore returns a fixed set of datasets for DSG filtering tests.
type mockDSGStore struct{ mockStoreImpl }

func (m *mockDSGStore) GetDatasets() (map[string]interface{}, error) {
	return map[string]interface{}{
		"hemibrain:v1.2.1": map[string]interface{}{
			"last-mod": "2024-06-01",
		},
		"public:v1": map[string]interface{}{
			"last-mod": "2024-07-01",
		},
		"tos:v1": map[string]interface{}{
			"last-mod": "2024-08-01",
		},
		"denied:v1": map[string]interface{}{
			"last-mod": "2024-09-01",
		},
		"closed-public-version:v1": map[string]interface{}{
			"last-mod": "2024-10-01",
		},
		"sa-granted:v1": map[string]interface{}{
			"last-mod": "2024-11-01",
		},
	}, nil
}

type dbmetaRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dbmetaRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func dbmetaJSONHTTPResponse(status int, body interface{}) *http.Response {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(&buf),
	}
}

func callGetDatasetsWithDSG(
	t *testing.T,
	identity *secure.DSGIdentity,
	token string,
) (map[string]interface{}, int) {
	t.Helper()

	authorizeCalls := 0
	transport := dbmetaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/dsg/v1/authorize" {
			return dbmetaJSONHTTPResponse(http.StatusNotFound, map[string]string{"error": "not found"}), nil
		}
		authorizeCalls++
		var body struct {
			Entries []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		bearer := r.Header.Get("Authorization")
		resp := struct {
			Entries []map[string]interface{} `json:"entries"`
		}{Entries: make([]map[string]interface{}, 0, len(body.Entries))}
		for _, entry := range body.Entries {
			decision := map[string]interface{}{
				"name":     entry.Name,
				"version":  entry.Version,
				"decision": "deny",
				"roles":    []string{},
			}
			if bearer == "Bearer sa-token" {
				if entry.Name == "sa-granted" {
					decision["decision"] = "allow"
					decision["roles"] = []string{"view"}
				}
			} else {
				switch entry.Name {
				case "hemibrain", "public":
					decision["decision"] = "allow"
					decision["roles"] = []string{"view"}
				case "tos":
					decision["decision"] = "tos_required"
					decision["roles"] = []string{"view"}
					decision["tos_url"] = "https://dsg.example.com/tos"
				}
			}
			resp.Entries = append(resp.Entries, decision)
		}
		return dbmetaJSONHTTPResponse(http.StatusOK, resp), nil
	})

	client := secure.NewDSGClient("http://dsg.test", 300, "neuprint")
	client.SetHTTPClient(&http.Client{Transport: transport})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if identity != nil {
		c.Set("dsg_identity", identity)
		c.Set("dsg_client", client)
		c.Set("dsg_token", token)
	}

	api := storeAPI{Store: &mockDSGStore{}}
	if err := api.getDatasets(c); err != nil {
		t.Fatalf("getDatasets error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	return result, authorizeCalls
}

func TestGetDatasets_NativeDecisionsFilterDropdown(t *testing.T) {
	result, calls := callGetDatasetsWithDSG(
		t, &secure.DSGIdentity{Email: "test@example.com"}, "user-token",
	)
	if calls != 1 {
		t.Fatalf("expected one batch authorize call, got %d", calls)
	}

	if _, ok := result["hemibrain:v1.2.1"]; !ok {
		t.Error("hemibrain:v1.2.1 should be visible")
	}
	if _, ok := result["public:v1"]; !ok {
		t.Error("public:v1 should be visible via native allow")
	}
	if _, ok := result["tos:v1"]; !ok {
		t.Error("tos:v1 should be visible while TOS is pending")
	}
	if _, ok := result["denied:v1"]; ok {
		t.Error("denied:v1 should be filtered out")
	}
	if _, ok := result["closed-public-version:v1"]; ok {
		t.Error("closed-public-version:v1 should be filtered out")
	}
	if _, ok := result["sa-granted:v1"]; ok {
		t.Error("sa-granted:v1 should be filtered out for the human token")
	}
}

func TestGetDatasets_NativeServiceAccountSeesOnlyGranted(t *testing.T) {
	result, calls := callGetDatasetsWithDSG(
		t,
		&secure.DSGIdentity{Email: "", ServiceAccount: true},
		"sa-token",
	)
	if calls != 1 {
		t.Fatalf("expected one batch authorize call, got %d", calls)
	}

	if _, ok := result["sa-granted:v1"]; !ok {
		t.Error("sa-granted:v1 should be visible to the service account token")
	}
	if len(result) != 1 {
		t.Errorf("service account should see exactly one explicit grant, got %d: %v", len(result), result)
	}
}

func TestGetDatasets_NativeAdminSeesAll(t *testing.T) {
	result, calls := callGetDatasetsWithDSG(
		t, &secure.DSGIdentity{Email: "admin@example.com", Admin: true}, "admin-token",
	)
	if calls != 0 {
		t.Fatalf("admin dropdown should not call authorize, got %d calls", calls)
	}

	if len(result) != 6 {
		t.Errorf("admin should see all 6 datasets, got %d: %v", len(result), result)
	}
}

func TestGetDatasets_NoDSGContext_NoFiltering(t *testing.T) {
	result, calls := callGetDatasetsWithDSG(t, nil, "")

	if calls != 0 {
		t.Fatalf("auth-disabled path should not call authorize, got %d calls", calls)
	}
	if len(result) != 6 {
		t.Errorf("with no DSG context, all 6 datasets should be returned, got %d", len(result))
	}
}

// Test graceful handling of non-map dataset info
func TestGetDatasets_BadDatasetInfoType(t *testing.T) {
	// Create Echo instance
	e := echo.New()

	// Create mock store with bad dataset info
	mockStore := &mockStoreWithBadDatasetInfo{}

	// Create API instance
	api := storeAPI{Store: mockStore}

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Handle the request - should succeed with warning logged
	err := api.getDatasets(c)
	if err != nil {
		t.Errorf("Expected no error for non-map dataset info, got: %v", err)
	}

	// Check that response is successful
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response and verify dataset is included (default behavior)
	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if _, exists := response["bad_dataset"]; !exists {
		t.Error("Expected bad_dataset to be included in response")
	}
}
