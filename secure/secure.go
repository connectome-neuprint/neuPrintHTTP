package secure

import (
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/acme/autocert"
)

// EchoSecure handles secure connection configuration and startup.
type EchoSecure struct {
	e       *echo.Echo
	manCert bool // true when PEM cert/key are provided
	sslCert string
	sslKey  string
}

// InitializeEchoSecure configures HTTPS (manual cert or autocert) and
// registers the DSG auth routes (login, logout, profile, token).
// It no longer sets up sessions, OAuth, or JWT — DSG handles all of that.
func InitializeEchoSecure(e *echo.Echo, sslCert, sslKey, hostname, dsgURL string, dsgClient *DSGClient) (*EchoSecure, error) {
	manCert := sslCert != "" && sslKey != ""

	if !manCert {
		e.AutoTLSManager.Cache = autocert.DirCache("./cache")
	}

	e.Pre(middleware.HTTPSRedirect())
	e.Pre(middleware.HTTPSNonWWWRedirect())

	// Register auth-related routes (no middleware — these must be accessible
	// before authentication).
	e.GET("/login", dsgLoginHandler(dsgURL))
	e.POST("/logout", DSGAuthMiddleware(dsgClient)(dsgLogoutHandler(dsgURL)))
	e.GET("/profile", DSGAuthMiddleware(dsgClient)(dsgProfileHandler))
	e.GET("/token", DSGAuthMiddleware(dsgClient)(dsgTokenHandler(dsgURL)))

	return &EchoSecure{
		e:       e,
		manCert: manCert,
		sslCert: sslCert,
		sslKey:  sslKey,
	}, nil
}

// StartEchoSecure starts the HTTPS server.
func (s *EchoSecure) StartEchoSecure(port int) {
	portstr := strconv.Itoa(port)
	if s.manCert {
		s.e.Logger.Fatal(s.e.StartTLS(":"+portstr, s.sslCert, s.sslKey))
	} else {
		s.e.Logger.Fatal(s.e.StartAutoTLS(":" + portstr))
	}
}
