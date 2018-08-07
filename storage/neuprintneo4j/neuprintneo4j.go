package neuprintneo4j

import (
        "github.com/janelia-flyem/neuPrintHTTP/storage"
        "github.com/blang/semver"
        "fmt"
)

func init() {
    version, _ := semver.Make(VERSION)
    e := Engine{NAME, version}
    storage.RegisterEngine(e)
}

const (
    // VERSION of database that is supported
    VERSION = "1.0"
    NAME = "neuPrint-neo4j"
)

type Engine struct {
    name string
    version semver.Version
    
}

func (e Engine) GetName() string {
    return e.name
}

func (e Engine) NewStore(data interface{}) (storage.Store, error) {
    datamap, ok := data.(map[string]interface{})
    var emptyStore storage.Store
    if !ok {
        return emptyStore, fmt.Errorf("incorrect configuration for neo4j") 
    }
    user, ok := datamap["user"].(string)
    if !ok {
        return emptyStore, fmt.Errorf("user not specified for neo4j") 
    }
    pass, ok := datamap["password"].(string)
    if !ok {
        return emptyStore, fmt.Errorf("password not specified for neo4j") 
    }
    datasetsInt, ok := datamap["datasets"].([]interface{})
    if !ok {
        return emptyStore, fmt.Errorf("datasets not specified for neo4j") 
    }
    datasets := make([]string, len(datasetsInt))
    for pos, val := range datasetsInt {
        if datasets[pos], ok = val.(string); !ok {
            return emptyStore, fmt.Errorf("datasets not specified properly for neo4j") 
        }
    }

    // ?! check if code is compatible with DB version
    dbversion, _ := semver.Make(VERSION)

    return Store{user, pass, datasets, dbversion}, nil
}

type Store struct {
    user string
    pass string
    datasets []string
    version semver.Version
}

func (store Store) GetName() string {
    return NAME
}

func (store Store) GetVersion() string {
    return store.version.String()
}

func (store Store) GetDatasets() []string {
    return store.datasets
}


// ?! implement connectomics API
