package cached

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"math/rand"

	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/knightjdr/hclust"
	"github.com/labstack/echo/v4"

	//"math"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/cached"

type cypherAPI struct {
	Store storage.Store
}

type CacheType int

var cachedResults map[CacheType]map[string]interface{}
var cacheMux sync.RWMutex

var datasetLastRefresh map[string]time.Time
var refreshMux sync.RWMutex

var roiConnectivityMux sync.Mutex
var roiCompletenessMux sync.Mutex
var dailyTypeMux sync.Mutex

const (
	ROIConn   CacheType = 1
	ROIComp   CacheType = 2
	DailyType CacheType = 3
)

// setupAPI loads all the endpoints for cached
func setupAPI(mainapi *api.ConnectomeAPI) error {
	// setup cache
	cachedResults = make(map[CacheType]map[string]interface{})
	datasetLastRefresh = make(map[string]time.Time)

	q := &cypherAPI{mainapi.Store}

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

	// cell type of the data
	endpoint = "dailytype"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getDailyType)
	mainapi.SupportedEndpoints[endpoint] = true
	cachedResults[DailyType] = make(map[string]interface{})

	go func() {
		// Initial cache population on startup
		datasets, err := mainapi.Store.GetDatasets()
		if err == nil {
			now := time.Now()
			for dataset, _ := range datasets {
				// cache roi connectivity
				if _, err = q.roiConnectivity(dataset); err != nil {
					fmt.Printf("Error caching roi connectivity for dataset %s: %v\n", dataset, err)
				} else {
					fmt.Printf("Cached roi connectivity for dataset %s\n", dataset)
				}

				// cache roi completeness
				if _, err = q.roiCompleteness(dataset); err != nil {
					fmt.Printf("Error caching roi completeness for dataset %s: %v\n", dataset, err)
				} else {
					fmt.Printf("Cached roi completeness for dataset %s\n", dataset)
				}

				// cache daily type
				if _, err = q.dailyType(dataset); err != nil {
					fmt.Printf("Error caching daily type for dataset %s: %v\n", dataset, err)
				} else {
					fmt.Printf("Cached daily type for dataset %s\n", dataset)
				}
				
				// Mark this dataset as refreshed
				refreshMux.Lock()
				datasetLastRefresh[dataset] = now
				refreshMux.Unlock()
			}
		}

		// Check for stale cache every hour
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		
		for range ticker.C {
			datasets, err := mainapi.Store.GetDatasets()
			if err != nil {
				continue
			}
			
			now := time.Now()
			for dataset := range datasets {
				refreshMux.RLock()
				lastRefresh, exists := datasetLastRefresh[dataset]
				refreshMux.RUnlock()
				
				if !exists {
					continue // Skip if we don't have refresh info
				}
				
				// Add random jitter: 24 hours + (0-10) * 10 minutes
				jitterMinutes := rand.Intn(11) * 10 // 0, 10, 20, ..., 100 minutes
				refreshThreshold := 24*time.Hour + time.Duration(jitterMinutes)*time.Minute
				
				if now.Sub(lastRefresh) >= refreshThreshold {
					fmt.Printf("Cache expired for dataset %s (age: %v), clearing cache\n", 
						dataset, now.Sub(lastRefresh))
					
					// Clear this dataset's cache
					cacheMux.Lock()
					delete(cachedResults[ROIConn], dataset)
					delete(cachedResults[ROIComp], dataset) 
					delete(cachedResults[DailyType], dataset)
					cacheMux.Unlock()
					
					// Don't update refresh time - let it refresh on next access
				}
			}
		}
	}()

	return nil
}

// returns how the ROIs connect to each other
func (ca cypherAPI) roiConnectivity(dataset string) (res interface{}, err error) {
	roiConnectivityMux.Lock() // Only one roiConnectivity request at a time
	defer roiConnectivityMux.Unlock()

	cacheMux.RLock()
	var ok bool
	if res, ok = cachedResults[ROIConn][dataset]; ok {
		cacheMux.RUnlock()
		return
	}
	cacheMux.RUnlock()

	res, err = ca.getROIConnectivity_int(dataset)
	if err != nil {
		return nil, err
	}

	cacheMux.Lock()
	cachedResults[ROIConn][dataset] = res
	cacheMux.Unlock()
	
	// Update refresh timestamp when we populate cache
	refreshMux.Lock()
	datasetLastRefresh[dataset] = time.Now()
	refreshMux.Unlock()
	
	return
}

// getROIConnectivity provides web handler for how the ROIs connect to each other
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

	res, err := ca.roiConnectivity(dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
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
	cypher := `
		MATCH (neuron :Neuron)
		RETURN
			toString(neuron.bodyId) AS bodyid,
			neuron.roiInfo AS roiInfo
	`
	res, err := ca.Store.GetMain(dataset).CypherRequest(cypher, true)
	if err != nil {
		return nil, err
	}

	// Restrict results to the overview ROIs.
	// If Meta.overviewRois is present, use that list.
	// Otherwise, use the primaryRois by default.
	// But
	cypher2 := `
		MATCH (meta :Meta)
		WITH
			CASE meta.overviewRois
				WHEN NULL THEN meta.primaryRois
				ELSE meta.overviewRois
			END AS overviewRois,
			meta,
			apoc.convert.fromJsonMap(meta.roiInfo) AS roiInfo
		UNWIND overviewRois as roi
		WITH roiInfo, roi
		WHERE NOT coalesce(roiInfo[roi]['excludeFromOverview'], FALSE)
		RETURN collect(roi) as rois
	`

	res2, err := ca.Store.GetMain(dataset).CypherRequest(cypher2, true)
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
						// skip in=>out if there are no inputs from this ROI
						if prepost2.Post == 0 {
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

	cypher3 := `
	MATCH (m:Meta)
	RETURN
		CASE m.overviewOrder
			WHEN NULL THEN 'clustered'
			ELSE m.overviewOrder
		END AS overviewOrder
	`
	res3, err := ca.Store.GetMain(dataset).CypherRequest(cypher3, true)
	if err != nil {
		return nil, err
	}
	var overviewOrder string
	if len(res3.Data) > 0 {
		overviewOrder = res3.Data[0][0].(string)
	}

	// If the dataset wants the overview ROIs to be auto-ordered,
	// then use clustering to find the order.
	// Otherwise, stick with the order given by Meta.overviewRois.
	if len(distmatrix) > 3 && overviewOrder == "clustered" {
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
	} else {
		return SortedROI{Names: roinames, Weights: roitable}, err
	}
}

// roiCompleteness returns the tracing completeness of each ROI
func (ca cypherAPI) roiCompleteness(dataset string) (res interface{}, err error) {
	roiCompletenessMux.Lock() // Only one roiCompleteness request at a time
	defer roiCompletenessMux.Unlock()

	cacheMux.RLock()
	var ok bool
	if res, ok = cachedResults[ROIComp][dataset]; ok {
		cacheMux.RUnlock()
		return
	}
	cacheMux.RUnlock()

	res, err = ca.getROICompleteness_int(dataset)
	if err != nil {
		return nil, err
	}

	cacheMux.Lock()
	cachedResults[ROIComp][dataset] = res
	cacheMux.Unlock()
	
	// Update refresh timestamp when we populate cache
	refreshMux.Lock()
	datasetLastRefresh[dataset] = time.Now()
	refreshMux.Unlock()

	return
}

// getROICompleteness is web handler that provides the tracing completeness of each ROI
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

	res, err := ca.roiCompleteness(dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	return c.JSON(http.StatusOK, res)
}

var completeStatuses = []string{"Traced", "Roughly traced", "Prelim Roughly traced", "final", "final (irrelevant)", "Finalized", "Leaves"}

// getROICompleteness_int fetches roi completeness from database
func (ca cypherAPI) getROICompleteness_int(dataset string) (interface{}, error) {
	cypher := "MATCH (n:Neuron) WHERE {status_conds} WITH apoc.convert.fromJsonMap(n.roiInfo) AS roiInfo WITH roiInfo AS roiInfo, keys(roiInfo) AS roiList UNWIND roiList AS roiName WITH roiName AS roiName, sum(roiInfo[roiName].pre) AS pre, sum(roiInfo[roiName].post) AS post MATCH (meta:Meta) WITH apoc.convert.fromJsonMap(meta.roiInfo) AS globInfo, roiName AS roiName, pre AS pre, post AS post RETURN roiName AS roi, pre AS roipre, post AS roipost, globInfo[roiName].pre AS totalpre, globInfo[roiName].post AS totalpost ORDER BY roiName"

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

	return ca.Store.GetMain(dataset).CypherRequest(cypher, true)
}

type SkeletonResp struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

// daily type returns information for a different neeuron each day
func (ca cypherAPI) dailyType(dataset string) (res []byte, err error) {
	dailyTypeMux.Lock() // Only one dailyType request at a time
	defer dailyTypeMux.Unlock()

	cacheMux.RLock()
	if resc, ok := cachedResults[DailyType][dataset]; ok {
		res, _ = resc.([]byte)
		cacheMux.RUnlock()
		return
	}
	cacheMux.RUnlock()

	res, err = ca.getDailyType_int(dataset)
	if err != nil {
		return nil, err
	}

	cacheMux.Lock()
	cachedResults[DailyType][dataset] = res
	cacheMux.Unlock()
	
	// Update refresh timestamp when we populate cache
	refreshMux.Lock()
	datasetLastRefresh[dataset] = time.Now()
	refreshMux.Unlock()

	return
}

// getDailyType is web handler that provides the information for a different neuron each day
func (ca cypherAPI) getDailyType(c echo.Context) error {
	// swagger:operation GET /api/cached/dailytype cached getDailyType
	//
	// Gets information for a different neuron type each day.
	//
	// The program updates the completeness numbers each day.  A different
	// cell type is randomly picked and an exemplar is chosen
	// from this type.
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
	//         connectivity:
	//           type: "object"
	//           description: "connectivity breakdown"
	//         info:
	//           type: "object"
	//           properties:
	//             typename:
	//               type: "string"
	//             numtype:
	//               type: "integer"
	//             numpre:
	//               type: "integer"
	//             numpost:
	//               type: "integer"
	//             bodyid:
	//               type: "integer"
	//           description: "information on the type and neuron id"
	//         skeleton:
	//           type: "string"
	//           description: "SWC contents for the chosen neuron"
	// security:
	// - Bearer: []

	dataset := c.QueryParam("dataset")

	res, err := ca.dailyType(dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	c.Response().Header().Set("Content-Encoding", "gzip")
	return c.Blob(http.StatusOK, "application/json", res)
}

func (ca cypherAPI) getDailyType_int(dataset string) ([]byte, error) {
	requester := ca.Store.GetMain(dataset)

	// find a random cell typee
	random_query := "MATCH (n :Neuron) WHERE (n.cropped IS NULL OR not n.cropped) AND n.status IN [\"Traced\",\"Anchor\"] WITH percentileDisc(n.pre, 0.2) AS prethres, percentileDisc(n.post, 0.2) AS postthres MATCH (n :Neuron) WHERE (n.cropped IS NULL OR not n.cropped) AND n.status IN [\"Traced\",\"Anchor\"] AND EXISTS(n.type) AND n.type<>\"\" AND (n.pre > prethres OR n.post > postthres) WITH n.type as type, collect(n.bodyId) as bodylist WITH type, rand() AS randvar RETURN type ORDER BY randvar LIMIT 1"

	rand_res, err := requester.CypherRequest(random_query, true)
	if err != nil {
		return nil, err
	}

	if len(rand_res.Data) == 0 {
		return nil, fmt.Errorf("no cell type exists")
	}

	typename, ok := rand_res.Data[0][0].(string)
	if !ok {
		return nil, fmt.Errorf("cell type could not be parsed")
	}

	// get an exemplar body
	biggest_query := "MATCH (n :Neuron {type: \"{typename}\"}) RETURN n.bodyId, n.pre, n.post ORDER BY n.pre*5+n.post DESC LIMIT 1"
	biggest_query = strings.Replace(biggest_query, "{typename}", typename, -1)

	ex_res, err := requester.CypherRequest(biggest_query, true)
	if err != nil {
		return nil, err
	}

	if len(ex_res.Data) == 0 {
		return nil, fmt.Errorf("no bodies exist for cell type")
	}

	var bodyid, numpre, numpost int64

	switch v := ex_res.Data[0][0].(type) {
	case int64:
		bodyid = v
	case int32:
		bodyid = int64(v)
	default:
		return nil, fmt.Errorf("body id is not an int: %T", v)
	}

	switch v := ex_res.Data[0][1].(type) {
	case int64:
		numpre = v
	case int32:
		numpre = int64(v)
	default:
		return nil, fmt.Errorf("presyn number is not an int: %T", v)
	}

	switch v := ex_res.Data[0][2].(type) {
	case int64:
		numpost = v
	case int32:
		numpost = int64(v)
	default:
		return nil, fmt.Errorf("postsyn number is not an int: %T", v)
	}

	// get body count
	count_query := "MATCH (n :Neuron {type: \"{typename}\"}) RETURN count(n)"
	count_query = strings.Replace(count_query, "{typename}", typename, -1)

	count_res, err := requester.CypherRequest(count_query, true)
	if err != nil {
		return nil, err
	}
	numtype, ok := count_res.Data[0][0].(int64)
	if !ok {
		return nil, fmt.Errorf("number of neurons could not be parsed: %T", count_res.Data[0][0])
	}

	// fetch connection info (for sunburst plot)
	connection_info := "MATCH (n :Neuron {bodyId: {bodyid}})-[x :ConnectsTo]->(m) RETURN toString(m.bodyId) as bodyId, m.type, x.weight, x.roiInfo, m.status, 'downstream' as direction UNION MATCH (n :Neuron {bodyId: {bodyid}})<-[x :ConnectsTo]-(m) RETURN toString(m.bodyId) as bodyId, m.type, x.weight, x.roiInfo, m.status, 'upstream' as direction"
	connection_info = strings.Replace(connection_info, "{bodyid}", strconv.FormatInt(bodyid, 10), -1)

	conninfo_res, err := requester.CypherRequest(connection_info, true)
	if err != nil {
		return nil, err
	}

	// fetch skeleton or empty string
	skeleton := &SkeletonResp{}

	// get key value store
	store, err := ca.Store.FindStore("skeletons", dataset)
	if err == nil {
		kvstore, ok := store.(storage.KeyValue)
		if !ok {
			return nil, fmt.Errorf("database doesn't support keyvalue")
		}

		// fetch the value
		keystr := strconv.FormatInt(bodyid, 10) + "_swc"
		res, err := kvstore.Get([]byte(keystr))
		fmt.Printf("skeleton for daily type example: %s\n", keystr)
		if err != nil {
			fmt.Printf("error fetching skeleton: %s\n", err.Error())
		}
		fmt.Printf("skeleton size retrieved: %d\n", len(res))

		if err == nil && len(res) > 0 {
			// copied from skeleton API
			// parse and write out json
			buffer := bytes.NewBuffer(res)

			data := make([][]interface{}, 0)
			columns := []string{"rowId", "x", "y", "z", "radius", "link"}
			for {
				line, err := buffer.ReadString('\n')
				if err != nil {
					break
				}

				entries := strings.Fields(line)
				if len(entries) == 0 {
					continue
				}
				// skip comments
				if entries[0][0] == '#' {
					continue
				}

				if len(entries) != 7 {
					return nil, fmt.Errorf("SWC not formatted properly")
				}

				rownum, _ := strconv.Atoi(entries[0])
				xloc, _ := strconv.ParseFloat(entries[2], 64)
				yloc, _ := strconv.ParseFloat(entries[3], 64)
				zloc, _ := strconv.ParseFloat(entries[4], 64)
				radius, _ := strconv.ParseFloat(entries[5], 64)
				link, _ := strconv.Atoi(entries[6])

				data = append(data, []interface{}{rownum, xloc, yloc, zloc, radius, link})
			}
			skeleton = &SkeletonResp{columns, data}
		} else {
			skeleton = nil
		}
	} else {
		skeleton = nil
	}

	output := make(map[string]interface{})
	output["connectivity"] = conninfo_res

	info := make(map[string]interface{})
	info["typename"] = typename
	info["numtype"] = numtype
	info["numpre"] = numpre
	info["numpost"] = numpost
	info["bodyid"] = strconv.FormatInt(bodyid, 10)
	output["info"] = info
	output["skeleton"] = skeleton

	// write to json string and compress to gzip
	json_output, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(json_output))
	w.Close()

	return b.Bytes(), nil

}
