package neuprintneo4j

import (
        "github.com/janelia-flyem/neuPrintHTTP/storage"
        "github.com/blang/semver"
        "fmt"
        "net/http"
        "time"
        "io/ioutil"
        "encoding/json"
        "bytes"
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
    server, ok := datamap["server"].(string)
    if !ok {
        return emptyStore, fmt.Errorf("server not specified for neo4j") 
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

    // TODO: check if code is compatible with DB version
    dbversion, _ := semver.Make(VERSION)

    // TODO: check connection to DB 
    /*if err != nil {
        return emptyStore, fmt.Errorf("could not connect to database") 
    }*/
    url := "http://" + user + ":" + pass + "@" + server + "/db/data/transaction/commit"

    return Store{datasets, dbversion, url}, nil
}


type neoResultProc struct {
    Columns []string `json:"columns"`
    Data [][]interface{}  `json:"data"`
}

type neoRow struct {
    Row []interface{} `json:"row"`
}
type neoResult struct {
    Columns []string `json:"columns"`
    Data []neoRow `json:"data"`
}
type neoError struct {
    Code string `json:"code"`
    Message string `json:"message"`
}

type neoResults struct {
    Results []neoResult `json:"results"`
    Errors []neoError `json:"errors"`
}

type neoStatement struct {
    Statement string `json:"statement"`
}
type neoStatements struct {
    Statements []neoStatement `json:"statements"`
}

func (store Store) makeRequest(cypher string) (*neoResultProc, error) {
    neoClient := http.Client{
		Timeout: time.Second * 60,
    }
    
    transaction := neoStatements{[]neoStatement{neoStatement{cypher}}}

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
    procRes := neoResultProc{result.Results[0].Columns, data}
    return &procRes, nil
}


type Store struct {
    datasets []string
    version semver.Version
    url string
}

func (store Store) GetDatabase() (loc string, desc string, err error) {
    return "somwhere", NAME, nil
}

func (store Store) GetVersion() (string, error) {
    return store.version.String(), nil
}

func (store Store) GetDatasets() ([]string, error) {
    return store.datasets, nil
}


func (store Store) CustomRequest(req map[string]interface{}) (res interface{}, err error) {
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
