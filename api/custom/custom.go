package custom

import (
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/utils"
	"github.com/labstack/echo"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

// list of available endpoints
var ENDPOINTS = [...]string{"custom"}

const PREFIX = "/custom"

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(c *api.ConnectomeAPI) error {
	if customInt, ok := c.Store.(StorageAPI); ok {
		q := &customQuery{customInt}
		for _, endPoint := range ENDPOINTS {
			c.SupportedEndpoints[endPoint] = true
			switch endPoint {
			case "custom":
				c.SetRoute(api.GET, PREFIX+"/custom", q.getCustom)
				c.SetRoute(api.POST, PREFIX+"/custom", q.getCustom)
			}
		}
	}
	return nil
}

type customQuery struct {
	engine StorageAPI
}

func (cq *customQuery) getCustom(c echo.Context) error {
	var reqObject map[string]interface{}
	c.Bind(&reqObject)
	if cypher, ok := reqObject["cypher"].(string); ok {
		c.Set("debug", cypher)
	}
	if data, err := cq.engine.CustomRequest(reqObject); err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

// CustomRequest implements API that allows users to specify exact query
func (store Store) CustomRequest(req map[string]interface{}) (res interface{}, err error) {
	// check version if provided
	version, ok := req["version"].(string)
	if ok {
		if !utils.CheckSubsetVersion(version, store.version.String()) {
			err = fmt.Errorf("neo4j data model version incompatible")
			return
		}
	}

	cypher, ok := req["cypher"].(string)
	if !ok {
		err = fmt.Errorf("cypher keyword not found in request JSON")
		return
	}
	return store.makeRequest(cypher)
}
