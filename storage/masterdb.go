package storage

import (
	"fmt"
	"strings"
)

// MasterDB implements the Store interface
type MasterDB struct {
	// MainStores contains all graph DBs
	// (first store is the default)
	MainStores    []SimpleStore
	DatasetStores map[string]SimpleStore
	Stores        []SimpleStore
	Instances     map[string]SimpleStore
	Types         map[string][]SimpleStore
}

// MaiinStore implements the Cypher interfacee
// and is responsible for automatically modifying cypher
// TODO: support multiple databases (concatenation) and optional no cypher overwrite
type CypherWrapper struct {
	dataset   string // just store one for now
	mainStore Cypher
}

func (cw *CypherWrapper) CypherRequest(query string, readonly bool) (CypherResult, error) {
	// if a dataset is provided, add dataset keyword in queries
	if cw.dataset != "" {
		// extract root dataset name
		vals := strings.Split(cw.dataset, ":")
		dataset := vals[0]

		// handle SynapsesTo exception
		query = strings.Replace(query, ":SynapsesTo", ":XSynapsesTo", -1)

		// replace keywords with dataset info
		query = strings.Replace(query, ":Neuron", ":`"+dataset+"_Neuron`", -1)
		query = strings.Replace(query, ":Segment", ":`"+dataset+"_Segment`", -1)
		query = strings.Replace(query, ":Meta", ":`"+dataset+"_Meta`", -1)
		query = strings.Replace(query, ":SynapseSet", ":`"+dataset+"_SynapseSet`", -1)
		query = strings.Replace(query, ":Synapse", ":`"+dataset+"_Synapse`", -1)

		query = strings.Replace(query, ":`Neuron`", ":`"+dataset+"_Neuron`", -1)
		query = strings.Replace(query, ":`Segment`", ":`"+dataset+"_Segment`", -1)
		query = strings.Replace(query, ":`Meta`", ":`"+dataset+"_Meta`", -1)
		query = strings.Replace(query, ":`SynapseSet`", ":`"+dataset+"_SynapseSet`", -1)
		query = strings.Replace(query, ":`Synapse`", ":`"+dataset+"_Synapse`", -1)

		// handle SynapsesTo exception
		query = strings.Replace(query, ":XSynapsesTo", ":SynapsesTo", -1)
	}

	return cw.mainStore.CypherRequest(query, readonly)
}

func (cw *CypherWrapper) StartTrans() (CypherTransaction, error) {

	return cw.mainStore.StartTrans()
}

func (db *MasterDB) GetMain(datasets ...string) Cypher {
	// just consider the first store for now
	// default to the primary main store
	if len(datasets) > 0 {
		if store, ok := db.DatasetStores[datasets[0]]; ok {
			return &CypherWrapper{datasets[0], store.(Cypher)}
		} else {
			return &CypherWrapper{datasets[0], db.MainStores[0].(Cypher)}
		}
	}

	return &CypherWrapper{"", db.MainStores[0].(Cypher)}
}

// **** Re-implement SimpleStore interface (since we could have multiple main stores) ****
// TODO: change the outward facing store interface to return an array of versions, datatbases, etc

func (db *MasterDB) GetVersion() (string, error) {
	// just return the default value
	return db.MainStores[0].GetVersion()
}

func (db *MasterDB) GetDatabase() (string, string, error) {
	// just return the default value
	return db.MainStores[0].GetDatabase()
}

func (db *MasterDB) GetType() string {
	return ""
}

func (db *MasterDB) GetInstance() string {
	return ""
}

func (db *MasterDB) GetDatasets() (map[string]interface{}, error) {
	allDatasets := make(map[string]interface{})
	for _, store := range db.MainStores {
		datasets, err := store.GetDatasets()
		if err != nil {
			return nil, err
		}
		for key, val := range datasets {
			allDatasets[key] = val
		}
	}
	return allDatasets, nil
}

func (db *MasterDB) GetStores() []SimpleStore {
	return db.Stores
}

func (db *MasterDB) GetInstances() map[string]SimpleStore {
	return db.Instances
}

func (db *MasterDB) GetTypes() map[string][]SimpleStore {
	return db.Types
}

func (db *MasterDB) FindStore(typename string, dataset string) (SimpleStore, error) {
	typestores, ok := db.GetTypes()[typename]
	if !ok {
		return nil, fmt.Errorf("no store for the given datatype available")
	}

	var store SimpleStore
	for _, cstore := range typestores {
		datasets, err := cstore.GetDatasets()
		if err != nil {
			return nil, fmt.Errorf("error reading dataset information")

		}
		if _, ok := datasets[dataset]; ok {
			store = cstore
		}
	}

	if store == nil {
		return nil, fmt.Errorf("no store found supporting the datatype and dataset")
	}

	return store, nil
}
