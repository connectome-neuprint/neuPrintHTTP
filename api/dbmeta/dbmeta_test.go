package dbmeta

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

// Test error handling for invalid hidden field type
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

	// Handle the request - should return error
	err := api.getDatasets(c)
	if err == nil {
		t.Error("Expected error for invalid hidden field type, got nil")
	}

	expectedErrorMsg := "hidden field for dataset bad_hidden_dataset is not a boolean"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
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

// Test error handling for non-map dataset info
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

	// Handle the request - should return error
	err := api.getDatasets(c)
	if err == nil {
		t.Error("Expected error for non-map dataset info, got nil")
	}

	expectedErrorMsg := "dataset info for bad_dataset is not a map"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}