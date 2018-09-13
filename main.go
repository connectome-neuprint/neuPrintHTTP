package main

import (
	"flag"
	"fmt"
	"github.com/janelia-flyem/echo-secure"
	"github.com/janelia-flyem/neuPrintHTTP/api"
	"github.com/janelia-flyem/neuPrintHTTP/config"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"net/http"
	"os"
)

func customUsage() {
	fmt.Printf("Usage: %s [OPTIONS] CONFIG.json\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	// create command line argument for port
	var port int = 11000
	flag.Usage = customUsage
	flag.IntVar(&port, "port", 11000, "port to start server")
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
	e.Use(middleware.Logger())
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
	readGrp.Use(secureAPI.AuthMiddleware(secure.READ))

	// setup default page
	// TODO: point to swagger documentation
	if options.StaticDir != "" {
		e.Static("/", options.StaticDir)
		customHTTPErrorHandler := func(err error, c echo.Context) {
			if he, ok := err.(*echo.HTTPError); ok {
				if he.Code == http.StatusNotFound {
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
