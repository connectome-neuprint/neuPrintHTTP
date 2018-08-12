// create server
// parse json to set storage plugin, point to authorized list, read forbidden into memory, secrete key?, metadata for datasets??
// find out what interfaces are disabled
// check db version number (that API should be valid)

package main

import (
        "flag"
        "fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"strconv"
        "os"
        "golang.org/x/crypto/acme/autocert"
        "github.com/janelia-flyem/neuPrintHTTP/api"
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
        fmt.Println(config.Store.GetName())
        fmt.Println(config.Store.GetVersion())
        fmt.Println(config.Store.GetDatasets())
        fmt.Println(config.AuthFile)

	// create echo web framework
	e := echo.New()

	// setup logging and panic recover
        
        manCert := false
        if config.CertPEM != "" && config.KeyPEM != "" {
            manCert = true
        }

        if !manCert {
            e.AutoTLSManager.Cache = autocert.DirCache("./cache")
        }
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
        e.Pre(middleware.HTTPSRedirect())
        e.Pre(middleware.HTTPSNonWWWRedirect())
        e.Pre(middleware.NonWWWRedirect())

        if err = api.SetupRoutes(e, config.Store); err != nil {
            fmt.Print(err)
            return
        }

        // start server
	portstr := strconv.Itoa(port)
        if manCert {
            e.Logger.Fatal(e.StartTLS(":"+portstr, config.CertPEM, config.KeyPEM))
        } else {
            e.Logger.Fatal(e.StartAutoTLS(":"+portstr))
        }
}
