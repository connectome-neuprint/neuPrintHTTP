package secure

import (
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
        "golang.org/x/crypto/acme/autocert"
        "github.com/labstack/echo-contrib/session"
        "github.com/gorilla/sessions"
        "github.com/dgrijalva/jwt-go"
        "fmt"
        "reflect"
        "net/http"
        "strconv"
)

// SecureConfig provides configuration options when initializing echo
type SecureConfig struct {
    SSLCert string // filename for SSL certificate (should be .PEM file) (default auto TLS)
    SSLKey string // filename for SSL key file (should be .PEM file) (default auto TLS)
    ClientID string // Google client ID for oauth (required for authentication)
    ClientSecret string // Google client secret for outh (required for authentication)
    AuthorizeChecker Authorizer // Checks whether user is authorized (default: authorize all)
    Hostname string // Hostname is the location of the server that will be used for Google oauth callback
}


// EchoSecure handles secure connection configuration
type EchoSecure struct {
    e *echo.Echo // context for secure object
    secret []byte // private key used for cookies and JWT
    enableAuthenticate bool 
    enableAuthorize bool 
    manCert bool // if true, requires https PEM files
    config SecureConfig
}

// AuthMiddleware checks authentication and authorization for the designated handlers
func (s EchoSecure) AuthMiddleware(authLevel AuthorizationLevel) echo.MiddlewareFunc {
        return func (next echo.HandlerFunc) echo.HandlerFunc {
            return func(c echo.Context) error {
                // if no authentication, no need to check
                if !s.enableAuthenticate {
                    return next(c)
                }

                email := ""
                // check for either Bearer token or cookie
                auth := c.Request().Header.Get(echo.HeaderAuthorization)
                l := len("Bearer")
                if len(auth) > l+1 && auth[:l] == "Bearer" {
                    auth = auth[l+1:]
                    claimsPtr := &jwtCustomClaims{}
                    t := reflect.ValueOf(claimsPtr).Type().Elem()
                    claims := reflect.New(t).Interface().(jwt.Claims)
                    token, err := jwt.ParseWithClaims(auth, claims, func(t *jwt.Token) (interface{}, error) {
                                                    // Check the signing method
                                                    if t.Method.Alg() != AlgorithmHS256 {
                                                        return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
                                                    }
                                                    return s.secret, nil})
                    if err == nil && token.Valid {
                        // Store user information from token into context.
                        c.Set("user", token)
                        claims := token.Claims.(*jwtCustomClaims)
                        email = claims.Email
                    } else {
                        return &echo.HTTPError{
                            Code:     http.StatusUnauthorized,
                            Message:  "invalid or expired jwt",
                            Internal: err,
                        }
                    }
                } else {
                    currSession, err  := session.Get(defaultSessionID, c)
                    redirectUrl := "/login?redirect=" + c.Request().URL.Path
                    if err != nil {
                        return c.Redirect(http.StatusFound, redirectUrl)
                    }
                    if profile, ok := currSession.Values[googleProfileSessionKey].(*Profile); !ok || profile == nil { 
                        return c.Redirect(http.StatusFound, redirectUrl)
                    } else {
                        email = profile.Email
                    }
                }

                // check authorize if it exists
                if (s.enableAuthorize) {
                    if isAuthorized := s.config.AuthorizeChecker.Authorize(email, authLevel); !isAuthorized {
                        return &echo.HTTPError{
                            Code:     http.StatusUnauthorized,
                            Message:  "unauthorized user",
                            Internal: fmt.Errorf("user not authorized"),
                        }
                    }
                }

                return next(c)
            }
        }
}


// InitializeEchoSecure sets up https configurations for echo.  If
// a Google authentication key is provided, authentication
// API is created.  Authentication routes are addded in the default
// echo context group.  Note: do not add auth middleware to the default context
// since it will disable login.
func InitializeEchoSecure(e *echo.Echo, config SecureConfig, secret []byte) (EchoSecure, error) {
    // setup logging and panic recover
    manCert := false
    if config.SSLCert != "" && config.SSLKey != "" {
        manCert = true
    }

    if !manCert {
        e.AutoTLSManager.Cache = autocert.DirCache("./cache")
    }
        
    e.Pre(middleware.HTTPSRedirect())
    e.Pre(middleware.HTTPSNonWWWRedirect())
   
    enableAuthenticate := false
    enableAuthorize := false
    if string(secret) != "" {
        enableAuthenticate = true
        enableAuthorize = true
        // add if enabled, auth guard some of the handlers
        e.Use(session.Middleware(sessions.NewCookieStore(secret)))
    }
    if config.AuthorizeChecker == nil {
        enableAuthorize = false
    }

    s := EchoSecure{e, secret, enableAuthenticate, enableAuthorize,  manCert, config}

    if (enableAuthenticate) {
        JWTSecret = secret

        e.GET("/login", loginHandler)
        e.GET("/oauth2callback", oauthCallbackHandler)

        // requires login
        e.POST("/logout", s.AuthMiddleware(READ)(logoutHandler))
        e.GET("/logout", s.AuthMiddleware(READ)(logoutHandler))
        e.GET("/profile", s.AuthMiddleware(READ)(profileHandler))
        e.GET("/token", s.AuthMiddleware(READ)(tokenHandler))
    }

    // return object
    return s, nil
}

func (s EchoSecure) StartEchoSecure(port int) {
    portstr := strconv.Itoa(port)
    if (s.enableAuthenticate) {
        // setup oauth object
        redirectURL := "https://" + s.config.Hostname + ":" + portstr + "/oauth2callback"
        configureOAuthClient(s.config.ClientID, s.config.ClientSecret, redirectURL)
    }

    if s.manCert {
        s.e.Logger.Fatal(s.e.StartTLS(":"+portstr, s.config.SSLCert, s.config.SSLKey))
    } else {
        s.e.Logger.Fatal(s.e.StartAutoTLS(":"+portstr))
    }
}

