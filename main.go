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
	"net/http"
        "golang.org/x/crypto/acme/autocert"
        "github.com/gorilla/sessions"
        "github.com/labstack/echo-contrib/session"
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
        e.Use(session.Middleware(sessions.NewCookieStore([]byte(config.CookieSecret))))

        // setup auth
        e.GET("/login", loginHandler)
	e.POST("/logout", logoutHandler)
	//e.GET("/logout", logoutHandler) // ?! temporary for easy testing
	e.GET("/oauth2callback", oauthCallbackHandler)
	e.GET("/profile", authMiddleWare(profileHandler))
        
        // ?! add middle ware auth (ignore auth functions, eventually add jwt check)
       
        // TODO add a default
        e.GET("/", func (c echo.Context) error { return c.HTML(http.StatusOK, "hello world") })

        // TODO add api to get token



        // load API
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
