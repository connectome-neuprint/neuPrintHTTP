package dbmeta

import (
	"fmt"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/dbmeta"

// ?! add get datasets endpoint
// ?! make swagger docs

type storeAPI struct {
	Store storage.SimpleStore
}

// setupAPI loads all the endpoints for dbmeta
func setupAPI(mainapi *api.ConnectomeAPI) error {
	if simpleEngine, ok := mainapi.Store.(storage.SimpleStore); ok {
		q := &storeAPI{simpleEngine}

		// version endpoint
		endpoint := "version"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getVersion)
		mainapi.SupportedEndpoints[endpoint] = true

		// database endpoint
		endpoint = "database"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getDatabase)
		mainapi.SupportedEndpoints[endpoint] = true

		// datasets endpoint
		endpoint = "datasets"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getDatasets)
		mainapi.SupportedEndpoints[endpoint] = true
	} else {
		// meta interface is required by default
		return fmt.Errorf("metadata interface is not available")
	}

	return nil
}

type dbVersion struct {
	Version string
}

func (sa storeAPI) getVersion(c echo.Context) error {
	if data, err := sa.Store.GetVersion(); err != nil {
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

func (sa storeAPI) getDatabase(c echo.Context) error {
	if loc, desc, err := sa.Store.GetDatabase(); err != nil {
		return err
	} else {
		data := &dbDatabase{loc, desc}
		return c.JSON(http.StatusOK, data)
	}
}

func (sa storeAPI) getDatasets(c echo.Context) error {
	if data, err := sa.Store.GetDatasets(); err != nil {
		return err
	} else {
		return c.JSON(http.StatusOK, data)
	}
}
