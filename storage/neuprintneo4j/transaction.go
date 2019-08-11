package neuprintneo4j

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"net/http"
	"strings"
	"time"
)

// CypherTransaction provides transaction access to a graph database
type CypherTransaction interface {
	CypherRequest(string, bool) (res interface{}, err error)
	Kill() error
	Commit() error
}

// Cypher is the main interface for accessing graph databases
type Cypher interface {
	CypherRequest(string, bool) (res interface{}, err error)
	StartTrans() (CypherTransaction, error)
}

type Transaction struct {
	currURL string // curr tranaction URL
	preURL  string // pre URL
}

func (t Transaction) CypherRequest(cypher string, readonly bool) (CypherResult, error) {
	neoClient := http.Client{
		Timeout: time.Second * 60,
	}

	transaction := neoStatements{[]neoStatement{neoStatement{cypher, true}}}

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(transaction)
	req, err := http.NewRequest(http.MethodPost, t.currURL, b)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Stream", "true")
	res, err := neoClient.Do(req)
	if err != nil {
		return nil, err
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

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf(result.Errors[0].Message)
	}

	locationURL, _ := res.Location()
	t.currURL = strings.Replace(locationURL.String(), "http://", t.preURL, -1)
	// if database was modified and readonly, rollback the transaction (only allow readonly)
	if readonly && result.Results[0].Stats["contains_updates"].(bool) {
		if err := t.Kill(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("not authorized to modify the database")
	}

	data := make([][]interface{}, len(result.Results[0].Data))
	for row, val := range result.Results[0].Data {
		arr := make([]interface{}, len(val.Row))
		for col, val2 := range val.Row {
			arr[col] = val2
		}
		data[row] = arr
	}
	procRes := storage.CypherResult{result.Results[0].Columns, data, cypher}
	return &procRes, nil
}

func (t Transaction) Kill() error {
	bempty := new(bytes.Buffer)
	newreq, err := http.NewRequest(http.MethodDelete, t.currURL, bempty)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	_, err = neoClient.Do(newreq)
	if err != nil {
		return fmt.Errorf("request failed")
	}

	return nil
}

func (t Transaction) Commit() error {
	commitLocation := t.currURL + "/commit"

	bempty := new(bytes.Buffer)
	newreq, err := http.NewRequest(http.MethodPost, commitLocation, bempty)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	_, err = neoClient.Do(newreq)
	if err != nil {
		return fmt.Errorf("request failed")
	}

	return nil
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
