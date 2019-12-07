package badger

import (
	"fmt"
	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	badgerdb "github.com/dgraph-io/badger"
)

/* Implements badger specific implemetation of storage. */

func init() {
	version, _ := semver.Make(VERSION)
	e := Engine{NAME, version}
	storage.RegisterEngine(e)
}

const (
	// VERSION of database that is supported
	VERSION = "1.0.0"
	NAME    = "badger"
)

type Engine struct {
	name    string
	version semver.Version
}

func (e Engine) GetName() string {
	return e.name
}

type badgerConfig struct {
	Dataset  string `json:"dataset"`
	Location string `json:"location"`
}

// NewStore creates an store instance that works with dvid.
// DVID requires  data instance name, server, branch, and dataset
func (e Engine) NewStore(data interface{}, typename, instance string) (storage.SimpleStore, error) {
	dbversion, _ := semver.Make(VERSION)
	datamap, ok := data.(map[string]interface{})

	cdataset, ok := datamap["dataset"].(string)
	if !ok {
		return nil, fmt.Errorf("incorrect configuration for neo4j")
	}
	clocation, ok := datamap["location"].(string)
	if !ok {
		return nil, fmt.Errorf("incorrect configuration for neo4j")
	}
	config := badgerConfig{cdataset, clocation}

	// initialize or open badger DB
	// Open the Badger database located location.
	// It will be created if it doesn't exist.
	db, err := badgerdb.Open(badgerdb.DefaultOptions(config.Location))
	if err != nil {
		return nil, err
	}

	return &Store{db, dbversion, typename, instance, config}, nil
}

// Store is the neo4j storage instance
type Store struct {
	db       *badgerdb.DB
	version  semver.Version
	typename string
	instance string
	config   badgerConfig
}

// GetDatabsae returns database information
func (store *Store) GetDatabase() (loc string, desc string, err error) {
	return store.config.Location, NAME, nil
}

// GetVersion returns the version of the driver
func (store *Store) GetVersion() (string, error) {
	return store.version.String(), nil
}

type databaseInfo struct {
	Location string `json:"location"`
}

// GetDatasets returns information on the datasets supported
func (store *Store) GetDatasets() (map[string]interface{}, error) {
	datasetmap := make(map[string]interface{})
	datasetmap[store.config.Dataset] = databaseInfo{store.config.Location}
	return datasetmap, nil
}

func (store *Store) GetInstance() string {
	return store.instance
}

func (store *Store) GetType() string {
	return store.typename
}

// **** Implements KeyValue Interface ****

func (s *Store) Close() {
	s.db.Close()
}

// Set wraps a transactionally safe key value write
func (s *Store) Set(key, val []byte) error {
	return s.db.Update(func(txn *badgerdb.Txn) error {
		return txn.Set(key, val)
	})
}

// Get wraps a transactionally safe key value get
func (s *Store) Get(key []byte) ([]byte, error) {
	var valCopy []byte
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		valCopy, err = item.ValueCopy(nil)
		return err
	})

	if err == badgerdb.ErrKeyNotFound {
		return nil, storage.ErrKeyNotFound
	}

	return valCopy, err
}
