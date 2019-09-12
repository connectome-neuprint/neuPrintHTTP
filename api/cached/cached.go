package cached

import (
	"encoding/json"
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/knightjdr/hclust"
	"github.com/labstack/echo"
	//"math"
	"net/http"
	"strings"
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

type CacheType int

var cachedResults map[CacheType]map[string]interface{}

const (
	ROIConn CacheType = 1
	ROIComp CacheType = 2
)

// setupAPI loads all the endpoints for cached
func setupAPI(mainapi *api.ConnectomeAPI) error {
	if cypherEngine, ok := mainapi.Store.GetMain().(storage.Cypher); ok {
		// setup cache
		cachedResults = make(map[CacheType]map[string]interface{})

		q := &cypherAPI{cypherEngine}

		// roi conenctivity cache
		endpoint := "roiconnectivity"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getROIConnectivity)
		mainapi.SupportedEndpoints[endpoint] = true
		cachedResults[ROIConn] = make(map[string]interface{})

		// roi completeness cache (TODO: connection completeness)
		endpoint = "roicompleteness"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getROICompleteness)
		mainapi.SupportedEndpoints[endpoint] = true
		cachedResults[ROIComp] = make(map[string]interface{})

		go func() {
			for {
				data, err := mainapi.Store.GetDatasets()
				if err == nil {
					// load connections
					for dataset, _ := range data {
						// cache roi connectivity
						if res, err := q.getROIConnectivity_int(dataset); err == nil {
							mux.Lock()
							cachedResults[ROIConn][dataset] = res
							mux.Unlock()
						}

						// cache roi completeness
						if res, err := q.getROICompleteness_int(dataset); err == nil {
							mux.Lock()
							cachedResults[ROIComp][dataset] = res
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

// getROI connectivity returns how the ROIs connect to each other
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
	//         roi_names:
	//           type: "array"
	//           items:
	//             type: "string"
	//           description: "sorted roi names based on clustering"
	//         weights:
	//           type: "object"
	//           description: "adjacency list between rois"
	//           properties:
	//             "roiin=>roiout":
	//               type: "object"
	//               properties:
	//                 count:
	//                   type: "integer"
	//                   description: "number of bodies between two ROIs"
	//                 weight:
	//                   type: "number"
	//                   description: "weighted connection strength between two ROIs"
	// security:
	// - Bearer: []

	dataset := c.QueryParam("dataset")

	mux.Lock()
	if res, ok := cachedResults[ROIConn][dataset]; ok {
		mux.Unlock()
		return c.JSON(http.StatusOK, res)
	}
	mux.Unlock()

	res, err := ca.getROIConnectivity_int(dataset)
	if err != nil {
		mux.Lock()
		cachedResults[ROIConn][dataset] = res
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
	Weight float64 `json:"weight"` // connection weight
}

type SortedROI struct {
	Names   []string                `json:"roi_names"` // names in sorted order based on clustering
	Weights map[string]*CountWeight `json:"weights"`
}

const MAXVAL = 10000000000

// getROIConnectivity_int implements API to find how ROIs are connected
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

	superrois := make(map[string]int)
	roinames := make([]string, 0, 0)
	if len(res2.Data) > 0 {
		roiarr := res2.Data[0][0].([]interface{})
		/*var roiarr []string
		err := json.Unmarshal([]byte(roistr), &roiarr)
		if err != nil {
			return nil, err
		}*/
		for idx, roib := range roiarr {
			roi := roib.(string)
			roinames = append(roinames, roi)
			superrois[roi] = idx
		}
	}
	distmatrix := make([][]float64, len(superrois), len(superrois))
	for idx, _ := range distmatrix {
		distmatrix[idx] = make([]float64, len(superrois), len(superrois))
		for idx2, _ := range distmatrix[idx] {
			distmatrix[idx][idx2] = MAXVAL
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
						perout := float64(numout*prepost2.Post) / float64(totalin)
						if _, ok := roitable[key]; !ok {
							roitable[key] = &CountWeight{Count: 1, Weight: perout}
						} else {
							countweight := roitable[key]
							countweight.Count += 1
							countweight.Weight += perout
						}
						idx1 := superrois[roi2]
						idx2 := superrois[roi]
						if roitable[key].Weight < 0.001 {
							distmatrix[idx1][idx2] = MAXVAL
						} else {
							distmatrix[idx1][idx2] = MAXVAL - roitable[key].Weight
						}
					}
				}
			}
		}
	}

	// sort roi names by clustering
	subcluster, err := hclust.Cluster(distmatrix, "single")
	if err != nil {
		return nil, err
	}
	//optcluster := subcluster
	optcluster := hclust.Optimize(subcluster, distmatrix, 0)
	tree, err := hclust.Tree(optcluster, roinames)
	if err != nil {
		return nil, err
	}

	return SortedROI{Names: tree.Order, Weights: roitable}, err
}

// getROICompleteness returns the tracing completeness of each ROI
func (ca cypherAPI) getROICompleteness(c echo.Context) error {
	// swagger:operation GET /api/cached/roicompleteness cached getROICompleteness
	//
	// Gets tracing completeness for each ROI.
	//
	// The program updates the completeness numbers each day.  Completeness is defined
	// as "Traced", "Roughly traced", "Prelim Roughly traced", "final", "final (irrelevant)", "Finalized".
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
	//           example: ["roi", "roipre", "roipost", "totalpre", "totalpost"]
	//           description: "ROI stat breakdown"
	//         data:
	//           type: "array"
	//           items:
	//             type: "array"
	//             items:
	//               type: "null"
	//               description: "Cell value"
	//             description: "Completeness for a given ROI"
	//           description: "ROI completenss results"
	// security:
	// - Bearer: []

	dataset := c.QueryParam("dataset")

	mux.Lock()
	if res, ok := cachedResults[ROIComp][dataset]; ok {
		mux.Unlock()
		return c.JSON(http.StatusOK, res)
	}
	mux.Unlock()

	res, err := ca.getROICompleteness_int(dataset)
	if err != nil {
		mux.Lock()
		cachedResults[ROIComp][dataset] = res
		mux.Unlock()
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// load result
	return c.JSON(http.StatusOK, res)
}

var completeStatuses = []string{"Traced", "Roughly traced", "Prelim Roughly traced", "final", "final (irrelevant)", "Finalized", "Leaves"}

//getROICompleteness_int fetches roi completeness from database
func (ca cypherAPI) getROICompleteness_int(dataset string) (interface{}, error) {
	cypher := "MATCH (n:`" + dataset + "-Neuron`) WHERE {status_conds} WITH apoc.convert.fromJsonMap(n.roiInfo) AS roiInfo WITH roiInfo AS roiInfo, keys(roiInfo) AS roiList UNWIND roiList AS roiName WITH roiName AS roiName, sum(roiInfo[roiName].pre) AS pre, sum(roiInfo[roiName].post) AS post MATCH (meta:Meta:" + dataset + ") WITH apoc.convert.fromJsonMap(meta.roiInfo) AS globInfo, roiName AS roiName, pre AS pre, post AS post RETURN roiName AS roi, pre AS roipre, post AS roipost, globInfo[roiName].pre AS totalpre, globInfo[roiName].post AS totalpost ORDER BY roiName"

	statusarr := ""
	for index, status := range completeStatuses {
		if index == 0 {
			statusarr = statusarr + "("
		} else {
			statusarr = statusarr + " OR "
		}
		statusarr = statusarr + "n.status = \"" + status + "\""
	}
	if statusarr != "" {
		statusarr = statusarr + ")"
	}
	cypher = strings.Replace(cypher, "{status_conds}", statusarr, -1)

	return ca.Store.CypherRequest(cypher, true)
}
