package neuprintneo4j

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"io/ioutil"
	"net/http"
	"strings"
)

type Transaction struct {
	currURL   string // curr tranaction URL
	preURL    string // pre URL
	neoClient http.Client
	isStarted bool
}

func (t *Transaction) CypherRequest(cypher string, readonly bool) (storage.CypherResult, error) {
	// empty result
	var cres storage.CypherResult

	transaction := neoStatements{[]neoStatement{neoStatement{cypher, true}}}

	b := new(bytes.Buffer)
	json.NewEncoder(b).Encode(transaction)
	req, err := http.NewRequest(http.MethodPost, t.currURL, b)
	if err != nil {
		return cres, fmt.Errorf("request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Stream", "true")
	res, err := t.neoClient.Do(req)
	if err != nil {
		return cres, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return cres, err
	}

	result := neoResults{}
	jsonErr := json.Unmarshal(body, &result)
	if jsonErr != nil {
		return cres, fmt.Errorf("error decoding json")
	}

	if len(result.Errors) > 0 {
		return cres, fmt.Errorf(result.Errors[0].Message)
	}

	if !t.isStarted {
		locationURL, _ := res.Location()
		t.currURL = strings.Replace(locationURL.String(), "http://", t.preURL, -1)
		t.isStarted = true
	}

	// if database was modified and readonly, rollback the transaction (only allow readonly)
	if readonly && result.Results[0].Stats["contains_updates"].(bool) {
		if err := t.Kill(); err != nil {
			return cres, err
		}
		return cres, fmt.Errorf("not authorized to modify the database")
	}

	data := make([][]interface{}, len(result.Results[0].Data))
	for row, val := range result.Results[0].Data {
		arr := make([]interface{}, len(val.Row))
		for col, val2 := range val.Row {
			arr[col] = val2
		}
		data[row] = arr
	}
	procRes := storage.CypherResult{Columns: result.Results[0].Columns, Data: data, Debug: cypher}
	return procRes, nil
}

func (t *Transaction) Kill() error {
	if !t.isStarted {
		// nothing to kill
		return nil
	}

	// technically allow reuse of transaction
	t.isStarted = false

	bempty := new(bytes.Buffer)
	newreq, err := http.NewRequest(http.MethodDelete, t.currURL, bempty)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	res, err := t.neoClient.Do(newreq)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("request failed")
	}

	result := neoResults{}
	jsonErr := json.Unmarshal(body, &result)
	if jsonErr != nil {
		return fmt.Errorf("error decoding json")
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf(result.Errors[0].Message)
	}

	return nil
}

func (t *Transaction) Commit() error {
	// technically allow reuse of transaction
	t.isStarted = false

	commitLocation := t.currURL + "/commit"

	bempty := new(bytes.Buffer)
	newreq, err := http.NewRequest(http.MethodPost, commitLocation, bempty)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	res, err := t.neoClient.Do(newreq)
	if err != nil {
		return fmt.Errorf("request failed")
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("request failed")
	}

	result := neoResults{}
	jsonErr := json.Unmarshal(body, &result)
	if jsonErr != nil {
		return fmt.Errorf("error decoding json")
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf(result.Errors[0].Message)
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
