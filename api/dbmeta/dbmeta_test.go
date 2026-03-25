package dbmeta

import (
	"encoding/json"
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

// --- DSG permission filtering tests ---

// mockDSGStore returns a fixed set of datasets for DSG filtering tests.
type mockDSGStore struct{ mockStoreImpl }

func (m *mockDSGStore) GetDatasets() (map[string]interface{}, error) {
	return map[string]interface{}{
		"hemibrain:v1.2.1": map[string]interface{}{
			"last-mod": "2024-06-01",
		},
		"vnc:v1.0": map[string]interface{}{
			"last-mod": "2024-07-01",
		},
		"manc:v1.0": map[string]interface{}{
			"last-mod": "2024-08-01",
		},
	}, nil
}

// helper: create a DSGClient with a dataset map and call getDatasets with the
// given user in context.  Returns the parsed response map.
func callGetDatasetsWithDSG(t *testing.T, user *secure.DSGUserCache, datasetMap map[string]string) map[string]interface{} {
	t.Helper()

	client := secure.NewDSGClient("http://dsg.test", 300, "neuprint", datasetMap)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("dsg_user", user)
	c.Set("dsg_client", client)

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
	return result
}

func TestGetDatasets_DSGFiltersUnauthorized(t *testing.T) {
	// User has view permission on hemibrain only.
	user := &secure.DSGUserCache{
		Email: "test@example.com",
		PermissionsV2: map[string][]string{
			"hemibrain": {"view"},
		},
	}
	// dataset-map: strip version → DSG slug happens automatically
	result := callGetDatasetsWithDSG(t, user, nil)

	if _, ok := result["hemibrain:v1.2.1"]; !ok {
		t.Error("hemibrain:v1.2.1 should be visible")
	}
	if _, ok := result["vnc:v1.0"]; ok {
		t.Error("vnc:v1.0 should be filtered out — user has no vnc permission")
	}
	if _, ok := result["manc:v1.0"]; ok {
		t.Error("manc:v1.0 should be filtered out — user has no manc permission")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 dataset, got %d: %v", len(result), result)
	}
}

func TestGetDatasets_DSGAdminSeesAll(t *testing.T) {
	user := &secure.DSGUserCache{
		Email: "admin@example.com",
		Admin: true,
	}
	result := callGetDatasetsWithDSG(t, user, nil)

	if len(result) != 3 {
		t.Errorf("admin should see all 3 datasets, got %d: %v", len(result), result)
	}
}

func TestGetDatasets_DSGDatasetMapUsed(t *testing.T) {
	// User has permission on DSG slug "VNC" (uppercase).
	// The neuprint DB name is "vnc:v1.0", which strips to "vnc".
	// Without a dataset-map entry, DatasetLevel won't find "VNC".
	// With the map "vnc" → "VNC", it should match.
	user := &secure.DSGUserCache{
		Email: "test@example.com",
		PermissionsV2: map[string][]string{
			"VNC": {"view"},
		},
	}

	// Without dataset-map: vnc should be filtered out
	result := callGetDatasetsWithDSG(t, user, nil)
	if _, ok := result["vnc:v1.0"]; ok {
		t.Error("without dataset-map, vnc:v1.0 should be filtered (slug 'vnc' != 'VNC')")
	}

	// With dataset-map: vnc → VNC — should now be visible
	result = callGetDatasetsWithDSG(t, user, map[string]string{"vnc": "VNC"})
	if _, ok := result["vnc:v1.0"]; !ok {
		t.Error("with dataset-map vnc→VNC, vnc:v1.0 should be visible")
	}
}

func TestGetDatasets_NoDSGContext_NoFiltering(t *testing.T) {
	// When dsg_user/dsg_client are absent (auth disabled), all datasets pass.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/dbmeta/datasets", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// deliberately NOT setting dsg_user or dsg_client

	api := storeAPI{Store: &mockDSGStore{}}
	if err := api.getDatasets(c); err != nil {
		t.Fatalf("getDatasets error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("with no DSG context, all 3 datasets should be returned, got %d", len(result))
	}
}

func TestGetDatasets_DSGMultiplePermissions(t *testing.T) {
	// User has access to hemibrain and manc but not vnc — mirrors the
	// reported bug scenario.
	user := &secure.DSGUserCache{
		Email: "wtkatz@alumni.stanford.edu",
		PermissionsV2: map[string][]string{
			"hemibrain": {"view"},
			"MANC":      {"view"},
		},
	}
	datasetMap := map[string]string{"manc": "MANC"}
	result := callGetDatasetsWithDSG(t, user, datasetMap)

	if _, ok := result["hemibrain:v1.2.1"]; !ok {
		t.Error("hemibrain should be visible")
	}
	if _, ok := result["manc:v1.0"]; !ok {
		t.Error("manc should be visible via dataset-map → MANC")
	}
	if _, ok := result["vnc:v1.0"]; ok {
		t.Error("vnc should NOT be visible — user has no vnc permission")
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