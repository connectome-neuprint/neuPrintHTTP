// neuprint API
//
// REST interface for neuPrint.  To test out the interface, copy  your token
// under your acocunt information. Then authorize Swagger by typing "Bearer " and
// pasting the token.
//
//	Version: 1.7.10
//	Contact: Neuprint Team<neuprint@janelia.hhmi.org>
//
//	SecurityDefinitions:
//	Bearer:
//	    type: apiKey
//	    name: Authorization
//	    in: header
//	    scopes:
//	      admin: Admin scope
//	      user: User scope
//	Security:
//	- Bearer:
//
// swagger:meta
//
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
	"github.com/connectome-neuprint/neuPrintHTTP/api/custom"
	"github.com/connectome-neuprint/neuPrintHTTP/config"
	"github.com/connectome-neuprint/neuPrintHTTP/internal/version"
	"github.com/connectome-neuprint/neuPrintHTTP/logging"
	"github.com/connectome-neuprint/neuPrintHTTP/secure"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func customUsage() {
	fmt.Printf("Usage: %s [OPTIONS] CONFIG.json\n", os.Args[0])
	flag.PrintDefaults()
}

func neuprintLogo() {
	fmt.Println("                                                                                    ")
	fmt.Println("                                    ooooooooo.             o8o                  .   ")
	fmt.Println("                                    `888   `Y88.           `\"'                .o8   ")
	fmt.Println("  ooo. .oo.    .ooooo.  oooo  oooo   888   .d88' oooo d8b oooo  ooo. .oo.   .o888oo ")
	fmt.Println("  `888P\"Y88b  d88' `88b `888  `888   888ooo88P'  `888\"\"8P `888  `888P\"Y88b    888   ")
	fmt.Println("   888   888  888ooo888  888   888   888          888      888   888   888    888   ")
	fmt.Println("   888   888  888    .o  888   888   888          888      888   888   888    888 . ")
	fmt.Println("  o888o o888o `Y8bod8P'  `V88V\"V8P' o888o        d888b    o888o o888o o888o   \"888\" ")
	fmt.Println("                                                                                    ")
	fmt.Printf("neuPrintHTTP v%s\n", version.Version)

}

func main() {

	// create command line argument for port
	var port = 11000
	var publicRead = false
	var pidfile = ""
	var arrowFlightPort = 11001
	var disableArrow = false
	flag.Usage = customUsage
	flag.IntVar(&port, "port", 11000, "port to start server")
	flag.StringVar(&pidfile, "pid-file", "", "file for pid")
	flag.BoolVar(&publicRead, "public_read", false, "allow all users read access")
	flag.BoolVar(&storage.Verbose, "verbose", false, "verbose mode")
	flag.BoolVar(&storage.VerboseNumeric, "verbose-numeric", false, "enable verbose numeric type conversion debugging")
	flag.BoolVar(&disableArrow, "disable-arrow", false, "disable Arrow format support (enabled by default)")
	flag.IntVar(&arrowFlightPort, "arrow-flight-port", 11001, "port for Arrow Flight gRPC server")
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

	// Set Arrow configuration
	// Arrow is enabled by default unless the disable-arrow flag is set
	options.EnableArrow = !disableArrow

	// Set Arrow Flight port
	if options.ArrowFlightPort == 0 && arrowFlightPort != 0 {
		options.ArrowFlightPort = arrowFlightPort
	} else if options.ArrowFlightPort != 0 {
		arrowFlightPort = options.ArrowFlightPort
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

	// Display Arrow status and start Flight server if enabled
	if options.EnableArrow {
		fmt.Println("✓ Arrow format enabled: HTTP endpoint available at /api/custom/arrow")

		// Create and start Arrow Flight server
		if options.ArrowFlightPort > 0 {
			// Wait a bit for API initialization to complete
			fmt.Printf("Starting Arrow Flight server on port %d\n", options.ArrowFlightPort)

			// Start the Flight server in a separate goroutine
			go func() {
				// Create minimal Flight service
				// Full Flight implementation will be added in a future release
				flightService := &custom.FlightService{
					Port: options.ArrowFlightPort,
				}

				// Start the Flight service
				if err := flightService.Start(); err != nil {
					fmt.Printf("Arrow Flight server error: %v\n", err)
				}
			}()
		}
	} else {
		fmt.Println("✗ Arrow format disabled (use --enable-arrow to enable)")
	}

	// create echo web framework
	e := echo.New()

	// setup logger
	logger, err := logging.GetLogger(port, options)

	e.Use(logging.LoggerWithConfig(logging.LoggerConfig{
		Format: "{\"dataset\": \"${dataset}\", \"uri\": \"${uri}\", \"status\": ${status}, \"bytes_in\": ${bytes_in}, \"bytes_out\": ${bytes_out}, \"duration\": ${latency}, \"time\": ${time_unix}, \"user\": \"${custom:email}\", \"category\": \"${category}\", \"debug\": \"${custom:debug}\"}\n",
		Output: logger,
	}))

	e.Use(middleware.Recover())
	e.Pre(middleware.NonWWWRedirect())

	// --- Auth setup ---
	var dsgClient *secure.DSGClient
	var secureAPI *secure.EchoSecure

	passthrough := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}

	if !options.DisableAuth {
		if options.DSGUrl == "" {
			fmt.Println("ERROR: dsg-url is required when auth is enabled")
			return
		}
		dsgClient = secure.NewDSGClient(options.DSGUrl, options.DSGCacheTTL, options.DatasetMap)

		secureAPI, err = secure.InitializeEchoSecure(e, options.CertPEM, options.KeyPEM, options.Hostname, options.DSGUrl, dsgClient)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	// create read only group
	readGrp := e.Group("/api")

	// Build auth and admin middleware based on settings.
	var authMiddleware echo.MiddlewareFunc
	var adminMiddleware echo.MiddlewareFunc

	if options.DisableAuth {
		authMiddleware = passthrough
		adminMiddleware = passthrough
	} else {
		authMiddleware = secure.DSGAuthMiddleware(dsgClient)
		adminMiddleware = secure.DSGAdminMiddleware()
	}

	if publicRead {
		// Public read: no auth required for /api routes
		readGrp.Use(passthrough)
	} else {
		readGrp.Use(authMiddleware)
	}

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
	e.GET("/api/serverinfo", func(c echo.Context) error {
		info := struct {
			IsPublic bool
			Version  string
		}{publicRead || options.DisableAuth, version.Version}
		return c.JSON(http.StatusOK, info)
	})

	e.GET("/api/vimoserver", func(c echo.Context) error {
		info := struct {
			Url string
		}{options.VimoServer}
		return c.JSON(http.StatusOK, info)
	})

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
		e.GET("/", func(c echo.Context) error {
			authText := ""
			if !options.DisableAuth {
				authText = "-H \"Authorization: Bearer YOURTOKEN\" "
			}
			return c.HTML(http.StatusOK, "<html><title>neuprint http</title><body><a href='/token'><button>Download API Token</button></a><p><b>Example query using neo4j cypher:</b><br>curl -X GET -H \"Content-Type: application/json\" "+authText+"https://SERVERADDR/api/custom/custom -d '{\"cypher\": \"MATCH (m :Meta) RETURN m.dataset AS dataset, m.lastDatabaseEdit AS lastmod\"}'</p><a href='/api/help'>Documentation</a><form action='/logout' method='post'><input type='submit' value='Logout' /></form></body></html>")
		})
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

	// swagger:operation GET /api/npexplorer/nglayers
	//
	// layer settings for neuroglancer view
	//
	// JSON files containing neuroglancer layer settings per dataset
	//
	// ---
	// responses:
	//   200:
	//     description: "successful operation"

	if options.NgDir != "" {
		e.Static("/api/npexplorer/nglayers", options.NgDir)
	}

	// The admin middleware is chained after auth — DSGAdminMiddleware checks
	// the dsg_user set by DSGAuthMiddleware, so admin routes get both.
	combinedAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return adminMiddleware(next)
	}

	// load connectomic default READ-ONLY API
	if err = api.SetupRoutes(e, readGrp, store, combinedAdmin); err != nil {
		fmt.Print(err)
		return
	}

	// print logo
	neuprintLogo()

	// if log file selected print location of logs
	if options.LoggerFile != "" {
		fmt.Printf("logging to file: %s", options.LoggerFile)
	}

	// start server
	if options.DisableAuth {
		if options.CertPEM != "" && options.KeyPEM != "" {
			// Create a minimal secure config just for SSL
			secureAPI, err = secure.InitializeEchoSecure(e, options.CertPEM, options.KeyPEM, options.Hostname, "", nil)
			if err != nil {
				fmt.Println(err)
				return
			}
			secureAPI.StartEchoSecure(port)
		} else {
			// Fall back to HTTP if no SSL certs provided
			portstr := strconv.Itoa(port)
			e.Logger.Fatal(e.Start(":" + portstr))
		}
	} else {
		secureAPI.StartEchoSecure(port)
	}
}
