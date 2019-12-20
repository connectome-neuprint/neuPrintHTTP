package npexplorer

import (
	"encoding/json"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"io"
	"net/http"
	"os/exec"
	"strconv"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/npexplorer"

type cypherAPI struct {
	Store storage.Store
}

// setupAPI sets up the optionally supported explorer endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &cypherAPI{mainapi.Store}

	endPoint := "findneurons"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getFindNeurons)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getFindNeurons)
	endPoint = "neuronmetavals"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getNeuronMetaVals)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getNeuronMetaVals)
	endPoint = "neuronmeta"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getNeuronMeta)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getNeuronMeta)
	endPoint = "roiconnectivity"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getROIConnectivity)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getROIConnectivity)
	endPoint = "rankedtable"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getRankedTable)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getRankedTable)
	endPoint = "simpleconnections"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getSimpleConnections)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getSimpleConnections)
	endPoint = "roisinneuron"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getROIsInNeuron)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getROIsInNeuron)
	endPoint = "commonconnectivity"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getCommonConnectivity)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getCommonConnectivity)
	endPoint = "autapses"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getAutapses)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getAutapses)
	endPoint = "distribution"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getDistribution)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getDistribution)
	endPoint = "completeness"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getCompleteness)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getCompleteness)
	endPoint = "celltype"
	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint+"/:dataset/:type", q.getCellType)
	mainapi.SetRoute(api.POST, PREFIX+"/"+endPoint+"/:dataset/:type", q.getCellType)
	return nil
}

type errorInfo struct {
	Error string `json:"error"`
}

func (ca *cypherAPI) getFindNeurons(c echo.Context) error {
	var reqObject FindNeuronsParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerFindNeurons(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getNeuronMetaVals(c echo.Context) error {
	var reqObject MetaValParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerNeuronMetaVals(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getNeuronMeta(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerNeuronMeta(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getROIConnectivity(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerROIConnectivity(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getRankedTable(c echo.Context) error {
	var reqObject ConnectionsParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerRankedTable(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getCellType(c echo.Context) error {
	// swagger:operation GET /api/npexplorer/celltype/{dataset}/{type} npexplorer getCellType
	//
	// Get cell type connectivity information
	//
	// Examines connectivity for every neuron instance of this type and tries
	// to determine a canonical connectivity.
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "dataset"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "dataset name"
	// - in: "path"
	//   name: "type"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "cell type"
	// responses:
	//   200:
	//     description: "JSON results for neurons that make up the given cell type"
	//     schema:
	//       type: "object"
	// security:
	// - Bearer: []

	dataset := c.Param("dataset")
	celltype := c.Param("type")

	if dataset == "" || celltype == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	cypher := "MATCH (n :Neuron {type: \"" + celltype + "\"})-[x :ConnectsTo]-(m) RETURN n.bodyId AS bodyId, n.instance AS instance, x.weight AS weight, m.bodyId AS bodyId2, m.type AS type2, (startNode(x) = n) as isOutput, n.status AS body1status, m.status AS body2status, m.cropped AS iscropped2, n.cropped AS iscropped1"

	res, err := ca.Store.GetMain(dataset).CypherRequest(cypher, true)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// nothing exists
	if len(res.Data) == 0 {
		errJSON := api.ErrorInfo{Error: "no cell type exists"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// constants
	primary_status := "Traced"
	minweight := 3

	neuron_instance := make(map[int64]string)
	unique_neurons := make(map[int64]struct{})
	good_neurons := make(map[int64]struct{})
	output_size := make(map[int64]int)
	input_size := make(map[int64]int)
	output_comp := make(map[int64]int)
	input_comp := make(map[int64]int)
	celltype_lists_outputs := make(map[int64][]interface{})
	celltype_lists_inputs := make(map[int64][]interface{})

	for _, row := range res.Data {

		bodyid_t, ok := row[0].(float64)
		if !ok {
			errJSON := api.ErrorInfo{Error: "body id1 not parsed properly"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
		bodyid := int64(bodyid_t)

		bodyid2_t, ok := row[3].(float64)
		if !ok {
			errJSON := api.ErrorInfo{Error: "body id2 not parsed properly"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
		bodyid2 := int64(bodyid2_t)

		instance, ok := row[1].(string)
		if !ok {
			instance = ""
		}

		weight_t, ok := row[2].(float64)
		if !ok {
			errJSON := api.ErrorInfo{Error: "weight not parsed properly"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
		weight := int(weight_t)

		type_status, ok := row[6].(string)
		if !ok {
			type_status = ""
		}

		type_status2, ok := row[7].(string)
		if !ok {
			type_status2 = ""
		}

		is_output, ok := row[5].(bool)
		if !ok {
			errJSON := api.ErrorInfo{Error: "output direction not parsed properly"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}

		conntype, ok := row[4].(string)
		if !ok {
			conntype = ""
		}

		is_cropped2, ok := row[8].(bool)
		if !ok {
			is_cropped2 = false
		}

		is_cropped1, ok := row[9].(bool)
		if !ok {
			is_cropped1 = false
		}

		neuron_instance[bodyid] = instance

		// ingore neuron types have not been traced in any way:
		if type_status != primary_status {
			continue
		}

		unique_neurons[bodyid] = struct{}{}

		if is_output {
			if _, exists := output_size[bodyid]; !exists {
				output_size[bodyid] = 0
			}
			output_size[bodyid] += weight
		}
		if !is_output {
			if _, exists := input_size[bodyid]; !exists {
				input_size[bodyid] = 0
			}
			input_size[bodyid] += weight
		}

		// might as well ignore connection as well if not to traced
		if type_status2 != primary_status {
			continue
		}

		// add stats if traced
		if is_output {
			if _, exists := output_comp[bodyid]; !exists {
				output_comp[bodyid] = 0
			}
			output_comp[bodyid] += weight
		}
		if !is_output {
			if _, exists := input_comp[bodyid]; !exists {
				input_comp[bodyid] = 0
			}
			input_comp[bodyid] += weight
		}

		hastype := true
		if conntype == "" {
			conntype = strconv.Itoa(int(bodyid2))
			hastype = false
		}

		// don't consider the edge for something that is leaves and has not type
		if !hastype && is_cropped2 {
			continue
		}

		//  don't consider a weak edge
		if weight < minweight {
			continue
		}

		// if type_status in connection_status:
		if !is_cropped1 {
			/*
				// make sure name exclusions are not in the instance name
				//name_exclusions := ".*_L"
				// hack for hemibrain
				if len(dataset) >= len("hemibrain") && ("hemibrain" == dataset[0:len("hemibrain")]) {
					if len(instance) < 2 || instance[len(instance)-1] != 'L' || instance[len(instance)-2] != '_' {
						good_neurons[bodyid] = struct{}{}
					}
				} else {*/
			good_neurons[bodyid] = struct{}{}
			//}
		}

		if is_output {
			arritem := [...]interface{}{weight, bodyid2, conntype, hastype}
			arr, exists := celltype_lists_outputs[bodyid]
			if !exists {
				arr = make([]interface{}, 0, 1)
			}
			arr = append(arr, arritem)
			celltype_lists_outputs[bodyid] = arr
		} else {
			arritem := [...]interface{}{weight, bodyid2, conntype, hastype}
			arr, exists := celltype_lists_inputs[bodyid]
			if !exists {
				arr = make([]interface{}, 0, 1)
			}
			arr = append(arr, arritem)
			celltype_lists_inputs[bodyid] = arr
		}
	}

	// save results to map
	unique_neurons_list := make([]int64, 0, len(unique_neurons))
	good_neurons_list := make([]int64, 0, len(good_neurons))
	for body, _ := range unique_neurons {
		unique_neurons_list = append(unique_neurons_list, body)
	}
	for body, _ := range good_neurons {
		good_neurons_list = append(good_neurons_list, body)
	}

	pythondata := make(map[string]interface{})
	pythondata["neuron_instance"] = neuron_instance
	pythondata["unique_neurons"] = unique_neurons_list
	pythondata["good_neurons"] = good_neurons_list
	pythondata["output_size"] = output_size
	pythondata["input_size"] = input_size
	pythondata["output_comp"] = output_comp
	pythondata["input_comp"] = input_comp
	pythondata["celltype_lists_outputs"] = celltype_lists_outputs
	pythondata["celltype_lists_inputs"] = celltype_lists_inputs

	pdstring, _ := json.Marshal(pythondata)

	// call python function
	//cmd := exec.Command("echo", "'"+string(pdstring)+"'", "|", "python", "-W", "ignore", "canonical_celltype.py")
	cmd := exec.Command("python", "-W", "ignore", "canonical_celltype.py")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, string(pdstring))
	}()

	res2, err := cmd.CombinedOutput()

	if err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	// return json
	return c.JSONBlob(http.StatusOK, res2)

}

func (ca *cypherAPI) getSimpleConnections(c echo.Context) error {
	var reqObject ConnectionsParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerSimpleConnections(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getROIsInNeuron(c echo.Context) error {
	var reqObject NeuronNameParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerROIsInNeuron(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getCommonConnectivity(c echo.Context) error {
	var reqObject CommonConnectivityParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerCommonConnectivity(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getAutapses(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerAutapses(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getDistribution(c echo.Context) error {
	var reqObject DistributionParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerDistribution(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (ca *cypherAPI) getCompleteness(c echo.Context) error {
	var reqObject CompletenessParams
	c.Bind(&reqObject)
	if data, err := ca.ExplorerCompleteness(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
