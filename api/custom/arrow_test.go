package custom

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/connectome-neuprint/neuPrintHTTP/secure"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

// MockCypher implements the Cypher interface for testing
type MockCypher struct{}

func (m MockCypher) CypherRequest(cypher string, useJSONNumbers bool) (storage.CypherResult, error) {
	// Return a sample result for testing
	return storage.CypherResult{
		Columns: []string{"id", "name", "count", "active"},
		Data: [][]interface{}{
			{json.Number("1"), "neuron1", json.Number("1000"), true},
			{json.Number("2"), "neuron2", json.Number("2000"), false},
			{json.Number("3"), "neuron3", json.Number("3000"), true},
		},
	}, nil
}

func (m MockCypher) StartTrans() (storage.CypherTransaction, error) {
	return nil, nil
}

// mockStoreImpl implements the storage.Store interface for testing
type mockStoreImpl struct {
	cypherStore MockCypher
}

func (m *mockStoreImpl) GetDataset(dataset string) (storage.Cypher, error) {
	return m.cypherStore, nil
}

func (m *mockStoreImpl) FindStore(storeType, storeName string) (storage.SimpleStore, error) {
	return m, nil
}

func (m *mockStoreImpl) GetMain(datasets ...string) storage.Cypher {
	return m.cypherStore
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
	return map[string]interface{}{"test": true}, nil
}

func (m *mockStoreImpl) GetType() string {
	return "test"
}

func (m *mockStoreImpl) GetInstance() string {
	return "test"
}

// Test the Neo4j to Arrow conversion for primitive types
func TestConvertCypherToArrow(t *testing.T) {
	// Create a sample CypherResult
	result := storage.CypherResult{
		Columns: []string{"id", "name", "count", "active"},
		Data: [][]interface{}{
			{json.Number("1"), "neuron1", json.Number("1000"), true},
			{json.Number("2"), "neuron2", json.Number("2000"), false},
			{json.Number("3"), "neuron3", json.Number("3000"), true},
		},
	}

	// Convert to Arrow
	allocator := memory.NewGoAllocator()
	arrowData, err := ConvertCypherToArrow(result, allocator)
	if err != nil {
		t.Fatalf("Error converting to Arrow: %v", err)
	}

	// Verify schema
	if len(arrowData.Schema.Fields()) != 4 {
		t.Errorf("Expected 4 fields in schema, got %d", len(arrowData.Schema.Fields()))
	}

	// Verify column names
	expectedFields := []string{"id", "name", "count", "active"}
	for i, field := range expectedFields {
		if arrowData.Schema.Field(i).Name != field {
			t.Errorf("Expected field %s at position %d, got %s", field, i, arrowData.Schema.Field(i).Name)
		}
	}

	// Verify record batch count
	if len(arrowData.Records) != 1 {
		t.Errorf("Expected 1 record batch, got %d", len(arrowData.Records))
	}

	// Verify row count
	if arrowData.Records[0].NumRows() != 3 {
		t.Errorf("Expected 3 rows, got %d", arrowData.Records[0].NumRows())
	}
}

// Test Neo4j node conversion to Arrow Map type
func TestConvertNodeMapsToArrow(t *testing.T) {
	// Create a sample CypherResult with node maps
	result := storage.CypherResult{
		Columns: []string{"n", "value"},
		Data: [][]interface{}{
			{map[string]interface{}{
				"bodyId": json.Number("1234"),
				"cellType": "neuron",
				"active": true,
			}, json.Number("100")},
			{map[string]interface{}{
				"bodyId": json.Number("5678"),
				"cellType": "glia",
				"active": false,
			}, json.Number("200")},
			{map[string]interface{}{
				"bodyId": json.Number("9012"),
				"cellType": "neuron",
				// Missing 'active' property - should be handled properly
			}, json.Number("300")},
		},
	}

	// Convert to Arrow
	allocator := memory.NewGoAllocator()
	arrowData, err := ConvertCypherToArrow(result, allocator)
	if err != nil {
		t.Fatalf("Error converting to Arrow: %v", err)
	}

	// Verify schema
	if len(arrowData.Schema.Fields()) != 2 {
		t.Errorf("Expected 2 fields in schema, got %d", len(arrowData.Schema.Fields()))
	}

	// Verify column names
	expectedFields := []string{"n", "value"}
	for i, field := range expectedFields {
		if arrowData.Schema.Field(i).Name != field {
			t.Errorf("Expected field %s at position %d, got %s", field, i, arrowData.Schema.Field(i).Name)
		}
	}

	// Verify the "n" column is a MAP type
	nodeField := arrowData.Schema.Field(0)
	if nodeField.Type.ID() != arrow.MAP {
		t.Errorf("Expected node column to be MAP type, got %v", nodeField.Type.ID())
	}

	// Verify row count
	if arrowData.Records[0].NumRows() != 3 {
		t.Errorf("Expected 3 rows, got %d", arrowData.Records[0].NumRows())
	}

	// Just verify we can access the record
	record := arrowData.Records[0]
	
	// Verify we have the right number of columns
	if record.NumCols() != 2 {
		t.Errorf("Expected 2 columns, got %d", record.NumCols())
	}
	
	// We'll skip detailed inspection of the map column since it requires specific methods
	// that may not be available in all Arrow versions
}

// Test the HTTP Arrow endpoint
func TestHTTPArrowEndpoint(t *testing.T) {
	// Create a new Echo instance
	e := echo.New()
	
	// Create a mock store with a GetDataset function
	mockCypherStore := MockCypher{}
	mockStore := &mockStoreImpl{
		cypherStore: mockCypherStore,
	}
	
	// Create the API
	api := cypherAPI{Store: mockStore}

	// Create a test request with a cypher query
	reqBody := customReq{
		Cypher:  "MATCH (n) RETURN n.id, n.name, n.count, n.active LIMIT 3",
		Dataset: "test",
	}
	jsonBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/custom/arrow", bytes.NewBuffer(jsonBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Set up DSG auth context so RequireDatasetAccess passes
	c.Set("dsg_user", &secure.DSGUserCache{
		Email: "test@example.com",
		Admin: true,
	})
	c.Set("dsg_client", secure.NewDSGClient("http://localhost", 300, nil))

	// Handle the request
	if err := api.getCustomArrow(c); err != nil {
		t.Fatalf("Error handling request: %v", err)
	}

	// Check response status
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Check content type
	contentType := rec.Header().Get(echo.HeaderContentType)
	expectedContentType := "application/vnd.apache.arrow.stream"
	if contentType != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, contentType)
	}

	// Try to decode the Arrow IPC stream
	reader, err := ipc.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("Error creating Arrow IPC reader: %v", err)
	}
	defer reader.Release()

	// Check if schema is valid
	schema := reader.Schema()
	if len(schema.Fields()) != 4 {
		t.Errorf("Expected 4 fields in schema, got %d", len(schema.Fields()))
	}

	// Check if we can read records
	for reader.Next() {
		record := reader.Record()
		if record.NumRows() != 3 {
			t.Errorf("Expected 3 rows, got %d", record.NumRows())
		}
		if record.NumCols() != 4 {
			t.Errorf("Expected 4 columns, got %d", record.NumCols())
		}
		
		// Verify the Arrow data types
		// With our improved type inference, json.Number values should be detected as INT64
		if record.Schema().Field(0).Type.ID() != arrow.INT64 {
			t.Errorf("Expected INT64 type for id column, got %v", record.Schema().Field(0).Type.ID())
		}
		if record.Schema().Field(1).Type.ID() != arrow.STRING {
			t.Errorf("Expected STRING type for name column, got %v", record.Schema().Field(1).Type.ID())
		}
		if record.Schema().Field(2).Type.ID() != arrow.INT64 {
			t.Errorf("Expected INT64 type for count column, got %v", record.Schema().Field(2).Type.ID())
		}
		if record.Schema().Field(3).Type.ID() != arrow.BOOL {
			t.Errorf("Expected BOOL type for active column, got %v", record.Schema().Field(3).Type.ID())
		}
	}

	if err := reader.Err(); err != nil {
		t.Fatalf("Error reading Arrow stream: %v", err)
	}
}