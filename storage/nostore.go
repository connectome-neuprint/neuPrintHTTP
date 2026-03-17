package storage

import "fmt"

// NoStore is a stub Store that requires no backend database.
// It allows neuPrintHTTP to start for testing auth flows, frontend
// development, etc. without needing a running neo4j instance.
type NoStore struct {
	Datasets []string // mock dataset names to advertise
}

func (n *NoStore) GetVersion() (string, error) {
	return "no-data", nil
}

func (n *NoStore) GetDatabase() (string, string, error) {
	return "no-data", "no-data", nil
}

func (n *NoStore) GetDatasets() (map[string]interface{}, error) {
	ds := make(map[string]interface{})
	for _, name := range n.Datasets {
		ds[name] = map[string]interface{}{
			"ROIs":           []string{},
			"superLevelROIs": []string{},
			"uuid":           "no-data",
			"last-mod":       "2026-01-01",
			"description":    "Mock dataset for local development",
		}
	}
	return ds, nil
}

func (n *NoStore) GetType() string {
	return "nostore"
}

func (n *NoStore) GetInstance() string {
	return "nostore"
}

func (n *NoStore) GetMain(datasets ...string) Cypher {
	return &noopCypher{}
}

func (n *NoStore) GetDataset(dataset string) (Cypher, error) {
	for _, name := range n.Datasets {
		if name == dataset {
			return &noopCypher{}, nil
		}
	}
	return nil, fmt.Errorf("dataset %q not available (running in no-data mode)", dataset)
}

func (n *NoStore) GetStores() []SimpleStore {
	return nil
}

func (n *NoStore) GetInstances() map[string]SimpleStore {
	return map[string]SimpleStore{}
}

func (n *NoStore) GetTypes() map[string][]SimpleStore {
	return map[string][]SimpleStore{}
}

func (n *NoStore) FindStore(typename string, dataset string) (SimpleStore, error) {
	return nil, fmt.Errorf("no stores available (running in no-data mode)")
}

// noopCypher returns empty results for any query.
type noopCypher struct{}

func (c *noopCypher) CypherRequest(query string, readonly bool) (CypherResult, error) {
	return CypherResult{Columns: []string{}, Data: [][]interface{}{}}, nil
}

func (c *noopCypher) StartTrans() (CypherTransaction, error) {
	return &noopTransaction{}, nil
}

type noopTransaction struct{}

func (t *noopTransaction) CypherRequest(query string, readonly bool) (CypherResult, error) {
	return CypherResult{Columns: []string{}, Data: [][]interface{}{}}, nil
}

func (t *noopTransaction) Kill() error  { return nil }
func (t *noopTransaction) Commit() error { return nil }
