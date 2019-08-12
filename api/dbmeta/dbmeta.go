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

type storeAPI struct {
	Store storage.Store
}

// setupAPI loads all the endpoints for dbmeta
func setupAPI(mainapi *api.ConnectomeAPI) error {
	if simpleEngine, ok := mainapi.Store.(storage.Store); ok {
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

		// instances endpoint
		endpoint = "instances"
		mainapi.SetRoute(api.GET, PREFIX+"/"+endpoint, q.getDataInstances)
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

// getVersion returns the version of the database
func (sa storeAPI) getVersion(c echo.Context) error {
	// swagger:operation GET /api/dbmeta/version dbmeta getVersion
	//
	// Gets version of the database
	//
	// Returns the version of the underlying neuprint data model.
	// Changes to the minor version not invalidate previous cypher
	// queries.
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         Version:
	//           type: "string"
	// security:
	// - Bearer: []

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

// getDatabase returns information on the main graph database
func (sa storeAPI) getDatabase(c echo.Context) error {
	// swagger:operation GET /api/dbmeta/database dbmeta getDatabase
	//
	// Database information
	//
	// Returns JSON information about the database.
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         Location:
	//           type: "string"
	//           description: "Server location"
	//         Description:
	//           type: "string"
	//           description: "Information about the backend"
	// security:
	// - Bearer: []

	if loc, desc, err := sa.Store.GetDatabase(); err != nil {
		return err
	} else {
		data := &dbDatabase{loc, desc}
		return c.JSON(http.StatusOK, data)
	}
}

// getDatasets returns datasets supported by the database
func (sa storeAPI) getDatasets(c echo.Context) error {
	// swagger:operation GET /api/dbmeta/datasets dbmeta getDatasets
	//
	// Gets datasets in the graph database
	//
	// Metadata associated with each dataset is also retrieved
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       properties:
	//         "last-mod":
	//           type: "string"
	//           description: "Last modification date for dataset"
	//         uuid:
	//           type: "string"
	//           description: "last version id for dataset (UUID for DVID)"
	//         ROIs:
	//           type: "array"
	//           items:
	//             type: "string"
	//           example: ["alpha1", "alpha2", "alpha3"]
	//           description: "regions of interest available for the dataset"
	// security:
	// - Bearer: []

	if data, err := sa.Store.GetDatasets(); err != nil {
		return err
	} else {
		return c.JSON(http.StatusOK, data)
	}
}

type instanceInfo struct {
	Instance string   `json:"instance"`
	Datasets []string `json:"datasets"`
}

// getDataInstances returns the secondary data instances available
func (sa storeAPI) getDataInstances(c echo.Context) error {
	// swagger:operation GET /api/dbmeta/instances dbmeta getDataInstances
	//
	// Gets secondary data instances avaiable through neupint http
	//
	// Contains datatype and instance info for data not within the neuprint
	// data model.
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	//     schema:
	//       type: "object"
	//       additionalProperties:
	//         type: "array"
	//         description: "instance type name"
	//         items:
	//           type: "object"
	//           properties:
	//             instance:
	//               type: "string"
	//               description: "name of data instance"
	//             datasets:
	//               type: "array"
	//               items:
	//                 type: "string"
	//                 description: "dataset supported by instance"
	// security:
	// - Bearer: []
	allstores := sa.Store.GetStores()

	var res map[string][]instanceInfo
	for _, store := range allstores {
		tname := store.GetType()
		iname := store.GetInstance()
		datasets, err := store.GetDatasets()
		if err != nil {
			return err
		}

		if _, ok := res[tname]; !ok {
			res[tname] = make([]instanceInfo, 0)
		}
		dlist := make([]string, 0)
		for dataset, _ := range datasets {
			dlist = append(dlist, dataset)
		}
		res[tname] = append(res[tname], instanceInfo{Instance: iname, Datasets: dlist})
	}

	return c.JSON(http.StatusOK, res)
}
