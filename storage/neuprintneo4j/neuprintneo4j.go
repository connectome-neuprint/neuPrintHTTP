package neuprintneo4j

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"io/ioutil"
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
	VERSION = "1.0"
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
	server, ok := datamap["server"].(string)
	if !ok {
		return emptyStore, fmt.Errorf("server not specified for neo4j")
	}

	// TODO: check if code is compatible with DB version
	dbversion, _ := semver.Make(VERSION)

	// TODO: check connection to DB
	/*if err != nil {
	    return emptyStore, fmt.Errorf("could not connect to database")
	}*/
	preurl := "http://" + user + ":" + pass + "@"
	url := preurl + server + "/db/data/transaction"

	return Store{server, dbversion, url, preurl}, nil
}

// neoResultProc contain the default response formatted from neo4j
// as column names and rows of data
type neoResultProc struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
	Debug   string          `json:"debug"`
}

// neoRow is an array of rows that are returned from neo4j
type neoRow struct {
	Row []interface{} `json:"row"`
}

// neoResult is the response for a given neo4j statement
type neoResult struct {
	Columns []string               `json:"columns"`
	Data    []neoRow               `json:"data"`
	Stats   map[string]interface{} `json:"stats"`
}

// neoError is the error information returned for a given statement
type neoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// neoResults is the set of results for all statements
type neoResults struct {
	Results []neoResult `json:"results"`
	Errors  []neoError  `json:"errors"`
}

// neoStatement is a single query statement
type neoStatement struct {
	Statement    string `json:"statement"`
	IncludeStats bool   `json:"includeStats"`
}

// neoStatements is a set of query statements
type neoStatements struct {
	Statements []neoStatement `json:"statements"`
}

// makeRequest makes a simple cypher request to neo4j
func (store Store) makeRequest(cypher string) (*neoResultProc, error) {
	neoClient := http.Client{
		Timeout: time.Second * 60,
	}

	transaction := neoStatements{[]neoStatement{neoStatement{cypher, true}}}

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(transaction)
	req, err := http.NewRequest(http.MethodPost, store.url, b)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Stream", "true")
	res, err := neoClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}

	result := neoResults{}
	jsonErr := json.Unmarshal(body, &result)
	if jsonErr != nil {
		return nil, fmt.Errorf("error decoding json")
	}

	// if database was modified, rollback the transaction (only allow readonly)
	if result.Results[0].Stats["contains_updates"].(bool) {
		locationURL, _ := res.Location()
		commitLocation := strings.Replace(locationURL.String(), "http://", store.preurl, -1)

		bempty := new(bytes.Buffer)
		newreq, err := http.NewRequest(http.MethodDelete, commitLocation, bempty)
		if err != nil {
			return nil, fmt.Errorf("request failed")
		}
		_, err = neoClient.Do(newreq)
		if err != nil {
			return nil, fmt.Errorf("request failed")
		}
		return nil, fmt.Errorf("not authorized to modify the database")
	} else {
		// commit transaction
		locationURL, _ := res.Location()
		commitLocation := strings.Replace(locationURL.String(), "http://", store.preurl, -1)
		commitLocation += "/commit"

		bempty := new(bytes.Buffer)
		newreq, err := http.NewRequest(http.MethodPost, commitLocation, bempty)
		if err != nil {
			return nil, fmt.Errorf("request failed")
		}
		_, err = neoClient.Do(newreq)
		if err != nil {
			return nil, fmt.Errorf("request failed")
		}
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("%s: %s", result.Errors[0].Code, result.Errors[0].Message)
	}

	data := make([][]interface{}, len(result.Results[0].Data))
	for row, val := range result.Results[0].Data {
		arr := make([]interface{}, len(val.Row))
		for col, val2 := range val.Row {
			arr[col] = val2
		}
		data[row] = arr
	}
	procRes := neoResultProc{result.Results[0].Columns, data, cypher}
	return &procRes, nil
}

// Store is the neo4j storage instance
type Store struct {
	server  string
	version semver.Version
	url     string
	preurl  string
}

// GetDatabsae returns database information
func (store Store) GetDatabase() (loc string, desc string, err error) {
	return store.server, NAME, nil
}

// GetVersion returns the version of the driver
func (store Store) GetVersion() (string, error) {
	return store.version.String(), nil
}

type databaseInfo struct {
	LastEdit string   `json:"last-mod"`
	UUID     string   `json:"uuid"`
	ROIs     []string `json:"ROIs"`
	Info     string   `json:"info"`
}

// GetDatasets returns information on the datasets supported
func (store Store) GetDatasets() (map[string]interface{}, error) {
	cypher := "MATCH (m :Meta) RETURN m.dataset, m.uuid, m.lastDatabaseEdit, m.roiInfo, m.info"
	metadata, err := store.makeRequest(cypher)
	if err != nil {
		return nil, err
	}

	res := make(map[string]interface{})

	for _, row := range metadata.Data {
		dataset := row[0].(string)
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
		dbInfo := databaseInfo{edit, uuid, make([]string, 0, len(roidata)), info}

		for roi := range roidata {
			dbInfo.ROIs = append(dbInfo.ROIs, roi)
		}

		res[dataset] = dbInfo
	}

	return res, nil
}

// CustomRequest implements API that allows users to specify exact query
func (store Store) CustomRequest(req map[string]interface{}) (res interface{}, err error) {
	// TODO: prevent modifications
	cypher, ok := req["cypher"].(string)
	if !ok {
		err = fmt.Errorf("cypher keyword not found in request JSON")
		return
	}
	return store.makeRequest(cypher)
}

/*
func (store Store) CustomRequest(req map[string]interface{}) (res interface{}, err error) {
    cypher, ok := req["cypher"].(string)
    if !ok {
        err = fmt.Errorf("cypher keyword not found in request JSON")
        return
    }
    res2 := []struct {
        Pname interface{} `json:"pname"`
    }{}
    cq := neoism.CypherQuery{
        Statement: cypher,
        Result: &res2,
    }
    err = store.database.Cypher(&cq)
    //fmt.Println(res2)
    //fmt.Println(res2[1].pname)
    if err != nil {
        err = fmt.Errorf("cypher query error")
        return
    }
    res = res2

    return
}
*/
