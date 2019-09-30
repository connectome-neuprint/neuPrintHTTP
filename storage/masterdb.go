package storage

import "fmt"

type MasterDB struct {
	SimpleStore
	Stores    []SimpleStore
	Instances map[string]SimpleStore
	Types     map[string][]SimpleStore
	MainStore SimpleStore
}

func (db MasterDB) GetMain() SimpleStore {
	return db.MainStore
}

func (db MasterDB) GetStores() []SimpleStore {
	return db.Stores
}

func (db MasterDB) GetInstances() map[string]SimpleStore {
	return db.Instances
}

func (db MasterDB) GetTypes() map[string][]SimpleStore {
	return db.Types
}

func (db MasterDB) FindStore(typename string, dataset string) (SimpleStore, error) {
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
