/*
   Provides top-level interface for accessing backend storage.
*/

package storage

import (
	"fmt"
)

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
	GetMain() SimpleStore
	GetStores() []SimpleStore
	GetInstances() map[string]SimpleStore
	GetTypes() map[string][]SimpleStore
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
func ParseConfig(engineName string, data interface{}, datatypes_raw interface{}) (Store, error) {
	if availEngines == nil {
		return nil, fmt.Errorf("No engines loaded")
	}
	var err error
	var mainStore SimpleStore

	if engine, found := availEngines[engineName]; !found {
		return nil, fmt.Errorf("Engine %s not found", engineName)
	} else {
		mainStore, err = engine.NewStore(data, "", "")
		if err != nil {
			return nil, err
		}
	}

	// load all data instance databases for auxiliary data
	datatypes, ok := datatypes_raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("incorrectly formatted datatypes field")
	}

	stores := make([]SimpleStore, 0)
	for key, val := range datatypes {
		instance_config := val.(DataInstance)

		if engine, found := availEngines[instance_config.Engine]; !found {
			return nil, fmt.Errorf("Engine %s not found", instance_config.Engine)
		} else {
			store, err := engine.NewStore(instance_config.Config, key, instance_config.Instance)
			if err != nil {
				return nil, err
			}
			stores = append(stores, store)
		}
	}

	// load MasterDB
	var instances map[string]SimpleStore
	var types map[string][]SimpleStore

	for _, val := range stores {
		name := val.GetInstance()
		if _, ok = instances[name]; !ok {
			return nil, fmt.Errorf("Non-unique instance given %s", name)
		}
		instances[name] = val

		tname := val.GetType()
		if _, ok = types[tname]; !ok {
			types[tname] = make([]SimpleStore, 0)
		}
		types[tname] = append(types[tname], val)
	}

	return MasterDB{mainStore, stores, instances, types, mainStore}, nil
}
