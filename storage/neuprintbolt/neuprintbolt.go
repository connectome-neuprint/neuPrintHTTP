package neuprintbolt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func init() {
	version, _ := semver.Make(VERSION)
	e := Engine{NAME, version}
	storage.RegisterEngine(e)
	fmt.Printf("Registered Neo4j Bolt engine: %s\n", NAME)
}

const (
	// VERSION of database that is supported
	VERSION = "0.5.0"
	NAME    = "neuPrint-bolt"
)

// Engine implements the storage.Engine interface for Neo4j Bolt protocol
type Engine struct {
	name    string
	version semver.Version
}

// GetName returns the name of the engine
func (e Engine) GetName() string {
	return e.name
}

// NewStore creates a store instance that works with neo4j using the Bolt protocol.
// The neo4j engine requires the location of the server and possibly
// a user name and password.
func (e Engine) NewStore(data interface{}, typename, instance string) (storage.SimpleStore, error) {
	datamap, ok := data.(map[string]interface{})
	var emptyStore storage.Store
	if !ok {
		return emptyStore, fmt.Errorf("incorrect configuration for neo4j")
	}
	
	// Get server URL (using bolt:// or neo4j:// scheme)
	server, ok := datamap["server"].(string)
	if !ok {
		return emptyStore, fmt.Errorf("server not specified for neo4j")
	}
	
	// Check if we need to add bolt:// prefix if it doesn't have a scheme
	if !strings.HasPrefix(server, "bolt://") && 
	   !strings.HasPrefix(server, "neo4j://") &&
	   !strings.HasPrefix(server, "neo4j+s://") &&
	   !strings.HasPrefix(server, "neo4j+ssc://") &&
	   !strings.HasPrefix(server, "bolt+s://") &&
	   !strings.HasPrefix(server, "bolt+ssc://") {
		server = "bolt://" + server
	}
	
	user, ok := datamap["user"].(string)
	if !ok {
		fmt.Printf("Noted: user not specified for neo4j\n")
	}
	
	pass, ok := datamap["password"].(string)
	if !ok {
		fmt.Printf("Noted: password not specified for neo4j\n")
	}
	
	// Check for database name (Neo4j 4.0+ supports multiple databases)
	dbName, _ := datamap["database"].(string)
	if dbName != "" {
		fmt.Printf("Using Neo4j database: %s\n", dbName)
	}
	
	// Create the driver
	ctx := context.Background()
	var driver neo4j.DriverWithContext
	var err error
	
	if user != "" && pass != "" {
		driver, err = neo4j.NewDriverWithContext(
			server, 
			neo4j.BasicAuth(user, pass, ""),
			func(config *neo4j.Config) {
				config.MaxConnectionPoolSize = 50
				config.MaxConnectionLifetime = time.Duration(storage.GlobalTimeout) * time.Second
			},
		)
	} else {
		driver, err = neo4j.NewDriverWithContext(
			server, 
			neo4j.NoAuth(),
			func(config *neo4j.Config) {
				config.MaxConnectionPoolSize = 50
				config.MaxConnectionLifetime = time.Duration(storage.GlobalTimeout) * time.Second
			},
		)
	}
	
	if err != nil {
		return emptyStore, fmt.Errorf("failed to create Neo4j driver: %w", err)
	}
	
	// Test the connection
	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		return emptyStore, fmt.Errorf("failed to connect to Neo4j: %w", err)
	}
	
	dbversion, _ := semver.Make(VERSION)
	
	return &Store{
		server:    server,
		version:   dbversion,
		driver:    driver,
		typename:  typename,
		instance:  instance,
		ctx:       ctx,
		database:  dbName,
	}, nil
}

// Store is the neo4j storage instance using the Bolt protocol
type Store struct {
	server   string
	version  semver.Version
	driver   neo4j.DriverWithContext
	typename string
	instance string
	ctx      context.Context
	database string // The Neo4j database name (for Neo4j 4.0+)
}

// GetDatabase returns database information
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
	Hidden         bool     `json:"hidden"`
	Logo           string   `json:"logo"`
	Description    string   `json:"description"`
}

// GetDatasets returns information on the datasets supported
func (store *Store) GetDatasets() (map[string]interface{}, error) {
	if storage.Verbose {
		fmt.Printf("Trying to get datasets\n")
	}
	
	cypher := "MATCH (m :Meta) RETURN m.dataset, m.uuid, m.lastDatabaseEdit, m.roiInfo, m.info, m.superLevelRois AS rois, m.tag AS tag, m.hideDataSet AS hidden, m.logo, m.description"
	metadata, err := store.CypherRequest(cypher, true)
	if err != nil {
		return nil, err
	}
	
	if storage.Verbose {
		fmt.Printf("GetDatasets: %v\n", metadata)
	}

	if len(metadata.Data) == 0 {
		return nil, fmt.Errorf("no datasets found in server %s", store.server)
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
		
		// Parse the ROI info JSON string
		var roidata map[string]interface{}
		err = json.Unmarshal([]byte(roistr), &roidata)
		if err != nil {
			return nil, err
		}

		hidden := false
		if row[7] != nil {
			hidden = row[7].(bool)
		}

		logo := ""
		if row[8] != nil {
			logo = row[8].(string)
		}

		description := ""
		if row[9] != nil {
			description = row[9].(string)
		}

		superROIs := row[5].([]interface{})
		dbInfo := databaseInfo{
			LastEdit:       edit,
			UUID:           uuid,
			ROIs:           make([]string, 0, len(roidata)),
			SuperLevelROIs: make([]string, 0, len(superROIs)),
			Info:           info,
			Hidden:         hidden,
			Logo:           logo,
			Description:    description,
		}

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

// CypherRequest makes a simple cypher request to neo4j
func (store *Store) CypherRequest(cypher string, readonly bool) (storage.CypherResult, error) {
	trans, err := store.StartTrans()
	if err != nil {
		return storage.CypherResult{}, err
	}
	
	res, err := trans.CypherRequest(cypher, readonly)
	var cres storage.CypherResult
	if err != nil {
		if strings.Contains(err.Error(), "Timeout") {
			return cres, fmt.Errorf("Timeout experienced. This could be due to database traffic or to non-optimal database queries. If the latter, please consult neuPrint documentation or post a question at https://groups.google.com/forum/#!forum/neuprint to understand other options.")
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
	return &Transaction{
		ctx:        store.ctx,
		driver:     store.driver,
		isExplicit: false,
		database:   store.database,
	}, nil
}

// Close closes the Neo4j driver
func (store *Store) Close() error {
	return store.driver.Close(store.ctx)
}