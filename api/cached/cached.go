package cached

import (
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"net/http"
	"sync"
	"time"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/cached"

type cypherAPI struct {
	Store storage.Cypher
}

var mux sync.Mutex

var cachedResults map[string]interface{}

// setupAPI loads all the endpoints for cached
func setupAPI(mainapi *api.ConnectomeAPI) error {
	if cypherEngine, ok := mainapi.Store.GetMain().(storage.Cypher); ok {
		// setup cache
		cachedResults = make(map[string]interface{})

		q := &cypherAPI{cypherEngine}

		// roi conenctivity cache

		endpoint := "roiconnectivity"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getROIConnectivity)
		mainapi.SupportedEndpoints[endpoint] = true

		go func() {
			for {
				data, err := mainapi.Store.GetDatasets()
				if err == nil {
					// load connections
					for dataset, _ := range data {
						if res, err := q.getROIConnectivity_int(dataset); err == nil {
							mux.Lock()
							cachedResults[dataset] = res
							mux.Unlock()
						}
					}
				}
				// reset cache every day
				time.Sleep(24 * time.Hour)
			}
		}()

	} else {
		// cypher interface is required by default
		return fmt.Errorf("Cypher interface is not available")
	}

	return nil
}

type dbVersion struct {
	Version string
}

// getVersion returns the version of the database
func (ca cypherAPI) getROIConnectivity(c echo.Context) error {
	// swagger:operation GET /api/cached/roiconnectivity cached getROIConnectivity
	//
	// Gets cached synapse connection projections for all neurons.
	//
	// The program caches the region connections for each neuron updating everyday.
	//
	// ---
	// parameters:
	// - in: "query"
	//   name: "dataset"
	//   description: "specify dataset name"
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
	//           example: ["bodyid", "roiInfo"]
	//           description: "body and roi info"
	//         data:
	//           type: "array"
	//           items:
	//             type: "array"
	//             items:
	//               type: "null"
	//               description: "Cell value"
	//             description: "Table row (integer body id and json string for roi info)"
	// security:
	// - Bearer: []

	dataset := c.QueryParam("dataset")

	mux.Lock()
	if res, ok := cachedResults[dataset]; ok {
		mux.Unlock()
		return c.JSON(http.StatusOK, res)
	}
	mux.Unlock()

	res, err := ca.getROIConnectivity_int(dataset)
	if err != nil {
		mux.Lock()
		cachedResults[dataset] = res
		mux.Unlock()
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// load result
	return c.JSON(http.StatusOK, res)
}

// ExplorerROIConnectivity implements API to find how ROIs are connected
func (ca cypherAPI) getROIConnectivity_int(dataset string) (res interface{}, err error) {
	cypher := "MATCH (neuron :`" + dataset + "-Neuron`) RETURN neuron.bodyId AS bodyid, neuron.roiInfo AS roiInfo"
	return ca.Store.CypherRequest(cypher, true)
}
