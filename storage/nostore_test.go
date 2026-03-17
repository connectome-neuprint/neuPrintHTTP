package storage

import "testing"

func TestNoStoreEmpty(t *testing.T) {
	store := &NoStore{}

	datasets, err := store.GetDatasets()
	if err != nil {
		t.Fatalf("GetDatasets returned error: %v", err)
	}
	if len(datasets) != 0 {
		t.Errorf("expected 0 datasets, got %d", len(datasets))
	}

	version, err := store.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if version != "no-data" {
		t.Errorf("expected version 'no-data', got %q", version)
	}

	_, err = store.GetDataset("anything")
	if err == nil {
		t.Error("expected error from GetDataset on empty NoStore")
	}
}

func TestNoStoreWithMockDatasets(t *testing.T) {
	store := &NoStore{Datasets: []string{"fish2", "hemibrain"}}

	datasets, err := store.GetDatasets()
	if err != nil {
		t.Fatalf("GetDatasets returned error: %v", err)
	}
	if len(datasets) != 2 {
		t.Fatalf("expected 2 datasets, got %d", len(datasets))
	}
	if _, ok := datasets["fish2"]; !ok {
		t.Error("expected 'fish2' in datasets")
	}
	if _, ok := datasets["hemibrain"]; !ok {
		t.Error("expected 'hemibrain' in datasets")
	}

	// Known dataset should return a cypher
	cypher, err := store.GetDataset("fish2")
	if err != nil {
		t.Fatalf("GetDataset('fish2') returned error: %v", err)
	}
	if cypher == nil {
		t.Fatal("expected non-nil cypher for 'fish2'")
	}

	// Unknown dataset should error
	_, err = store.GetDataset("unknown")
	if err == nil {
		t.Error("expected error from GetDataset for unknown dataset")
	}
}

func TestNoopCypherReturnsEmpty(t *testing.T) {
	store := &NoStore{Datasets: []string{"test"}}
	cypher := store.GetMain("test")

	result, err := cypher.CypherRequest("MATCH (n) RETURN n", true)
	if err != nil {
		t.Fatalf("CypherRequest returned error: %v", err)
	}
	if len(result.Columns) != 0 {
		t.Errorf("expected 0 columns, got %d", len(result.Columns))
	}
	if len(result.Data) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Data))
	}
}

func TestNoopTransaction(t *testing.T) {
	store := &NoStore{Datasets: []string{"test"}}
	cypher := store.GetMain("test")

	tx, err := cypher.StartTrans()
	if err != nil {
		t.Fatalf("StartTrans returned error: %v", err)
	}

	result, err := tx.CypherRequest("MATCH (n) RETURN n", true)
	if err != nil {
		t.Fatalf("transaction CypherRequest returned error: %v", err)
	}
	if len(result.Data) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Data))
	}

	if err := tx.Commit(); err != nil {
		t.Errorf("Commit returned error: %v", err)
	}
}
