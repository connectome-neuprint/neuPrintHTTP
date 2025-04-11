package dvid

// TBD
import (
	"fmt"
	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
)

func init() {
	version, _ := semver.Make(VERSION)
	e := Engine{NAME, version}
	storage.RegisterEngine(e)
}

const (
	// VERSION of database that is supported
	VERSION = "0.1.0"
	NAME    = "dvid"
)

type Engine struct {
	name    string
	version semver.Version
}

func (e Engine) GetName() string {
	return e.name
}

type dvidConfig struct {
	Dataset  string `json:"dataset"`
	Server   string `json:"server"`
	Branch   string `json:"branch"`
	Instance string `json:"instance"`
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
	cserver, ok := datamap["server"].(string)
	if !ok {
		return nil, fmt.Errorf("incorrect configuration for neo4j")
	}
	cbranch, ok := datamap["branch"].(string)
	if !ok {
		return nil, fmt.Errorf("incorrect configuration for neo4j")
	}
	cinstance, ok := datamap["instance"].(string)
	if !ok {
		return nil, fmt.Errorf("incorrect configuration for neo4j")
	}

	return &Store{dbversion, typename, instance, dvidConfig{cdataset, cserver, cbranch, cinstance}}, nil
}

// Store is the neo4j storage instance
type Store struct {
	version  semver.Version
	typename string
	instance string
	config   dvidConfig
}

// GetDatabsae returns database information
func (store *Store) GetDatabase() (loc string, desc string, err error) {
	return store.config.Server, NAME, nil
}

// GetVersion returns the version of the driver
func (store *Store) GetVersion() (string, error) {
	return store.version.String(), nil
}

type databaseInfo struct {
	Branch   string `json:"branch"`
	Instance string `json:"instance"`
}

// GetDatasets returns information on the datasets supported
func (store *Store) GetDatasets() (map[string]interface{}, error) {
	datasetmap := make(map[string]interface{})
	datasetmap[store.config.Dataset] = databaseInfo{store.config.Branch, store.config.Instance}

	return datasetmap, nil
}

func (store *Store) GetInstance() string {
	return store.instance
}

func (store *Store) GetType() string {
	return store.typename
}

// *** Spatial Query Interfacde ****

func (store *Store) QueryByPoint(point storage.Point) ([]uint64, error) {
	return nil, nil
}

func (store *Store) QueryByBbox(point1 storage.Point, point2 storage.Point) ([]uint64, error) {
	return nil, nil
}

func (store *Store) Raw3dData(point1 storage.Point, point2 storage.Point, scale storage.Scale, compression storage.Compression) ([]uint64, error) {
	return nil, nil
}
