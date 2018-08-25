package api

import (
	"github.com/janelia-flyem/neuPrintHTTP/storage"
	"github.com/labstack/echo"
	"net/http"
)

const APIVERSION = "1.0"
const PREFIX = "/api"

type setupAPI func(*ConnectomeAPI) error

var (
	availAPIs map[string]setupAPI
)

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

func (c *ConnectomeAPI) SetRoute(connType ConnectionType, prefix string, route echo.HandlerFunc) {
	switch connType {
	case GET:
		c.e.GET(prefix, route)
		c.e.GET("/v:ver"+prefix, route)
	case POST:
		c.e.POST(prefix, route)
		c.e.POST("/v:ver"+prefix, route)
	case PUT:
		c.e.PUT(prefix, route)
		c.e.PUT("/v:ver"+prefix, route)
	case DELETE:
		c.e.DELETE(prefix, route)
		c.e.DELETE("/v:ver"+prefix, route)
	}
}

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

	// TODO serve out swagger
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
