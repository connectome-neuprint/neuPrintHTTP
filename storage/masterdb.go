package storage

type MasterDB struct {
	SimpleStore
	Stores    []SimpleStore
	Instances map[string]SimpleStore
	Types     map[string][]SimpleStore
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
