package npexplorer

import (
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/labstack/echo"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

// list of endpoints corresponding to neuprint explorer plugins
var ENDPOINTS = [...]string{"findneurons", "neuronmeta", "neuronmetavals", "roiconnectivity", "rankedtable", "simpleconnections", "roisinneuron", "commonconnectivity", "autapses", "distribution", "completeness"}

const PREFIX = "/npexplorer"

type explorerQuery struct {
	engine StorageAPI
}

// setupAPI sets up the optionally supported explorer endpoints
func setupAPI(c *api.ConnectomeAPI) error {
	if expInt, ok := c.Store.(StorageAPI); ok {
		q := &explorerQuery{expInt}
		for _, endPoint := range ENDPOINTS {
			c.SupportedEndpoints[endPoint] = true
			switch endPoint {
			case "findneurons":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getFindNeurons)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getFindNeurons)
			case "neuronmetavals":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getNeuronMetaVals)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getNeuronMetaVals)
			case "neuronmeta":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getNeuronMeta)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getNeuronMeta)
			case "roiconnectivity":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getROIConnectivity)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getROIConnectivity)
			case "rankedtable":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getRankedTable)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getRankedTable)
			case "simpleconnections":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getSimpleConnections)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getSimpleConnections)
			case "roisinneuron":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getROIsInNeuron)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getROIsInNeuron)
			case "commonconnectivity":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getCommonConnectivity)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getCommonConnectivity)
			case "autapses":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getAutapses)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getAutapses)
			case "distribution":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getDistribution)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getDistribution)
			case "completeness":
				c.SetRoute(api.GET, PREFIX+"/"+endPoint, q.getCompleteness)
				c.SetRoute(api.POST, PREFIX+"/"+endPoint, q.getCompleteness)
			default:
				return fmt.Errorf("Endpoint definition not found")
			}
		}
	}
	return nil
}

type errorInfo struct {
	Error string `json:"error"`
}

func (exp *explorerQuery) getFindNeurons(c echo.Context) error {
	var reqObject FindNeuronsParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerFindNeurons(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getNeuronMetaVals(c echo.Context) error {
	var reqObject MetaValParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerNeuronMetaVals(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getNeuronMeta(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerNeuronMeta(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getROIConnectivity(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerROIConnectivity(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getRankedTable(c echo.Context) error {
	var reqObject ConnectionsParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerRankedTable(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getSimpleConnections(c echo.Context) error {
	var reqObject ConnectionsParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerSimpleConnections(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getROIsInNeuron(c echo.Context) error {
	var reqObject NeuronNameParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerROIsInNeuron(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getCommonConnectivity(c echo.Context) error {
	var reqObject CommonConnectivityParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerCommonConnectivity(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getAutapses(c echo.Context) error {
	var reqObject DatasetParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerAutapses(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getDistribution(c echo.Context) error {
	var reqObject DistributionParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerDistribution(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

func (exp *explorerQuery) getCompleteness(c echo.Context) error {
	var reqObject CompletenessParams
	c.Bind(&reqObject)
	if data, err := exp.engine.ExplorerCompleteness(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
