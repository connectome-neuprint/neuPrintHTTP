// Package api neuprint API.
//
// REST interface for neuPrint.
//
//	Version: 0.1.0
//	Contact: Neuprint Team<neuprint@janelia.hhmi.org>
//
// swagger:meta
package api

import (
	"net/http"
	"sync"

	"github.com/connectome-neuprint/neuPrintHTTP/internal/version"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/connectome-neuprint/neuPrintHTTP/utils"
	"github.com/labstack/echo/v4"
)

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

// RoutePolicy is the declared authorization contract for an API route.
type RoutePolicy string

const (
	GuardedRoute        RoutePolicy = "guarded"
	AdminRoute          RoutePolicy = "admin"
	NamedExceptionRoute RoutePolicy = "named-exception"
)

type RoutePolicyKey struct {
	Method string
	Path   string
}

var routePolicyRegistry = struct {
	sync.RWMutex
	policies map[RoutePolicyKey]RoutePolicy
}{policies: make(map[RoutePolicyKey]RoutePolicy)}

type ErrorInfo struct {
	Error string `json:"error"`
}

type SuccessInfo struct {
	Msg string `json:"msg"`
}

type ConnectomeAPI struct {
	Store              storage.Store
	SupportedEndpoints map[string]bool
	e                  *echo.Group
	adminMiddleware    echo.MiddlewareFunc
	Package            interface{} // Can hold the package-specific API object
}

// AddSwaggerDefinition adds a swagger definition
func (c *ConnectomeAPI) AddSwaggerDefinition(name string, description string) {
	// This is just a stub for documentation purposes
}

// AddSwaggerTag adds a swagger tag
func (c *ConnectomeAPI) AddSwaggerTag(name string, description string, externalDocs string) {
	// This is just a stub for documentation purposes
}

func newConnectomeAPI(store storage.Store, e *echo.Group, admincheck echo.MiddlewareFunc) *ConnectomeAPI {
	return &ConnectomeAPI{
		Store:              store,
		SupportedEndpoints: make(map[string]bool),
		e:                  e,
		adminMiddleware:    admincheck,
		Package:            nil,
	}
}

func CheckVersion(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		vals := c.ParamValues()
		if len(vals) > 0 {
			if !utils.CheckSubsetVersion(vals[0], version.Version) {
				errJSON := ErrorInfo{"Incompatible API version"}
				return c.JSON(http.StatusBadRequest, errJSON)
			}
		}
		//c.SetParamValues()
		//c.SetParamNames()
		return next(c)
	}
}

// SetRoute sets a handler function and declares its authorization policy for
// both the versioned and versionless API aliases.
func (c *ConnectomeAPI) SetRoute(connType ConnectionType, prefix string, route echo.HandlerFunc, policy RoutePolicy) {
	c.setRoute(connType, prefix, route, policy)
}

func (c *ConnectomeAPI) setRoute(connType ConnectionType, prefix string, route echo.HandlerFunc, policy RoutePolicy) {
	method := methodForConnection(connType)
	DeclareRoutePolicy(method, PREFIX+prefix, policy)
	DeclareRoutePolicy(method, PREFIX+"/v:ver"+prefix, policy)
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

// SetAdminRoute sets a handler function to a given prefix with admin privileges.
func (c *ConnectomeAPI) SetAdminRoute(connType ConnectionType, prefix string, route echo.HandlerFunc) {
	c.setRoute(connType, prefix, c.adminMiddleware(route), AdminRoute)
}

// SetGroupRoute registers a non-versioned /api route with an explicit policy.
func SetGroupRoute(group *echo.Group, connType ConnectionType, path string, route echo.HandlerFunc, policy RoutePolicy) {
	DeclareRoutePolicy(methodForConnection(connType), PREFIX+path, policy)
	switch connType {
	case GET:
		group.GET(path, route)
	case POST:
		group.POST(path, route)
	case PUT:
		group.PUT(path, route)
	case DELETE:
		group.DELETE(path, route)
	}
}

// DeclareRoutePolicy records a route created by a specialized Echo helper,
// such as a static-file route.
func DeclareRoutePolicy(method, path string, policy RoutePolicy) {
	if policy != GuardedRoute && policy != AdminRoute && policy != NamedExceptionRoute {
		panic("invalid API route policy: " + policy)
	}
	key := RoutePolicyKey{Method: method, Path: path}
	routePolicyRegistry.Lock()
	defer routePolicyRegistry.Unlock()
	if existing, ok := routePolicyRegistry.policies[key]; ok && existing != policy {
		panic("conflicting API route policy for " + method + " " + path)
	}
	routePolicyRegistry.policies[key] = policy
}

// DeclaredRoutePolicies returns a copy of the route-to-policy registry.
func DeclaredRoutePolicies() map[RoutePolicyKey]RoutePolicy {
	routePolicyRegistry.RLock()
	defer routePolicyRegistry.RUnlock()
	result := make(map[RoutePolicyKey]RoutePolicy, len(routePolicyRegistry.policies))
	for key, policy := range routePolicyRegistry.policies {
		result[key] = policy
	}
	return result
}

func methodForConnection(connType ConnectionType) string {
	switch connType {
	case GET:
		return http.MethodGet
	case POST:
		return http.MethodPost
	case PUT:
		return http.MethodPut
	case DELETE:
		return http.MethodDelete
	default:
		panic("invalid API connection type")
	}
}

// Map to store initialized API packages
var apiPackages = make(map[string]interface{})

// GetAPIPackage returns the API package with the given name
func GetAPIPackage(name string) (interface{}, error) {
	if pkg, ok := apiPackages[name]; ok {
		return pkg, nil
	}
	return nil, echo.NewHTTPError(http.StatusBadRequest, "API package not found: "+name)
}

// SetupRoutes intializes all the loaded API.
func SetupRoutes(e *echo.Echo, eg *echo.Group, store storage.Store, admincheck echo.MiddlewareFunc) error {
	apiObj := newConnectomeAPI(store, eg, admincheck)

	for name, f := range availAPIs {
		if err := f(apiObj); err != nil {
			return err
		}

		// Store the package in our map if it was set
		if apiObj.Package != nil {
			apiPackages[name] = apiObj.Package
			// Reset for next iteration
			apiObj.Package = nil
		}
	}

	// swagger:operation GET /api/version apimeta getAPIVersion
	//
	// version of the connectomics API
	//
	// version number
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: []
	SetGroupRoute(eg, GET, "/version", apiObj.getAPIVersion, NamedExceptionRoute)

	// swagger:operation GET /api/available apimeta routes
	//
	// list of available REST api routes
	//
	// list of all routes in /api
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
	// security:
	// - Bearer: []
	SetGroupRoute(eg, GET, "/available", func(c echo.Context) error {
		return c.JSON(http.StatusOK, e.Routes())
	}, NamedExceptionRoute)

	return nil
}

type apiVersion struct {
	Version string
}

func (api *ConnectomeAPI) getAPIVersion(c echo.Context) error {
	vers := apiVersion{version.Version}
	return c.JSON(http.StatusOK, vers)
}
