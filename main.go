// create server
// parse json to set storage plugin, point to authorized list, read forbidden into memory, secrete key?, metadata for datasets??
// find out what interfaces are disabled
// check db version number (that API should be valid)

package main

import (
	"flag"
	"fmt"
	"github.com/janelia-flyem/echo-secure"
	"github.com/janelia-flyem/neuPrintHTTP/api"
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

	config, err := loadConfig(flag.Args()[0])
	if err != nil {
		fmt.Print(err)
		return
	}

	// create echo web framework
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Pre(middleware.NonWWWRedirect())

	var authorizer secure.Authorizer
	// call new secure API and set authorization method
	fmt.Println(config.AuthDatastore)
	if config.AuthDatastore != "" {
		authorizer, err = secure.NewDatastoreAuthorizer(config.AuthDatastore, config.AuthToken)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		authorizer, err = secure.NewFileAuthorizer(config.AuthFile)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	sconfig := secure.SecureConfig{
		SSLCert:          config.CertPEM,
		SSLKey:           config.KeyPEM,
		ClientID:         config.ClientID,
		ClientSecret:     config.ClientSecret,
		AuthorizeChecker: authorizer,
		Hostname:         config.Hostname,
	}
	secureAPI, err := secure.InitializeEchoSecure(e, sconfig, []byte(config.Secret))
	if err != nil {
		fmt.Println(err)
		return
	}

	// create read only group
	readGrp := e.Group("/api")
	readGrp.Use(secureAPI.AuthMiddleware(secure.READ))

	// setup default page
	// TODO: point to swagger documentation
	if config.StaticDir != "" {
		e.Static("/", config.StaticDir)
	} else {
		e.GET("/", secureAPI.AuthMiddleware(secure.READ)(func(c echo.Context) error {
			return c.HTML(http.StatusOK, "<html><title>neuprint http</title><body><a href='/token'><button>Download API Token</button></a><form action='/logout' method='post'><input type='submit' value='Logout' /></form></body></html>")
		}))
	}

	// load connectomic READ-ONLY API
	if err = api.SetupRoutes(e, readGrp, config.Store); err != nil {
		fmt.Print(err)
		return
	}

	// start server
	secureAPI.StartEchoSecure(port)
}
