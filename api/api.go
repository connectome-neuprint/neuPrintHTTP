package api

import (
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"net/http"
	"strings"
)

const APIVERSION = "0.1"
const PREFIX = "/api"

type setupAPI func(*ConnectomeAPI) error

var (
	availAPIs map[string]setupAPI
)

// RegisterAPI loads api for specified names
func RegisterAPI(name string, f setupAPI) {
	if availAPIs == nil {
		availAPIs = map[string]setupAPI{name: f}
	} else {
		availAPIs[name] = f
	}
}

type ConnectionType int

const (
	GET ConnectionType = iota
	POST
	PUT
	DELETE
)

type ConnectomeAPI struct {
	Store              storage.Store
	SupportedEndpoints map[string]bool
	e                  *echo.Group
}

func newConnectomeAPI(store storage.Store, e *echo.Group) *ConnectomeAPI {
	return &ConnectomeAPI{store, make(map[string]bool), e}
}

func CheckVersion(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		vals := c.ParamValues()
		if len(vals) > 0 {
			versionsplit := strings.Split(vals[0], ".")
			curr_versionsplit := strings.Split(APIVERSION, ".")

			for idx, part := range versionsplit {
				if part != "" {
					if idx >= len(curr_versionsplit) || part != curr_versionsplit[idx] {
						return c.HTML(http.StatusBadRequest, "Incompatible API version")
					}
				}
			}
			/*version, _ := strconv.Atoi(versionsplit[0])
			curr_version, _ := strconv.Atoi(curr_versionsplit[0])
			if version != curr_version {
				return c.HTML(http.StatusBadRequest, "Incompatible API version")
			}*/
		}
		return next(c)
	}
}

// SetRoute sets a handler function to a given prefix.  It provides routes
// to a versioned and versionless API.
func (c *ConnectomeAPI) SetRoute(connType ConnectionType, prefix string, route echo.HandlerFunc) {
	switch connType {
	case GET:
		c.e.GET(prefix, route)
		c.e.GET("/v:ver"+prefix, CheckVersion(route))
	case POST:
		c.e.POST(prefix, route)
		c.e.POST("/v:ver"+prefix, CheckVersion(route))
	case PUT:
		c.e.PUT(prefix, route)
		c.e.PUT("/v:ver"+prefix, CheckVersion(route))
	case DELETE:
		c.e.DELETE(prefix, route)
		c.e.DELETE("/v:ver"+prefix, CheckVersion(route))
	}
}

// SetupRoutes intializes all the loaded API.
// TODO: middleware to check version number specified by each endpoint
func SetupRoutes(e *echo.Echo, eg *echo.Group, store storage.Store) error {
	apiObj := newConnectomeAPI(store, eg)

	for _, f := range availAPIs {
		if err := f(apiObj); err != nil {
			return err
		}
	}

	eg.GET("/version", apiObj.getAPIVersion)
	eg.GET("/available", func(c echo.Context) error {
		return c.JSON(http.StatusOK, e.Routes())
	})
	// TODO: endpont to serve out swagger

	return nil
}

// TODO: swagger document
type apiVersion struct {
	Version string
}

func (api *ConnectomeAPI) getAPIVersion(c echo.Context) error {
	vers := apiVersion{APIVERSION}
	return c.JSON(http.StatusOK, vers)
}
