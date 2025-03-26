package custom

import (
	"net/http"

	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/connectome-neuprint/neuPrintHTTP/utils"
	"github.com/labstack/echo/v4"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/custom"

// ConnectomeCustom is the main API object for custom endpoints
type ConnectomeCustom struct {
	api *api.ConnectomeAPI
}

type cypherAPI struct {
	Store storage.Store
}

// NewArrowFlightServer creates a new Arrow Flight server with the given store
func (cc *ConnectomeCustom) NewArrowFlightServer(store storage.Store) interface{} {
	return &FlightService{
		Store: store,
	}
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &cypherAPI{mainapi.Store}

	// custom endpoint
	endPoint := "custom"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getCustom)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getCustom)
	
	// Arrow IPC stream endpoint
	arrowEndpoint := "arrow"
	mainapi.SupportedEndpoints[arrowEndpoint] = true
	mainapi.SetRoute(api.GET, PREFIX+"/"+arrowEndpoint, q.getCustomArrow)
	mainapi.SetRoute(api.POST, PREFIX+"/"+arrowEndpoint, q.getCustomArrow)
	
	// Add swagger documentation for the Arrow endpoint
	mainapi.AddSwaggerDefinition("ArrowResponse", "Apache Arrow IPC Stream format response containing query results")
	mainapi.AddSwaggerTag("arrow", "Apache Arrow", "Endpoints returning data in Apache Arrow format")
	
	// Create and return the ConnectomeCustom object
	customAPI := &ConnectomeCustom{api: mainapi}
	mainapi.Package = customAPI
	
	return nil
}

// customReq defines the input for the custom endpoint
// swagger:model customReq
type customReq struct {
	Cypher  string `json:"cypher"`
	Version string `json:"version,omitempty"`
	Dataset string `json:"dataset,omitempty"`
}

// getCustom enables custom cypher queries
func (ca cypherAPI) getCustom(c echo.Context) error {
	// swagger:operation GET /api/custom/custom custom custom
	//
	// Make custom cypher query against the database (read only)
	//
	// Endpoint expects valid cypher and returns rows of data.
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
	// - Bearer: []
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

	// set dataset for logging
	c.Set("dataset", req.Dataset)

	cypher, err := ca.Store.GetDataset(req.Dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusNotFound, errJSON)
	}
	if data, err := cypher.CypherRequest(req.Cypher, true); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
