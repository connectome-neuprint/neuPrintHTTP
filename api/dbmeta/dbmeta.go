package dbmeta

import (
	"fmt"
	"github.com/janelia-flyem/neuPrintHTTP/api"
	"github.com/labstack/echo"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

// list of available endpoints
var ENDPOINTS = [...]string{"datasets", "database", "version"}

const PREFIX = "/dbmeta"

func setupAPI(c *api.ConnectomeAPI) error {
	if _, ok := c.Store.(StorageAPI); ok {
		q := &metaQuery{c.Store}
		for _, endPoint := range ENDPOINTS {
			c.SupportedEndpoints[endPoint] = true
			switch endPoint {
			case "version":
				c.SetRoute(api.GET, PREFIX+"/version", q.getVersion)
			case "database":
				c.SetRoute(api.GET, PREFIX+"/database", q.getDatabase)
			case "datasets":
				c.SetRoute(api.GET, PREFIX+"/datasets", q.getDatasets)
			}
		}
	} else {
		// meta interface is required by default
		return fmt.Errorf("metadata interface is not available")
	}

	return nil
}

type metaQuery struct {
	engine StorageAPI
}

// TODO: swagger document
type dbVersion struct {
	Version string
}

func (m *metaQuery) getVersion(c echo.Context) error {
	if data, err := m.engine.GetVersion(); err != nil {
		return err
	} else {
		data := &dbVersion{data}
		return c.JSON(http.StatusOK, data)
	}
}

type dbDatabase struct {
	Location    string
	Description string
}

func (m *metaQuery) getDatabase(c echo.Context) error {
	if loc, desc, err := m.engine.GetDatabase(); err != nil {
		return err
	} else {
		data := &dbDatabase{loc, desc}
		return c.JSON(http.StatusOK, data)
	}
}

func (m *metaQuery) getDatasets(c echo.Context) error {
	if data, err := m.engine.GetDatasets(); err != nil {
		return err
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
