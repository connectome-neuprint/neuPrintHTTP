/*
   Provides top-level interface for accessing backend storage.
*/

package storage

import (
	"fmt"
)

var GlobalTimeout = 60

// ***** Main interfaces to top-level databases *****

// SimpleStore is an instance of Engine
type SimpleStore interface {
	GetVersion() (string, error)
	GetDatabase() (string, string, error)
	GetDatasets() (map[string]interface{}, error)
	GetType() string
	GetInstance() string
	// interface query should be done by checking match to different interfaces
}

// Store provides the interface to access the database and all instances
type Store interface {
	SimpleStore
	GetMain(datasets ...string) Cypher
	GetStores() []SimpleStore
	GetInstances() map[string]SimpleStore
	GetTypes() map[string][]SimpleStore
	FindStore(typename string, dataset string) (SimpleStore, error)
}

// **** Low-level interfaces that different backend can support ****

// CypherResult contain the default response formatted from neo4j as column names and rows of data
type CypherResult struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
	Debug   string          `json:"debug"`
}

// CypherTransaction provides transaction access to a graph database
type CypherTransaction interface {
	CypherRequest(string, bool) (CypherResult, error)
	Kill() error
	Commit() error
}

// Cypher is the main interface for accessing graph databases
type Cypher interface {
	CypherRequest(string, bool) (CypherResult, error)
	StartTrans() (CypherTransaction, error)
}

// Spatial is the main interface for accessing spatial databases
type Spatial interface {
	// TODO: high-level wrapper could implement a shortest path based using a mask
	QueryByPoint(Point) ([]uint64, error)
	QueryByBbox(Point, Point) ([]uint64, error)
	Raw3dData(Point, Point, Scale, Compression) ([]byte, error)
}

// KeyValueis the main interface for accessing keyvalue databases
type KeyValue interface {
	Get([]byte) ([]byte, error)
	Set([]byte, []byte) error
}

// ParseConfig finds the appropriate storage engine from the configuration and initializes it
func ParseConfig(engineName string, data interface{}, mainstores []interface{}, datatypes_raw interface{}, timeout int) (Store, error) {
	GlobalTimeout = timeout
	if availEngines == nil {
		return nil, fmt.Errorf("No engines loaded")
	}
	var err error
	mainStores := make([]SimpleStore, 0, 0)
	datasetStores := make(map[string]SimpleStore)

	var firstStore SimpleStore
	if engine, found := availEngines[engineName]; !found {
		return nil, fmt.Errorf("Engine %s not found", engineName)
	} else {
		firstStore, err = engine.NewStore(data, "", "")
		if err != nil {
			return nil, err
		}
	}
	mainStores = append(mainStores, firstStore)

	var mainStore SimpleStore
	for _, engine_data_raw := range mainstores {
		engine_data := engine_data_raw.(map[string]interface{})

		engineName, ok := engine_data["engine"].(string)
		if !ok {
			return nil, fmt.Errorf("alternative engine not formatted correctly")
		}
		if engine, found := availEngines[engineName]; !found {
			return nil, fmt.Errorf("Engine %s not found", engineName)
		} else {
			data = engine_data["engine-config"]

			mainStore, err = engine.NewStore(data, "", "")
			if err != nil {
				return nil, err
			}
		}

		// add to dataset stores
		datasets, err := mainStore.GetDatasets()
		if err != nil {
			return nil, err
		}
		for dataset, _ := range datasets {
			if _, ok = datasetStores[dataset]; ok {
				return nil, fmt.Errorf("dataset exists multiple times")
			}

			datasetStores[dataset] = mainStore
		}

		mainStores = append(mainStores, mainStore)
	}

	// add default store to dataset stores
	datasets, err := firstStore.GetDatasets()
	if err != nil {
		return nil, err
	}
	for dataset, _ := range datasets {
		if _, ok := datasetStores[dataset]; ok {
			return nil, fmt.Errorf("dataset exists multiple times")
		}
		datasetStores[dataset] = firstStore
	}

	// load all data instance databases for auxiliary data
	datatypes, ok := datatypes_raw.(map[string]interface{})
	if !ok {
		//return nil, fmt.Errorf("incorrectly formatted datatypes field")
		fmt.Println("WARNING: No auxiliary datatypes specified (skeleton endpoints will not work)")
		datatypes = make(map[string]interface{})
	}

	stores := make([]SimpleStore, 0)
	for key, val := range datatypes {
		instance_configs := val.([]interface{})
		for _, iconfig_int := range instance_configs {
			iconfig, ok := iconfig_int.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("data instance not formatted properly")
			}
			instance, ok := iconfig["instance"].(string)
			if !ok {
				return nil, fmt.Errorf("data instance not formatted properly")
			}
			engine, ok := iconfig["engine"].(string)
			if !ok {
				return nil, fmt.Errorf("data instance not formatted properly")
			}
			config, ok := iconfig["engine-config"].(interface{})
			if !ok {
				return nil, fmt.Errorf("data instance not formatted properly")
			}
			if engine, found := availEngines[engine]; !found {
				return nil, fmt.Errorf("Engine %s not found", engine)
			} else {
				store, err := engine.NewStore(config, key, instance)
				if err != nil {
					return nil, err
				}
				stores = append(stores, store)
			}
		}
	}

	// load MasterDB
	instances := make(map[string]SimpleStore)
	types := make(map[string][]SimpleStore)

	for _, val := range stores {
		name := val.GetInstance()
		if _, exists := instances[name]; exists {
			return nil, fmt.Errorf("Non-unique instance given %s", name)
		}
		instances[name] = val

		tname := val.GetType()
		if _, ok = types[tname]; !ok {
			types[tname] = make([]SimpleStore, 0)
		}
		types[tname] = append(types[tname], val)
	}

	return &MasterDB{mainStores, datasetStores, stores, instances, types}, nil
}
