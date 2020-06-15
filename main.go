// neuprint API
//
// REST interface for neuPrint.  To test out the interface, copy  your token
// under your acocunt information. Then authorize Swagger by typing "Bearer " and
// pasting the token.
//
//     Version: 0.1.0
//     Contact: Stephen Plaza<plazas@janelia.hhmi.org>
//
//     SecurityDefinitions:
//     Bearer:
//         type: apiKey
//         name: Authorization
//         in: header
//         scopes:
//           admin: Admin scope
//           user: User scope
//     Security:
//     - Bearer:
//
// swagger:meta
//go:generate swagger generate spec -o ./swaggerdocs/swagger.yaml
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

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

func neuprintLogo() {
	fmt.Println("                                                                                                                                                               ")
	fmt.Println("                                                                                                                                                               ")
	fmt.Println("                                                          @@@@@@@@@@@%%%*.  @@@@@@@@@@@&&%*.    @@@@@@@@@@@@@@@@@@@       @@@@@@@@@@@@@@@@@@@@@@@@&            ")
	fmt.Println("                                                           &(#@@@@@@@@@@@@%*  @@@@@@@@@@@@@@%*   &@@@@@@   &@@@@@@@#*      &&@@@@& @@@@@@@@@@@@@@@%            ")
	fmt.Println("                                                           &@@@@@@@@@@@@@@@@  @@@@@@@@@@@@@@@@   &@@@@@@   &@@@@@@@@@*     &@@@@@@@@@@@@@@@@@@@@&,.            ")
	fmt.Println("                                                           &@@@@@&   @@@@@@@* @@@@&&&  @@@@@@@   &@@@@@@   &@@@@@@@@@@@*   &@@@@@& &   &@@@@@@    @            ")
	fmt.Println("         .,,,, *#%%#*         .#%@&%#     ,,,,.    .,,,.   &@@@@@&   *&@@@@@@ @@@@@@&  &@@@@@@   &@@@@@@   &@@@@@@@@@@@@*  &@@@@@& @   &@@@@@@                 ")
	fmt.Println("        &@@@@#@  &@@@      *&@@   %@@%  *#@@@@    *@@@@    &@@@@@@&&@@@@@@@@  @@@@@@@@@@@@@@@@   &@@@@@@   &@@@@@@@@@@@@@& &@@@@@&     &@@@@@@                 ")
	fmt.Println("       &@@@@@   *@@@@    *@@@@   @@@@@  @@@@     &@@@@     %@@@@@@@@@@@@@#@@  @@@@@@@@@@@@@@     &@@@@@@   &@@@@@& @@@@@@@@@@@@@@&     &@@@@@@                 ")
	fmt.Println("     *#@@@@    @@@@@    @@@@  *@@@@    @@@@@   *#@@@@    *@@@@@@@@@@@@@@@@    @@@@@@@@@@@@&*     &@@@@@@   &@@@@@&   @@@@@@@@@@@@&     &@@@@@@                 ")
	fmt.Println("    *@@@@    *@@@@    *@@@@@         #@@@@    #@@@@    *@  & %@@@&            @@@@@@@@@@@@@@&*   &@@@@@@   &@@@@@&     @@@@@@@@@@&     &@@@@@@                 ")
	fmt.Println("   &@@@@    #@@@@   *@@&@@&*       *%@@@    #@@@@@   *#@   &@@@@@&            @@@@@@& @@@@@@@@#  &@@@@@@   &@@@@@&      @@@@@@@@@&     &@@@@@@                 ")
	fmt.Println(" *&@@@@     @@@& .*@@  @@@@@*,,,*@@  @@& .%@  @@@  #@      &@@@@@&            @@@@@@&  @@@@@@@@#*&@@@@@@   &@@@@@&       @@@@@@@@&     &@@@@@@                 ")
	fmt.Println("              @@@        @@@@@@      @@@@      @@@@       @@@@@@@@@@@      @@@@@@@@@@ @@@@@@@@@@@@@@@@@@@ @@@@@@@@@       @@@@@@@@@  @@@@@@@@@@@               ")
	fmt.Println("                                                                                                                                                               ")
	fmt.Println("                                                                                                                                                               ")
	fmt.Println("neuPrintHTTP v1.2.1")

}

func main() {

	// create command line argument for port
	var port = 11000
	var proxyport = 0
	var publicRead = false
	var pidfile = ""
	flag.Usage = customUsage
	flag.IntVar(&port, "port", 11000, "port to start server")
	flag.IntVar(&proxyport, "proxy-port", 0, "proxy port to start server")
	flag.StringVar(&pidfile, "pid-file", "", "file for pid")
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

	if pidfile != "" {
		pid := os.Getpid()

		// Open file using READ & WRITE permission.
		fout, err := os.OpenFile(pidfile, os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			fmt.Println(err)
			return
		}

		stopSig := make(chan os.Signal)
		go func() {
			for range stopSig {
				os.Remove(pidfile)
				os.Exit(0)
			}
		}()
		signal.Notify(stopSig, os.Interrupt, os.Kill, syscall.SIGTERM)

		// Write some text line-by-line to file.
		_, err = fout.WriteString(strconv.Itoa(pid))
		if err != nil {
			fmt.Println(err)
			fout.Close()
			return
		}
		fout.Close()
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

	if options.DisableAuth {
		e.GET("/", func(c echo.Context) error {
			return c.HTML(http.StatusOK, "<html><title>neuprint http</title><body><a href='/token'><button>Download API Token</button></a><p><b>Example query using neo4j cypher:</b><br>curl -X GET -H \"Content-Type: application/json\" http://SERVERADDR/api/custom/custom -d '{\"cypher\": \"MATCH (m :Meta) RETURN m.dataset AS dataset, m.lastDatabaseEdit AS lastmod\"}'</p><a href='/api/help'>Documentation</a><form action='/logout' method='post'><input type='submit' value='Logout' /></form></body></html>")
		})

		// swagger:operation GET /api/help/swagger.yaml apimeta helpyaml
		//
		// swagger REST documentation
		//
		// YAML file containing swagger API documentation
		//
		// ---
		// responses:
		//   200:
		//     description: "successful operation"

		if options.SwaggerDir != "" {
			e.Static("/api/help", options.SwaggerDir)
		}
		readGrp := e.Group("/api")

		portstr := strconv.Itoa(port)

		// load connectomic default READ-ONLY API
		if err = api.SetupRoutes(e, readGrp, store, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}); err != nil {
			fmt.Print(err)
			return
		}

		// print logo
		neuprintLogo()

		// start server
		e.Logger.Fatal(e.Start(":" + portstr))

		return
	}

	secure.ProxyPort = proxyport

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
		ProxyAuth:        options.ProxyAuth,
		ProxyInsecure:    options.ProxyInsecure,
	}
	secureAPI, err := secure.InitializeEchoSecure(e, sconfig, []byte(options.Secret), "neuPrintHTTP")
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

	// swagger:operation GET /api/serverinfo apimeta serverinfo
	//
	// Returns whether the server is public
	//
	// If it is public,  no authorization is required
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"
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

	// swagger:operation GET /api/help/swagger.yaml apimeta helpyaml
	//
	// swagger REST documentation
	//
	// YAML file containing swagger API documentation
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"

	if options.SwaggerDir != "" {
		e.Static("/api/help", options.SwaggerDir)
	}

	// load connectomic default READ-ONLY API
	if err = api.SetupRoutes(e, readGrp, store, secureAPI.AuthMiddleware(secure.ADMIN)); err != nil {
		fmt.Print(err)
		return
	}

	// print logo
	neuprintLogo()

	// start server
	secureAPI.StartEchoSecure(port)
}
