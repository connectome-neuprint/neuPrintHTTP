package api

import (
    "net/http"
    "github.com/labstack/echo"
    "github.com/janelia-flyem/neuPrintHTTP/storage"
)

const APIVERSION = "1.0"
const PREFIX = "/api"

type ConnectionType int
const (
    GET ConnectionType = iota
    POST
    PUT
    DELETE
)

type ConnectomeAPI struct {
    store storage.Store
    supportedEndpoints map[string]bool
}

func newConnectomeAPI(store storage.Store) *ConnectomeAPI {
    return &ConnectomeAPI{store, make(map[string]bool)} 
}

func (c *ConnectomeAPI) SetRoute(e echo.Echo, connType ConnectionType, prefix string, route echo.HandlerFunc) {
    switch connType {
        case GET: 
            e.GET("/api" + prefix, route)
            e.GET("/api/v:ver" + prefix, route)
        case POST: 
            e.POST("/api" + prefix, route)
            e.POST("/api/v:ver" + prefix, route)
        case PUT: 
            e.PUT("/api/" + prefix, route)
            e.PUT("/api/v:ver" + prefix, route)
        case DELETE: 
            e.DELETE("/api" + prefix, route)
            e.DELETE("/api/v:ver" + prefix, route)
    }
}


func SetupRoutes(e *echo.Echo, store storage.Store) error {
    apiObj := newConnectomeAPI(store)

    // meta API
    /*if err := apiObj.setUpMeta(e); err != nil {
        return err
    }*/
    // TODO generically add specialized API



    e.GET("/api/version", apiObj.getAPIVersion)
    e.GET("/api/available", func (c echo.Context) error {
        return c.JSON(http.StatusOK, e.Routes())
    })

    // TODO serve out swagger
    return nil
}

// TODO: swagger document
type apiVersion struct  {
    Version string
}
func (api *ConnectomeAPI) getAPIVersion(c echo.Context) error {
    vers := apiVersion{APIVERSION}
    return c.JSON(http.StatusOK, vers)
}



