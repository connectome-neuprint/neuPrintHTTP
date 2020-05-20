package cached

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/knightjdr/hclust"
	"github.com/labstack/echo"
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

var mux sync.Mutex

type CacheType int

var cachedResults map[CacheType]map[string]interface{}

const (
	ROIConn   CacheType = 1
	ROIComp   CacheType = 2
	DailyType CacheType = 3
)

// setupAPI loads all the endpoints for cached
func setupAPI(mainapi *api.ConnectomeAPI) error {
	// setup cache
	cachedResults = make(map[CacheType]map[string]interface{})

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

					// cache daily type
					if res, err := q.getDailyType_int(dataset); err == nil {
						mux.Lock()
						cachedResults[DailyType][dataset] = res
						mux.Unlock()
					}

				}
			}
			// reset cache every day
			time.Sleep(24 * time.Hour)
		}
	}()

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
	if err == nil {
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
	cypher := "MATCH (neuron :Neuron) RETURN neuron.bodyId AS bodyid, neuron.roiInfo AS roiInfo"
	res, err := ca.Store.GetMain(dataset).CypherRequest(cypher, true)
	if err != nil {
		return nil, err
	}

	// restrict the query to the super level ROIs
	cypher2 := "MATCH (m :Meta) RETURN m.superLevelRois AS rois"
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

	if len(distmatrix) > 3 {
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

// getDailyType returns information for a different neeuron each day
func (ca cypherAPI) getDailyType(c echo.Context) error {
	// swagger:operation GET /api/cached/dailytype cached getDailyType
	//
	// Gets information for a different neuron type each day..
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

	mux.Lock()
	if res, ok := cachedResults[DailyType][dataset]; ok {
		mux.Unlock()
		c.Response().Header().Set("Content-Encoding", "gzip")
		resc, _ := res.([]byte)
		return c.Blob(http.StatusOK, "application/json", resc)
	}
	mux.Unlock()

	res, err := ca.getDailyType_int(dataset)
	if err != nil {
		mux.Lock()
		cachedResults[DailyType][dataset] = res
		mux.Unlock()
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	c.Response().Header().Set("Content-Encoding", "gzip")
	return c.Blob(http.StatusOK, "application/json", res)
}

func (ca cypherAPI) getDailyType_int(dataset string) ([]byte, error) {
	requester := ca.Store.GetMain(dataset)

	// find a random cell typee
	random_query := "MATCH (n :Neuron {status: \"Traced\"}) WHERE not n.cropped WITH percentileDisc(n.pre, 0.2) AS prethres, percentileDisc(n.post, 0.2) AS postthres MATCH (n :Neuron {status: \"Traced\"}) WHERE not n.cropped AND EXISTS(n.type) AND n.type<>\"\" AND (n.pre > prethres OR n.post > postthres) WITH n.type as type, collect(n.bodyId) as bodylist WITH type, rand() AS randvar RETURN type ORDER BY randvar LIMIT 1"

	rand_res, err := requester.CypherRequest(random_query, true)
	if err != nil {
		return nil, err
	}

	if len(rand_res.Data) == 0 {
		return nil, fmt.Errorf("No cell type exists")
	}

	typename, ok := rand_res.Data[0][0].(string)
	if !ok {
		return nil, fmt.Errorf("Cell type could not be parsed")
	}

	// get an exemplar body
	biggest_query := "MATCH (n :Neuron {type: \"{typename}\"}) RETURN n.bodyId, n.pre, n.post ORDER BY n.pre*5+n.post DESC LIMIT 1"
	biggest_query = strings.Replace(biggest_query, "{typename}", typename, -1)

	ex_res, err := requester.CypherRequest(biggest_query, true)
	if err != nil {
		return nil, err
	}

	if len(ex_res.Data) == 0 {
		return nil, fmt.Errorf("No bodies exist for cell type")
	}

	bodyidf, ok := ex_res.Data[0][0].(float64)
	if !ok {
		return nil, fmt.Errorf("Body id could not be parsed")
	}
	bodyid := int(bodyidf)

	numpref, ok := ex_res.Data[0][1].(float64)
	if !ok {
		return nil, fmt.Errorf("pre could not be parsed")
	}
	numpre := int(numpref)

	numpostf, ok := ex_res.Data[0][2].(float64)
	if !ok {
		return nil, fmt.Errorf("post could not be parsed")
	}
	numpost := int(numpostf)

	// get body count
	count_query := "MATCH (n :Neuron {type: \"{typename}\"}) RETURN count(n)"
	count_query = strings.Replace(count_query, "{typename}", typename, -1)

	count_res, err := requester.CypherRequest(count_query, true)
	if err != nil {
		return nil, err
	}
	numtypef, ok := count_res.Data[0][0].(float64)
	if !ok {
		return nil, fmt.Errorf("Number of neurons could not be parsed")
	}
	numtype := int(numtypef)

	// fetch connection info (for sunburst plot)
	connection_info := "MATCH (n :Neuron {bodyId: {bodyid}})-[x :ConnectsTo]->(m) RETURN m.bodyId, m.type, x.weight, x.roiInfo, m.status, 'downstream' as direction UNION MATCH (n :Neuron {bodyId: {bodyid}})<-[x :ConnectsTo]-(m) RETURN m.bodyId, m.type, x.weight, x.roiInfo, m.status, 'upstream' as direction"
	connection_info = strings.Replace(connection_info, "{bodyid}", strconv.Itoa(bodyid), -1)

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
		keystr := strconv.Itoa(bodyid) + "_swc"
		res, err := kvstore.Get([]byte(keystr))

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
	info["bodyid"] = bodyid
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
