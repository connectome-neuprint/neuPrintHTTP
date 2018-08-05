// create server
// parse json to set storage plugin, point to authorized list, read forbidden into memory, secrete key?, metadata for datasets??
// find out what interfaces are disabled
// check db version number (that API should be valid)

package main

import (
	"flag"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"strconv"
)

func main() {
	// create command line argument for port
	var port int = 11000
	flag.IntVar(&port, "port", 11000, "port to start server")
	flag.Parse()

	// create echo web framework
	e := echo.New()

	// setup logging and panic recover
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// start server
	portstr := strconv.Itoa(port)
	e.Logger.Fatal(e.Start(":" + portstr))
}
