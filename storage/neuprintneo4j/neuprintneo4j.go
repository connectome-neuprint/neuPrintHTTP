package neuprintneo4j

import (
	"encoding/json"
	"fmt"
	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"net/http"
	"strings"
	"time"
)

func init() {
	version, _ := semver.Make(VERSION)
	e := Engine{NAME, version}
	storage.RegisterEngine(e)
}

const (
	// VERSION of database that is supported
	VERSION = "0.5.0"
	NAME    = "neuPrint-neo4j"
)

type Engine struct {
	name    string
	version semver.Version
}

func (e Engine) GetName() string {
	return e.name
}

// NewStore creates an store instance that works with neo4j.
// The neo4j engine requires a user name and password to authenticate and
// the location of the server.
func (e Engine) NewStore(data interface{}, typename, instance string) (storage.SimpleStore, error) {
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
	server, ok := datamap["server"].(string)
	if !ok {
		return emptyStore, fmt.Errorf("server not specified for neo4j")
	}

	dbversion, _ := semver.Make(VERSION)

	// TODO: check connection to DB
	/*if err != nil {
	    return emptyStore, fmt.Errorf("could not connect to database")
	}*/
	preurl := "http://" + user + ":" + pass + "@"
	url := preurl + server + "/db/data/transaction"

	return &Store{server, dbversion, url, preurl, typename, instance}, nil
}

// Store is the neo4j storage instance
type Store struct {
	server   string
	version  semver.Version
	url      string
	preurl   string
	typename string
	instance string
}

// GetDatabsae returns database information
func (store *Store) GetDatabase() (loc string, desc string, err error) {
	return store.server, NAME, nil
}

// GetVersion returns the version of the driver
func (store *Store) GetVersion() (string, error) {
	return store.version.String(), nil
}

type databaseInfo struct {
	LastEdit       string   `json:"last-mod"`
	UUID           string   `json:"uuid"`
	ROIs           []string `json:"ROIs"`
	SuperLevelROIs []string `json:"superLevelROIs"`
	Info           string   `json:"info"`
}

// GetDatasets returns information on the datasets supported
func (store *Store) GetDatasets() (map[string]interface{}, error) {
	cypher := "MATCH (m :Meta) RETURN m.dataset, m.uuid, m.lastDatabaseEdit, m.roiInfo, m.info, m.superLevelRois AS rois, m.tag AS tag"
	metadata, err := store.CypherRequest(cypher, true)
	if err != nil {
		return nil, err
	}

	res := make(map[string]interface{})

	for _, row := range metadata.Data {
		dataset := row[0].(string)

		// add tag to the dataset name if it exists
		if row[6] != nil {
			tag := row[6].(string)
			dataset += (":" + tag)
		}

		uuid := "latest"
		if row[1] != nil {
			uuid = row[1].(string)
		}
		edit := row[2].(string)
		roistr := row[3].(string)
		info := "N/A"
		if row[4] != nil {
			info = row[4].(string)
		}
		roibytes := []byte(roistr)
		var roidata map[string]interface{}
		err = json.Unmarshal(roibytes, &roidata)
		if err != nil {
			return nil, err
		}

		superROIs := row[5].([]interface{})
		dbInfo := databaseInfo{edit, uuid, make([]string, 0, len(roidata)), make([]string, 0, len(superROIs)), info}

		for roi := range roidata {
			dbInfo.ROIs = append(dbInfo.ROIs, roi)
		}

		for _, superROI := range superROIs {
			sroi := superROI.(string)
			dbInfo.SuperLevelROIs = append(dbInfo.SuperLevelROIs, sroi)
		}

		res[dataset] = dbInfo
	}

	return res, nil
}

func (store *Store) GetInstance() string {
	return store.instance
}

func (store *Store) GetType() string {
	return store.typename
}

// **** Cypher Specific Interface ****

// CypherRequest makes a simple cypher request to neo4j
func (store *Store) CypherRequest(cypher string, readonly bool) (storage.CypherResult, error) {
	trans, _ := store.StartTrans()
	res, err := trans.CypherRequest(cypher, readonly)
	var cres storage.CypherResult
	if err != nil {
		if strings.Contains(err.Error(), "Timeout") {
			return cres, fmt.Errorf("Timeout experienced.  This could be due to database traffic or to non-optimal database queries. If the latter, please consult neuPrint documentation or post a question at https://groups.google.com/forum/#!forum/neuprint to understand other options.")
		}
		return cres, err
	}
	if err = trans.Commit(); err != nil {
		return cres, err
	}
	return res, nil
}

// StartTrans starts a graph DB transaction
func (store *Store) StartTrans() (storage.CypherTransaction, error) {
	neoClient := http.Client{
		Timeout: time.Second * time.Duration(storage.GlobalTimeout),
	}
	return &Transaction{currURL: store.url, preURL: store.preurl, neoClient: neoClient, isStarted: false}, nil
}
