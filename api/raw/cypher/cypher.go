package cypuer

import (
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/connectome-neuprint/neuPrintHTTP/utils"
	"github.com/labstack/echo"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/raw/cypher"

type cypherAPI struct {
	Store storage.Store
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &cypherAPI{mainapi.Store}

	// custom endpoint
	endPoint := "cypher"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint, q.execCypher)
	return nil
}

// customReq defines the input for the custom endpoint
// swagger:model customReq
type customReq struct {
	Cypher  string `json:"cypher"`
	Version string `json:"version,omitempty"`
	Dataset string `json:"dataset,omitempty"`
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

	if data, err := ca.Store.GetMain(req.Dataset).CypherRequest(req.Cypher, true); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
