package keyvalue

import (
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"io/ioutil"
	"net/http"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/raw/keyvalue"

type masterAPI struct {
	Store storage.Store
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &masterAPI{mainapi.Store}

	// key endpoint
	endPoint := "key"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint+"/:instance/:key", q.getKV)
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint+"/:instance/:key", q.setKV)
	return nil
}

// getKV fetches data specified by a given database instance and key
func (ma masterAPI) getKV(c echo.Context) error {
	// swagger:operation GET /api/raw/keyvalue/key/{instance}/{key} raw-keyvalue getKV
	//
	// Get data stored at the key.
	//
	// The data address is given by both the instance name and key.
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "instance"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "database instance name"
	// - in: "path"
	//   name: "key"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "location of the data"
	// responses:
	//   200:
	//     description: "blob data"
	// security:
	// - Bearer: []

	instance := c.Param("instance")
	keyname := c.Param("key")

	if instance == "" || keyname == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	store, ok := ma.Store.GetInstances()[instance]
	if !ok {
		errJSON := api.ErrorInfo{Error: "provided instance not found"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	kvstore, ok := store.(storage.KeyValue)
	if !ok {
		errJSON := api.ErrorInfo{Error: "database doesn't support keyvalue"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// fetch the value
	res, err := kvstore.Get([]byte(keyname))
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	return c.Blob(http.StatusOK, "application/octet-stream", res)
}

// setKV posts data to a given database instance and key
func (ma masterAPI) setKV(c echo.Context) error {
	// swagger:operation POST /api/raw/keyvalue/key/{instance}/{key} raw-keyvalue postKV
	//
	// Post data stored at the key.
	//
	// The data address is given by both the instance name and key.
	//
	// ---
	// parameters:
	// - in: "path"
	//   name: "instance"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "database instance name"
	// - in: "path"
	//   name: "key"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "location of the data"
	// - in: "body"
	//   name: "blob"
	//   description: "binary blob"
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: []

	instance := c.Param("instance")
	keyname := c.Param("key")

	if instance == "" || keyname == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	store, ok := ma.Store.GetInstances()[instance]
	if !ok {
		errJSON := api.ErrorInfo{Error: "provided instance not found"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	kvstore, ok := store.(storage.KeyValue)
	if !ok {
		errJSON := api.ErrorInfo{Error: "database doesn't support keyvalue"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// post the value
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		errJSON := api.ErrorInfo{Error: "error reading binary data"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	err = kvstore.Set([]byte(keyname), body)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	return c.String(http.StatusOK, "")
}
