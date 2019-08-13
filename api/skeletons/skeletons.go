package skeletons

import (
	"bytes"
	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

func init() {
	api.RegisterAPI(PREFIX, setupAPI)
}

const PREFIX = "/skeletons"

type masterAPI struct {
	Store storage.Store
}

// setupAPI sets up the optionally supported custom endpoints
func setupAPI(mainapi *api.ConnectomeAPI) error {
	q := &masterAPI{mainapi.Store}

	// skeleton endpoint
	endPoint := "skeleton"
	mainapi.SupportedEndpoints[endPoint] = true

	mainapi.SetRoute(api.GET, PREFIX+"/"+endPoint+"/:dataset/:id", q.getSkeleton)
	mainapi.SetAdminRoute(api.POST, PREFIX+"/"+endPoint+"/:dataset/:id", q.setSkeleton)
	return nil
}

type SkeletonResp struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

// getSkeleton fetches the skeleton at the given body id
func (ma masterAPI) getSkeleton(c echo.Context) error {
	// swagger:operation GET /api/skeletons/skeleton/{dataset}/{id} skeletons getSkeleton
	//
	// Get skeleton for given body id
	//
	// The skeletons are stored as swc but the default response is a table
	// of skeleton nodes.
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
	//   name: "id"
	//   schema:
	//     type: "integer"
	//   required: true
	//   description: "body id"
	// - in: "query"
	//   name: "format"
	//   description: "specify response format (\"swc\" or nothing)"
	// responses:
	//   200:
	//     description: "binary swc file if \"format=swc\" specified or JSON"
	//     schema:
	//       type: "object"
	//       properties:
	//         columns:
	//           type: "array"
	//           items:
	//             type: "string"
	//           description: "Name of each result column"
	//         data:
	//           type: "array"
	//           items:
	//             type: "array"
	//             items:
	//               type: "null"
	//               description: "Cell value"
	//             description: "Table row"
	//           description: "Table of skeleton nodes"
	// security:
	// - Bearer: []

	dataset := c.Param("dataset")
	bodyid := c.Param("id")

	if dataset == "" || bodyid == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	if _, err := strconv.Atoi(bodyid); err != nil {
		errJSON := api.ErrorInfo{Error: "body id should be an integer"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// get key value store
	store, err := ma.Store.FindStore("skeletons", dataset)
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
	keystr := bodyid + "_swc"
	res, err := kvstore.Get([]byte(keystr))
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// if swc fommat specified, return binary
	if c.QueryParam("format") == "swc" {
		return c.Blob(http.StatusOK, "text/plain", res)

	}

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
			errJSON := api.ErrorInfo{Error: "SWC not formatted properly"}
			return c.JSON(http.StatusBadRequest, errJSON)
		}

		rownum, _ := strconv.Atoi(entries[0])
		xloc, _ := strconv.ParseFloat(entries[2], 64)
		yloc, _ := strconv.ParseFloat(entries[3], 64)
		zloc, _ := strconv.ParseFloat(entries[4], 64)
		radius, _ := strconv.ParseFloat(entries[5], 64)
		link, _ := strconv.Atoi(entries[6])

		data = append(data, []interface{}{rownum, xloc, yloc, zloc, radius, link})
	}
	jsonresp := SkeletonResp{columns, data}
	return c.JSON(http.StatusOK, jsonresp)
}

// setSkeleton posts the skeleton at the given body id
func (ma masterAPI) setSkeleton(c echo.Context) error {
	// swagger:operation POST /api/skeletons/skeleton/{dataset}/{id} skeletons setSkeleton
	//
	// Post skeleton for the given body id
	//
	// The skeletons are stored as swc.
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
	//   name: "id"
	//   schema:
	//     type: "integer"
	//   required: true
	//   description: "body id"
	// - in: "body"
	//   name: "swc"
	//   description: "skeleton in SWC format"
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: []

	dataset := c.Param("dataset")
	bodyid := c.Param("id")

	if dataset == "" || bodyid == "" {
		errJSON := api.ErrorInfo{Error: "parameters not properly provided in uri"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	if _, err := strconv.Atoi(bodyid); err != nil {
		errJSON := api.ErrorInfo{Error: "body id should be an integer"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// get key value store
	store, err := ma.Store.FindStore("skeletons", dataset)
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
	keystr := bodyid + "_swc"
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		errJSON := api.ErrorInfo{Error: "error reading binary data"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	err = kvstore.Set([]byte(keystr), body)
	if err != nil {
		errJSON := api.ErrorInfo{Error: err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	return c.String(http.StatusOK, "")
}
