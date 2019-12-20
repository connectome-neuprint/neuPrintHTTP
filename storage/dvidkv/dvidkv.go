package dvidkv

import (
	"bytes"
	"fmt"
	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"io/ioutil"
	"net/http"
	"time"
)

func init() {
	version, _ := semver.Make(VERSION)
	e := Engine{NAME, version}
	storage.RegisterEngine(e)
}

const (
	// VERSION of database that is supported
	VERSION = "0.1.0"
	NAME    = "dvidkv"
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
	Token    string `json:"token,omitempty"`
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
	token, ok := datamap["token"].(string)
	if !ok {
		token = ""
	}

	config := dvidConfig{cdataset, cserver, cbranch, cinstance, token}
	endPoint := config.Server + "/api/node/" + config.Branch + "/" + config.Instance + "/key/"
	return &Store{dbversion, typename, instance, config, endPoint}, nil
}

// Store is the neo4j storage instance
type Store struct {
	version  semver.Version
	typename string
	instance string
	config   dvidConfig
	endPoint string
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

// *** KeyValue Query Interfacde ****

// Set puts data into DVID
func (s *Store) Set(key, val []byte) error {
	dvidClient := http.Client{
		Timeout: time.Second * 60,
	}

	req, err := http.NewRequest(http.MethodPost, s.endPoint+string(key), bytes.NewBuffer(val))
	if err != nil {
		return fmt.Errorf("request failed")
	}

	res, err := dvidClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("request failed")
	}
	return nil
}

// Get retrieve data from DVID
func (s *Store) Get(key []byte) ([]byte, error) {
	dvidClient := http.Client{
		Timeout: time.Second * 60,
	}

	//fmt.Println(s.endPoint + string(key))
	req, err := http.NewRequest(http.MethodGet, s.endPoint+string(key), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	if s.config.Token != "" {
		req.Header.Add("Authorization", "Bearer "+s.config.Token)
	}

	res, err := dvidClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if len(body) > 0 {
			return nil, fmt.Errorf("%s", body)
		}
		return nil, fmt.Errorf("request failed")
	}

	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	return body, err
}
