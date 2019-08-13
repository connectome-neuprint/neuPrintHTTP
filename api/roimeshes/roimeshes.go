package roimeshes

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

const PREFIX = "/roimeshes"

type masterAPI struct {
	Store storage.Store
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &masterAPI{mainapi.Store}

	// mesh endpoint
	endPoint := "mesh"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint+"/:dataset/:roi", q.getMesh)
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint+"/:dataset/:roi", q.setMesh)
	return nil
}

// getMesh fetches the mesh for the given ROIs
func (ma masterAPI) getMesh(c echo.Context) error {
	// swagger:operation GET /api/roimeshes/mesh/{dataset}/{roi} roimeshes getMesh
	//
	// Get mesh for given ROI
	//
	// The meshes are stored in OBJ format
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
	//   name: "roi"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "roi name"
	// responses:
	//   200:
	//     description: "binary OBJ file"
	// security:
	// - Bearer: []

	dataset := c.Param("dataset")
	roiname := c.Param("roi")

	if dataset == "" || roiname == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	store, err := ma.Store.FindStore("roimeshes", dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	kvstore, ok := store.(storage.KeyValue)
	if !ok {
		errJSON := api.ErrorInfo{Error: "database doesn't support keyvalue"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// fetch the value
	res, err := kvstore.Get([]byte(roiname))
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	return c.Blob(http.StatusOK, "text/plain", res)
}

// setMesh posts the mesh at the roi
func (ma masterAPI) setMesh(c echo.Context) error {
	// swagger:operation POST /api/roimeshes/mesh/{dataset}/{roi} roimeshes setMesh
	//
	// Post mesh for the given ROI
	//
	// The mesh are stored as OBJ files
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
	//   name: "roi"
	//   schema:
	//     type: "string"
	//   required: true
	//   description: "roi name"
	// - in: "body"
	//   name: "obj"
	//   description: "mesh in OBJ format"
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: []

	dataset := c.Param("dataset")
	roiname := c.Param("roi")

	if dataset == "" || roiname == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	store, err := ma.Store.FindStore("roimeshes", dataset)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
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
	err = kvstore.Set([]byte(roiname), body)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	return c.String(http.StatusOK, "")
}
