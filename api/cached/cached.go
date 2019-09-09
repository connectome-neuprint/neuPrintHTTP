package cached

import (
	"encoding/json"
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
	//         roi2roi:
	//           type: "object"
	//           properties:
	//             count:
	//               type: "integer"
	//               description: "number of bodies between two ROIs"
	//             weight:
	//               type: "number"
	//               description: "weighted connection strength between two ROIs"
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

type prePost struct {
	Pre  int `json:"pre"`
	Post int `json:"post"`
}

type CountWeight struct {
	Count  int     `json:"count"`  // number of neurons in this roi connection
	Weight float32 `json:"weight"` // connection weight
}

// ExplorerROIConnectivity implements API to find how ROIs are connected
func (ca cypherAPI) getROIConnectivity_int(dataset string) (interface{}, error) {
	cypher := "MATCH (neuron :`" + dataset + "-Neuron`) RETURN neuron.bodyId AS bodyid, neuron.roiInfo AS roiInfo"
	res, err := ca.Store.CypherRequest(cypher, true)
	if err != nil {
		return nil, err
	}

	// restrict the query to the super level ROIs
	cypher2 := "MATCH (m :Meta) WHERE m.dataset=\"" + dataset + "\" RETURN m.superLevelRois AS rois"
	res2, err := ca.Store.CypherRequest(cypher2, true)
	if err != nil {
		return nil, err
	}

	superrois := make(map[string]interface{})
	if len(res2.Data) > 0 {
		roiarr := res2.Data[0][0].([]interface{})
		/*var roiarr []string
		err := json.Unmarshal([]byte(roistr), &roiarr)
		if err != nil {
			return nil, err
		}*/
		for _, roib := range roiarr {
			roi := roib.(string)
			superrois[roi] = nil
		}
	}

	roitable := make(map[string]*CountWeight)

	// grab input distribution
	for _, row := range res.Data {
		var roidata map[string]prePost
		roistr, ok := row[1].(string)
		if !ok {
			continue
		}
		err := json.Unmarshal([]byte(roistr), &roidata)
		if err != nil {
			continue
		}

		for roi, prepost := range roidata {
			if _, ok := superrois[roi]; !ok {
				continue
			}
			numout := prepost.Pre
			if numout > 0 {
				// grab total inputs
				totalin := 0
				for roi2, prepost2 := range roidata {
					if _, ok := superrois[roi2]; !ok {
						continue
					}
					totalin += prepost2.Post
				}

				if totalin > 0 {
					// weight connection by input percentage for each ROI
					for roi2, prepost2 := range roidata {
						if _, ok := superrois[roi2]; !ok {
							continue
						}
						key := roi2 + "=>" + roi
						perout := float32(numout*prepost2.Post) / float32(totalin)
						if _, ok := roitable[key]; !ok {
							roitable[key] = &CountWeight{Count: 1, Weight: perout}
						} else {
							countweight := roitable[key]
							countweight.Count += 1
							countweight.Weight += perout
						}
					}
				}
			}
		}

	}

	return roitable, err
}
