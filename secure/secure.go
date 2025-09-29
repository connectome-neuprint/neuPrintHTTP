package secure

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/crypto/acme/autocert"
)

// Horrible Hack
var ProxyPort = 0

// TokenBlocklist manages a thread-safe set of blocked JWT tokens
type TokenBlocklist struct {
	tokens map[string]bool
	mu     sync.RWMutex
}

// NewTokenBlocklist creates a new token blocklist
func NewTokenBlocklist() *TokenBlocklist {
	return &TokenBlocklist{
		tokens: make(map[string]bool),
	}
}

// AddToken adds a token to the blocklist
func (tb *TokenBlocklist) AddToken(token string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens[token] = true
}

// IsBlocked checks if a token is in the blocklist
func (tb *TokenBlocklist) IsBlocked(token string) bool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.tokens[token]
}

// RemoveToken removes a token from the blocklist
func (tb *TokenBlocklist) RemoveToken(token string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	delete(tb.tokens, token)
}

// LoadTokensFromSlice loads tokens from a slice into the blocklist
func (tb *TokenBlocklist) LoadTokensFromSlice(tokens []string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = make(map[string]bool)
	for _, token := range tokens {
		tb.tokens[token] = true
	}
}

// Count returns the number of tokens in the blocklist
func (tb *TokenBlocklist) Count() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.tokens)
}

// Global token blocklist instance
var globalTokenBlocklist = NewTokenBlocklist()

// GetTokenBlocklist returns the global token blocklist instance
func GetTokenBlocklist() *TokenBlocklist {
	return globalTokenBlocklist
}

// AccessLevel is an alias for AuthorizationLevel for backward compatibility
type AccessLevel = AuthorizationLevel

// SecureAPI is an alias for EchoSecure for backward compatibility
type SecureAPI = EchoSecure

// SecureConfig provides configuration options when initializing echo
type SecureConfig struct {
	SSLCert          string     // filename for SSL certificate (should be .PEM file) (default auto TLS)
	SSLKey           string     // filename for SSL key file (should be .PEM file) (default auto TLS)
	ClientID         string     // Google client ID for oauth (required for authentication)
	ClientSecret     string     // Google client secret for outh (required for authentication)
	AuthorizeChecker Authorizer // Checks whether user is authorized (default: authorize all)
	Hostname         string     // Hostname is the location of the server that will be used for Google oauth callback
	ProxyAuth        string     // ProxyAuth name of proxy server (optional)
	ProxyInsecure    bool       // If true, disable secure connection
	TokenBlocklistFile string   // Path to file containing blocked JWT tokens (optional)
}

// EchoSecure handles secure connection configuration
type EchoSecure struct {
	e                  *echo.Echo // context for secure object
	secret             []byte     // private key used for cookies and JWT
	enableAuthenticate bool
	enableAuthorize    bool
	manCert            bool // if true, requires https PEM files
	config             SecureConfig
}

// AuthMiddleware checks authentication and authorization for the designated handlers
func (s EchoSecure) AuthMiddleware(authLevel AuthorizationLevel) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// if no authentication, no need to check
			if !s.enableAuthenticate {
				return next(c)
			}

			email := ""
			imageurl := ""
			// check for either Bearer token or cookie
			auth := c.Request().Header.Get(echo.HeaderAuthorization)
			l := len("Bearer")
			if len(auth) > l+1 && auth[:l] == "Bearer" {
				auth = auth[l+1:]

				// Check if token is in the blocklist
				if globalTokenBlocklist.IsBlocked(auth) {
					return &echo.HTTPError{
						Code:     http.StatusUnauthorized,
						Message:  "token has been revoked",
						Internal: fmt.Errorf("blocked token used"),
					}
				}

				claimsPtr := &jwtCustomClaims{}
				t := reflect.ValueOf(claimsPtr).Type().Elem()
				claims := reflect.New(t).Interface().(jwt.Claims)
				token, err := jwt.ParseWithClaims(auth, claims, func(t *jwt.Token) (interface{}, error) {
					// Check the signing method
					if t.Method.Alg() != AlgorithmHS256 {
						return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
					}
					return s.secret, nil
				})
				if err == nil && token.Valid {
					// Store user information from token into context.
					c.Set("user", token)
					claims := token.Claims.(*jwtCustomClaims)
					email = claims.Email
					imageurl = claims.ImageURL
				} else {
					return &echo.HTTPError{
						Code:     http.StatusUnauthorized,
						Message:  "invalid or expired jwt",
						Internal: err,
					}
				}
			} else {
				currSession, err := session.Get(defaultSessionID, c)

				type ErrorMessage struct {
					Error string `json:"error"`
				}

				errorMessage := &ErrorMessage{
					Error: "Please provide valid credentials",
				}

				if err != nil {
					return c.JSON(http.StatusUnauthorized, errorMessage)
				}
				if profile, ok := currSession.Values[googleProfileSessionKey].(*Profile); !ok || profile == nil {
					// call fetchProxyProfile if there is a proxy server
					if ProxyAuth != "" {
						profile, err := fetchProxyProfile(c)
						if err != nil {
							return c.JSON(http.StatusUnauthorized, errorMessage)
						}
						currSession.Values[googleProfileSessionKey] = stripProfile(profile)

						email = profile.Email
						imageurl = profile.Picture

					} else {
						return c.JSON(http.StatusUnauthorized, errorMessage)
					}
				} else {
					email = profile.Email
					imageurl = profile.ImageURL
				}

				currSession.Save(c.Request(), c.Response())
			}

			// set email so logger can potentially read
			c.Set("email", email)
			c.Set("imageurl", imageurl)

			// check authorize if it exists
			if s.enableAuthorize {
				if isAuthorized := s.config.AuthorizeChecker.Authorize(email, authLevel); !isAuthorized {
					return &echo.HTTPError{
						Code:     http.StatusUnauthorized,
						Message:  "unauthorized user",
						Internal: fmt.Errorf("user not authorized"),
					}
				} else {
					// level exists
					level, _ := s.config.AuthorizeChecker.Level(email)
					levelstr, _ := StringFromLevel(level)
					c.Set("level", levelstr)
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
func InitializeEchoSecure(e *echo.Echo, config SecureConfig, secret []byte, sessionID string) (*EchoSecure, error) {
	// setup logging and panic recover
	manCert := false
	if config.SSLCert != "" && config.SSLKey != "" {
		manCert = true
	}
	defaultSessionID = sessionID
	defaultDomain = config.Hostname
	defaultProxyInsecure = config.ProxyInsecure
	defaultHostName = "https://" + config.Hostname
	parts := strings.Split(config.Hostname, ".")
	if len(parts) >= 2 {
		defaultDomain = parts[len(parts)-2] + "." + parts[len(parts)-1]
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

	s := &EchoSecure{e, secret, enableAuthenticate, enableAuthorize, manCert, config}
	ProxyAuth = config.ProxyAuth

	if enableAuthenticate {
		JWTSecret = secret

		// Load blocked tokens from file if specified
		if config.TokenBlocklistFile != "" {
			if err := LoadBlockedTokensFromFile(config.TokenBlocklistFile); err != nil {
				return nil, fmt.Errorf("failed to load token blocklist: %v", err)
			}
		}

		// swagger:operation GET /login user loginHandler
		//
		// Login user
		//
		// Login user redirecting to profile
		//
		// ---
		// responses:
		//   302:
		//     description: "Redirect to /profile"
		e.GET("/login", loginHandler)
		e.GET("/oauth2callback", oauthCallbackHandler)

		// requires login

		// swagger:operation POST /logout user logoutHandler
		//
		// Logout user
		//
		// Clears session cookie for the user
		//
		// ---
		// responses:
		//   200:
		//     description: "successful operation"
		// security:
		// - Bearer: []
		e.POST("/logout", s.AuthMiddleware(NOAUTH)(logoutHandler))

		// swagger:operation GET /profile user profileHandler
		//
		// Returns user information
		//
		// Returns user information
		//
		// ---
		// responses:
		//   200:
		//     description: "successful operation"
		// security:
		// - Bearer: []
		e.GET("/profile", s.AuthMiddleware(NOAUTH)(profileHandler))

		// swagger:operation GET /token user tokenHandler
		//
		// Returns JWT user bearer token
		//
		// JWT token should be passed in header for authentication
		//
		// ---
		// responses:
		//   200:
		//     description: "successful operation"
		// security:
		// - Bearer: []
		e.GET("/token", s.AuthMiddleware(NOAUTH)(tokenHandler))
	}

	// return object
	return s, nil
}

func (s EchoSecure) StartEchoSecure(port int) {
	portstr := strconv.Itoa(port)
	portstr2 := portstr
	if ProxyPort != 0 {
		portstr2 = strconv.Itoa(ProxyPort)
	}
	defaultHostName = defaultHostName + ":" + portstr2

	if s.enableAuthenticate {
		// setup oauth object
		redirectURL := "https://" + s.config.Hostname + ":" + portstr2 + "/oauth2callback"
		configureOAuthClient(s.config.ClientID, s.config.ClientSecret, redirectURL)
	}

	if s.manCert {
		s.e.Logger.Fatal(s.e.StartTLS(":"+portstr, s.config.SSLCert, s.config.SSLKey))
	} else {
		s.e.Logger.Fatal(s.e.StartAutoTLS(":" + portstr))
	}
}
