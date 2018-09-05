/*
   Provides top-level interface for accessing backend storage.
*/

package storage

import (
	"fmt"
)

// Store is an instance of Engine
type Store interface {
	GetVersion() (string, error)
	GetDatabase() (string, string, error)
	GetDatasets() ([]string, error)
}

// Engine is the backend database that implements connectomics API
type Engine interface {
	GetName() string
	NewStore(data interface{}) (Store, error)
}

var (
	availEngines map[string]Engine
)

// RegisterEngine associates a given storage backend with a name
func RegisterEngine(e Engine) {
	if availEngines == nil {
		availEngines = map[string]Engine{e.GetName(): e}
	} else {
		availEngines[e.GetName()] = e
	}
}

// ParseConfig finds the appropriate storage engine from the configuration and initializes it
func ParseConfig(engineName string, data interface{}) (store Store, err error) {
	if availEngines == nil {
		return nil, fmt.Errorf("No engines loaded")
	}
	if engine, found := availEngines[engineName]; !found {
		return nil, fmt.Errorf("Engine %s not found", engineName)
	} else {
		return engine.NewStore(data)
	}
}
