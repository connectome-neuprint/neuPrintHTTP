package cypuer

import (
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/connectome-neuprint/neuPrintHTTP/utils"
	"github.com/labstack/echo"
	"net/http"
	"strconv"
	"sync"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/raw/cypher"

type cypherAPI struct {
	Store storage.Store
}

var TransactionNum = 1
var CypherTransaction map[int]storage.CypherTransaction
var mux sync.Mutex

func setTransaction(trans storage.CypherTransaction) int {
	mux.Lock()
	defer mux.Unlock()
	CypherTransaction[TransactionNum] = trans
	TransactionNum += 1
	return TransactionNum - 1
}
func getTransaction(transid int) (storage.CypherTransaction, error) {
	mux.Lock()
	defer mux.Unlock()

	trans, ok := CypherTransaction[transid]
	if !ok {
		return nil, fmt.Errorf("Transaction id not found")
	}
	return trans, nil
}
func deleteTransaction(transid int) {
	mux.Lock()
	defer mux.Unlock()
	delete(CypherTransaction, transid)
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &cypherAPI{mainapi.Store}
	CypherTransaction = make(map[int]storage.CypherTransaction)

	// custom endpoint
	endPoint := "cypher"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.execCypher)

	// start trans
	endPoint = "transaction"
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.startTrans)

	// commit trans
	endPoint = "transaction/:id/commit"
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.commitTrans)

	// execute trans query
	endPoint = "transaction/:id/cypher"
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.execTranCypher)

	// kill trans
	endPoint = "transaction/:id/kill"
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.killTrans)

	return nil
}

// customReq defines the input for the custom endpoint
// swagger:model customReq
type customReq struct {
	Cypher  string `json:"cypher"`
	Version string `json:"version,omitempty"`
	Dataset string `json:"dataset,omitempty"`
}

type datasetReq struct {
	Dataset string `json:"dataset"`
}

type transResp struct {
	TransId int `json:"transaction_id"`
}

// startTrans starts a transaction
func (ca cypherAPI) startTrans(c echo.Context) error {
	// swagger:operation POST /api/raw/cypher/transaction raw-cypher startTrans
	//
	// Start a cypher transaction.
	//
	// Starts and transaction and returns an id.
	//
	// ---
	// parameters:
	// - in: "body"
	//   name: "body"
	//   required: true
	//   schema:
	//     type: "object"
	//     required: ["cypher"]
	//     properties:
	//       dataset:
	//         type: "string"
	//         description: "dataset name"
	//         example: "hemibrain"
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         transaction_id:
	//           type: "integer"
	//           description: "transcation id"
	// security:
	// - Bearer: [admin]

	var req datasetReq
	if err := c.Bind(&req); err != nil {
		errJSON := api.ErrorInfo{Error: "request object not formatted correctly"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	store := ca.Store.GetMain(req.Dataset)
	trans, err := store.StartTrans()
	if err != nil {
		errJSON := api.ErrorInfo{Error: "request object not formatted correctly"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	transid := setTransaction(trans)
	resp := &transResp{transid}

	return c.JSON(http.StatusOK, resp)
}

// commitTrans commits a transaction
func (ca cypherAPI) commitTrans(c echo.Context) error {
	// swagger:operation POST /api/raw/cypher/transaction/:id/commit raw-cypher commitTrans
	//
	// Commits transaction.
	//
	// Commits and removes transaction.  If there is an error, the transaction will still be deleted.
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "id"
	//   schema:
	//     type: "integer"
	//   required: true
	//   description: "transaction id"
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: [admin]

	id := c.Param("id")
	tid, err := strconv.Atoi(id)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	trans, err := getTransaction(tid)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	defer deleteTransaction(tid)
	if err := trans.Commit(); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	successJSON := api.SuccessInfo{Msg: "committed"}
	return c.JSON(http.StatusOK, successJSON)
}

// killTrans kills a transaction
func (ca cypherAPI) killTrans(c echo.Context) error {
	// swagger:operation POST /api/raw/cypher/transaction/:id/kill raw-cypher killTrans
	//
	// Kill transaction.
	//
	// This will rollback the specified transaction.  If there is an error, the transaction will still be deleted.
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "id"
	//   schema:
	//     type: "integer"
	//   required: true
	//   description: "transaction id"
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: [admin]

	id := c.Param("id")
	tid, err := strconv.Atoi(id)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	trans, err := getTransaction(tid)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	defer deleteTransaction(tid)
	if err := trans.Kill(); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	successJSON := api.SuccessInfo{Msg: "killed"}
	return c.JSON(http.StatusOK, successJSON)
}

// execTranCypher enables custom cypher queries
func (ca cypherAPI) execTranCypher(c echo.Context) error {
	// swagger:operation POST /api/raw/cypher/transaction/:id/cypher raw-cypher execTranCypher
	//
	// Execute cypher against the main database in a transaction
	//
	// This query allows for reads and writes (admin only).
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "id"
	//   schema:
	//     type: "integer"
	//   required: true
	//   description: "transaction id"
	// - in: "body"
	//   name: "body"
	//   required: true
	//   schema:
	//     type: "object"
	//     required: ["cypher"]
	//     properties:
	//       dataset:
	//         type: "string"
	//         description: "dataset name"
	//         example: "hemibrain"
	//       cypher:
	//         type: "string"
	//         description: "cypher statement (read only)"
	//         example: "MATCH (n) RETURN n limit 1"
	//       version:
	//         type: "string"
	//         description: "specify a neuprint model version for explicit check"
	//         example: "0.5.0"
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         columns:
	//           type: "array"
	//           items:
	//             type: "string"
	//           example: ["name", "size"]
	//           description: "Name of each result column"
	//         data:
	//           type: "array"
	//           items:
	//             type: "array"
	//             items:
	//               type: "null"
	//               description: "Cell value"
	//             description: "Table row"
	//           example: [["t4", 323131], ["mi1", 232323]]
	//           description: "Table of results"
	// security:
	// - Bearer: [admin]
	var req customReq
	if err := c.Bind(&req); err != nil {
		errJSON := api.ErrorInfo{Error: "request object not formatted correctly"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	id := c.Param("id")
	tid, err := strconv.Atoi(id)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	trans, err := getTransaction(tid)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// set cypher for debugging
	c.Set("debug", req.Cypher)

	if req.Version != "" {
		sstore := ca.Store.(storage.SimpleStore)
		sversion, _ := sstore.GetVersion()
		if !utils.CheckSubsetVersion(req.Version, sversion) {
			errJSON := api.ErrorInfo{Error: "neo4j data model version incompatible"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
	}

	if data, err := trans.CypherRequest(req.Cypher, false); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

// execCypher enables custom cypher queries
func (ca cypherAPI) execCypher(c echo.Context) error {
	// swagger:operation POST /api/raw/cypher/cypher raw-cypher execCypher
	//
	// Execute cypher against the main database
	//
	// This query allows for reads and writes (admin only).
	//
	// ---
	// parameters:
	// - in: "body"
	//   name: "body"
	//   required: true
	//   schema:
	//     type: "object"
	//     required: ["cypher"]
	//     properties:
	//       dataset:
	//         type: "string"
	//         description: "dataset name"
	//         example: "hemibrain"
	//       cypher:
	//         type: "string"
	//         description: "cypher statement (read only)"
	//         example: "MATCH (n) RETURN n limit 1"
	//       version:
	//         type: "string"
	//         description: "specify a neuprint model version for explicit check"
	//         example: "0.5.0"
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         columns:
	//           type: "array"
	//           items:
	//             type: "string"
	//           example: ["name", "size"]
	//           description: "Name of each result column"
	//         data:
	//           type: "array"
	//           items:
	//             type: "array"
	//             items:
	//               type: "null"
	//               description: "Cell value"
	//             description: "Table row"
	//           example: [["t4", 323131], ["mi1", 232323]]
	//           description: "Table of results"
	// security:
	// - Bearer: [admin]
	var req customReq
	if err := c.Bind(&req); err != nil {
		errJSON := api.ErrorInfo{Error: "request object not formatted correctly"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// set cypher for debugging
	c.Set("debug", req.Cypher)

	if req.Version != "" {
		sstore := ca.Store.(storage.SimpleStore)
		sversion, _ := sstore.GetVersion()
		if !utils.CheckSubsetVersion(req.Version, sversion) {
			errJSON := api.ErrorInfo{Error: "neo4j data model version incompatible"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
	}

	if data, err := ca.Store.GetMain(req.Dataset).CypherRequest(req.Cypher, false); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
