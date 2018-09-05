package custom

import (
	"github.com/janelia-flyem/neuPrintHTTP/api"
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
			}
		}
	}
	return nil
}

type customQuery struct {
	engine StorageAPI
}

type errorInfo struct {
	Error string `json:"error"`
}

// TODO: swagger document
func (cq *customQuery) getCustom(c echo.Context) error {
	var reqObject map[string]interface{}
	c.Bind(&reqObject)
	if data, err := cq.engine.CustomRequest(reqObject); err != nil {
		errJSON := errorInfo{err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
