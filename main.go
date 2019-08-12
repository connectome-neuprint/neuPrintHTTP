// neuprint API
//
// REST interface for neuPrint.  To test out the interface, copy  your token
// under your acocunt information. Then authorize Swagger by typing "Bearer " and
// pasting the token.
//
//     Version: 0.1.0
//     Contact: Stephen Plaza<plazas@janelia.hhmi.org>
//     Security:
//     - Bearer
//
//     SecurityDefinitions:
//     Bearer:
//         type: apiKey
//         name: Authorization
//         in: header
//
// swagger:meta
//go:generate swagger generate spec -o ./swagger.yaml
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/connectome-neuprint/neuPrintHTTP/api"
	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"github.com/connectome-neuprint/neuPrintHTTP/logging"
	secure "github.com/janelia-flyem/echo-secure"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

func customUsage() {
	fmt.Printf("Usage: %s [OPTIONS] CONFIG.json\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {

	// create command line argument for port
	var port = 11000
	var publicRead = false
	flag.Usage = customUsage
	flag.IntVar(&port, "port", 11000, "port to start server")
	flag.BoolVar(&publicRead, "public_read", false, "allow all users read access")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		return
	}

	// parse options
	options, err := config.LoadConfig(flag.Args()[0])
	if err != nil {
		fmt.Print(err)
		return
	}

	// create datastore based on configuration
	store, err := config.CreateStore(options)
	if err != nil {
		fmt.Println(err)
		return
	}

	// create echo web framework
	e := echo.New()

	// setup logger
	logger, err := logging.GetLogger(port, options)

	e.Use(logging.LoggerWithConfig(logging.LoggerConfig{
		Format: "{\"uri\": \"${uri}\", \"status\": ${status}, \"bytes_in\": ${bytes_in}, \"bytes_out\": ${bytes_out}, \"duration\": ${latency}, \"time\": ${time_unix}, \"user\": \"${custom:email}\", \"category\": \"${category}\", \"debug\": \"${custom:debug}\"}\n",
		Output: logger,
	}))

	e.Use(middleware.Recover())
	e.Pre(middleware.NonWWWRedirect())

	var authorizer secure.Authorizer
	// call new secure API and set authorization method
	if options.AuthDatastore != "" {
		authorizer, err = secure.NewDatastoreAuthorizer(options.AuthDatastore, options.AuthToken)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		authorizer, err = secure.NewFileAuthorizer(options.AuthFile)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	sconfig := secure.SecureConfig{
		SSLCert:          options.CertPEM,
		SSLKey:           options.KeyPEM,
		ClientID:         options.ClientID,
		ClientSecret:     options.ClientSecret,
		AuthorizeChecker: authorizer,
		Hostname:         options.Hostname,
	}
	secureAPI, err := secure.InitializeEchoSecure(e, sconfig, []byte(options.Secret))
	if err != nil {
		fmt.Println(err)
		return
	}

	// create read only group
	readGrp := e.Group("/api")
	if publicRead {
		readGrp.Use(secureAPI.AuthMiddleware(secure.NOAUTH))
	} else {
		readGrp.Use(secureAPI.AuthMiddleware(secure.READ))
	}
	// setup server status message to show if it is public
	e.GET("/api/serverinfo", secureAPI.AuthMiddleware(secure.NOAUTH)(func(c echo.Context) error {
		info := struct {
			IsPublic bool
		}{publicRead}
		return c.JSON(http.StatusOK, info)
	}))

	// setup default page
	if options.StaticDir != "" {
		e.Static("/", options.StaticDir)
		customHTTPErrorHandler := func(err error, c echo.Context) {
			if he, ok := err.(*echo.HTTPError); ok {
				req := c.Request()
				if !strings.HasPrefix(req.RequestURI, "/api") && (he.Code == http.StatusNotFound) {
					c.File(options.StaticDir)
				}
			}
			e.DefaultHTTPErrorHandler(err, c)
		}

		e.HTTPErrorHandler = customHTTPErrorHandler

	} else {
		e.GET("/", secureAPI.AuthMiddleware(secure.NOAUTH)(func(c echo.Context) error {
			return c.HTML(http.StatusOK, "<html><title>neuprint http</title><body><a href='/token'><button>Download API Token</button></a><p><b>Example query using neo4j cypher:</b><br>curl -X GET -H \"Content-Type: application/json\" -H \"Authorization: Bearer YOURTOKEN\" https://SERVERADDR/api/custom/custom -d '{\"cypher\": \"MATCH (m :Meta) RETURN m.dataset AS dataset, m.lastDatabaseEdit AS lastmod\"}'</p><a href='/api/help'>Documentation</a><form action='/logout' method='post'><input type='submit' value='Logout' /></form></body></html>")
		}))
	}

	if options.SwaggerDir != "" {
		e.Static("/api/help", options.SwaggerDir)
	}

	// load connectomic READ-ONLY API
	if err = api.SetupRoutes(e, readGrp, store); err != nil {
		fmt.Print(err)
		return
	}

	// start server
	secureAPI.StartEchoSecure(port)
}
