package secure

import (
	"bufio"
	"crypto/tls"
	"encoding/gob"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	uuid "github.com/satori/go.uuid"
)

const (
	// The following keys are used for the default session. For example:
	//  session, _ := bookshelf.SessionStore.New(r, defaultSessionID)
	//  session.Values[oauthTokenSessionKey]
	googleProfileSessionKey = "google_profile"
	oauthTokenSessionKey    = "oauth_token"

	// This key is used in the OAuth flow session to store the URL to redirect the
	// user to after the OAuth flow is complete.
	oauthFlowRedirectKey = "redirect"

	AlgorithmHS256 = "HS256"

	// TODO: cookies set to expire ~ 6 months from now. This is a patch
	// to stop ugly errors from occurring in neuprint for at least six
	// months. This is not a permanent fix!!!
	COOKIEEXPIRE = 86400 * 30 * 6 // six months to expire
)

// global to hold oauth configuration
var OAuthConfig *oauth2.Config
var JWTSecret []byte
var ProxyAuth string
var defaultSessionID string
var defaultDomain string
var defaultHostName string
var defaultProxyInsecure bool

func init() {
	// Gob encoding for gorilla/sessions
	gob.Register(&oauth2.Token{})
	gob.Register(&Profile{})
}

func configureOAuthClient(clientID, clientSecret, url string) {
	OAuthConfig = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  url,
		Scopes:       []string{"email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

type jwtCustomClaims struct {
	Email    string `json:"email"`
	Level    string `json:"level"`
	ImageURL string `json:"image-url"`
	jwt.RegisteredClaims
}

// loginHandler initiates an OAuth flow to authenticate the user.
func loginHandler(c echo.Context) error {
	sessionID := uuid.NewV4().String()
	r := c.Request()
	w := c.Response()

	redirectURL, err := validateRedirectURL(c.FormValue("redirect"))
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %v", err)
	}

	auto := c.QueryParam("auto")
	if auto == "true" {
		// check if already logged in
		if currSession, err := session.Get(defaultSessionID, c); err == nil {
			if profile, ok := currSession.Values[googleProfileSessionKey].(*Profile); ok && profile != nil {
				currSession.Save(c.Request(), c.Response())
				return c.Redirect(http.StatusFound, redirectURL)
			}
		}
	}

	// redirect to the proxy server for login if available
	if ProxyAuth != "" {
		//return c.Redirect(http.StatusFound, ProxyAuth+"/login?"+c.QueryString())
		if redirectURL[0] == '/' {
			redirectURL = defaultHostName + redirectURL
		}
		if auto == "true" {
			redirectURL = redirectURL + "&auto=true"
		}
		redirectURL = url.QueryEscape(redirectURL)
		return c.Redirect(http.StatusFound, ProxyAuth+"/login?redirect="+redirectURL)
	}

	oauthFlowSession, err := session.Get(sessionID, c)
	if err != nil {
		return fmt.Errorf("could not create oauth session: %v", err)
	}
	oauthFlowSession.Options = &sessions.Options{
		MaxAge:   600,
		HttpOnly: true,
	}

	oauthFlowSession.Values[oauthFlowRedirectKey] = redirectURL

	if err := oauthFlowSession.Save(r, w); err != nil {
		return fmt.Errorf("could not save session: %v", err)
	}

	// Use the session ID for the "state" parameter.
	// This protects against CSRF (cross-site request forgery).
	// See https://godoc.org/golang.org/x/oauth2#Config.AuthCodeURL for more detail.
	//url := OAuthConfig.AuthCodeURL(sessionID, oauth2.AccessTypeOnline)
	url := OAuthConfig.AuthCodeURL(sessionID, oauth2.SetAuthURLParam("prompt", "consent"), oauth2.SetAuthURLParam("access_type", "online"))
	return c.Redirect(http.StatusFound, url)
}

// validateRedirectURL checks that the URL provided is valid.
// If the URL is missing, redirect the user to the application's root.
// The URL must not be absolute (i.e., the URL must refer to a path within this
// application).
func validateRedirectURL(path string) (string, error) {
	if path == "" {
		return "/profile", nil
	}

	// add check to make sure redirect is either local or from the same domain.
	// non local query
	if path[0] != '/' {
		parsedURL, err := url.Parse(path)
		if err != nil {
			return "/profile", err
		}
		parts := strings.Split(parsedURL.Hostname(), ".")
		if len(parts) >= 2 {
			redDomain := parts[len(parts)-2] + "." + parts[len(parts)-1]
			if redDomain != defaultDomain {
				return "/profile", errors.New("Redirect must be to same domain")
			}
		}

	}

	/*
		if parsedURL.IsAbs() {
			return "/profile", errors.New("URL must not be absolute")
		}
	*/
	return path, nil
}

// oauthCallbackHandler completes the OAuth flow, retreives the user's profile
// information and stores it in a session.
func oauthCallbackHandler(c echo.Context) error {
	oauthFlowSession, err := session.Get(c.FormValue("state"), c)
	if err != nil {
		return fmt.Errorf("invalid state parameter. try logging in again.")
	}

	redirectURL, ok := oauthFlowSession.Values[oauthFlowRedirectKey].(string)
	// Validate this callback request came from the app.
	if !ok {
		return fmt.Errorf("invalid state parameter. try logging in again.")
	}

	code := c.FormValue("code")
	tok, err := OAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("could not get auth token: %v", err)
	}

	sessionNew, err := session.Get(defaultSessionID, c)
	if err != nil {
		return fmt.Errorf("could not get default session: %v", err)
	}

	ctx := context.Background()
	profile, err := fetchProfile(ctx, tok)
	if err != nil {
		return fmt.Errorf("could not fetch Google profile: %v", err)
	}

	sessionNew.Values[oauthTokenSessionKey] = tok
	// Strip the profile to only the fields we need. Otherwise the struct is too big.
	sessionNew.Values[googleProfileSessionKey] = stripProfile(profile)

	// set domain
	sessionNew.Options.Domain = defaultDomain

	if err := sessionNew.Save(c.Request(), c.Response()); err != nil {

		return fmt.Errorf("could not save session: %v", err)
	}

	return c.Redirect(http.StatusFound, redirectURL)
}

type userInfo struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Profile       string `json:"profile"`
	Picture       string `json:"picture"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Gender        string `json:"gender"`
}

// fetchProxyProfile retrieves a profile from the proxy server if the user is logged in.
// Cookies need to be sent in the request
func fetchProxyProfile(c echo.Context) (*userInfo, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if !defaultProxyInsecure {
		tr = nil
	}
	client := http.Client{
		Timeout:   time.Second * 60,
		Transport: tr,
	}
	req, err := http.NewRequest(http.MethodGet, ProxyAuth+"/profile", nil)
	if err != nil {
		return nil, fmt.Errorf("profile request failed")
	}
	req.Header.Set("Content-Type", "application/json")

	// copy cookies
	cookies := c.Request().Cookies()
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// if we don't get a 200 response back from the proxied auth server, then there is no reason to
	// check the profile information and we should just bailout with the error status.
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}

	// load profile information into google profile struct
	var result userInfo
	var tresult ProfileAuth
	if err := json.Unmarshal(data, &tresult); err != nil {
		return nil, err
	}
	result.Email = tresult.Email
	result.Picture = tresult.ImageURL

	return &result, nil
}

// fetchProfile retrieves the Google+ profile of the user associated with the
// provided OAuth token.
func fetchProfile(ctx context.Context, tok *oauth2.Token) (*userInfo, error) {
	client := oauth2.NewClient(ctx, OAuthConfig.TokenSource(ctx, tok))

	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result userInfo
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// logoutHandler clears the default session.
func logoutHandler(c echo.Context) error {
	// post logout to the proxy as well
	if ProxyAuth != "" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		if !defaultProxyInsecure {
			tr = nil
		}
		client := http.Client{
			Timeout:   time.Second * 60,
			Transport: tr,
		}
		req, err := http.NewRequest(http.MethodPost, ProxyAuth+"/logout", nil)
		if err != nil {
			return fmt.Errorf("logout request failed")
		}

		// copy cookies
		cookies := c.Request().Cookies()
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}

		_, err = client.Do(req)
		if err != nil {
			return err
		}
	}

	currSession, err := session.Get(defaultSessionID, c)
	if err != nil {
		return fmt.Errorf("could not get default session: %v", err)
	}
	currSession.Options.MaxAge = -1 // Clear session.

	/*if err := currSession.Save(c.Request(), c.Response()); err != nil {
		return fmt.Errorf("could not save session: %v", err)
	}
	redirectURL := c.FormValue("redirect")
	if redirectURL == "" {
		redirectURL = "/"
	}*/

	return c.HTML(http.StatusOK, "")
}

// profileFromSession retreives the Google+ profile from the default session.
// Returns nil if the profile cannot be retreived (e.g. user is logged out).
func profileFromSession(c echo.Context) *Profile {
	user, ok := c.Get("user").(*jwt.Token)
	if ok {
		claims := user.Claims.(*jwtCustomClaims)
		email := claims.Email
		url := claims.ImageURL
		return &Profile{url, email}
	}

	currSession, err := session.Get(defaultSessionID, c)
	if err != nil {
		return nil
	}
	profile, ok := currSession.Values[googleProfileSessionKey].(*Profile)
	if !ok {
		return nil
	}
	return profile
}

func profileHandler(c echo.Context) error {
	profile := profileFromSession(c)
	level, ok := c.Get("level").(string)
	if !ok {
		level = "noauth"
	}
	profileout := &ProfileAuth{profile, level}
	return c.JSON(http.StatusOK, profileout)
}

func tokenHandler(c echo.Context) error {
	// Set claims
	profile := profileFromSession(c)

	level, ok := c.Get("level").(string)
	if !ok {
		level = "noauth"
	}

	claims := &jwtCustomClaims{
		profile.Email,
		level,
		profile.ImageURL,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 50000)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token and send it as response.
	t, err := token.SignedString(JWTSecret)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{
		"token": t,
	})
}

type Profile struct {
	ImageURL, Email string
}

type ProfileAuth struct {
	*Profile
	AuthLevel string
}

// stripProfile returns a subset of the user profile.
func stripProfile(p *userInfo) *Profile {
	return &Profile{
		ImageURL: p.Picture + "?sz=50",
		Email:    p.Email,
	}
}

// AddTokenToBlocklist adds a JWT token to the global blocklist
func AddTokenToBlocklist(token string) {
	globalTokenBlocklist.AddToken(token)
}

// RemoveTokenFromBlocklist removes a JWT token from the global blocklist
func RemoveTokenFromBlocklist(token string) {
	globalTokenBlocklist.RemoveToken(token)
}

// LoadBlockedTokens loads a list of blocked tokens into the global blocklist
func LoadBlockedTokens(tokens []string) {
	globalTokenBlocklist.LoadTokensFromSlice(tokens)
}

// LoadBlockedTokensFromFile reads blocked tokens from a file and loads them into the global blocklist
// The file should contain one token per line, with optional comments starting with #
func LoadBlockedTokensFromFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open blocklist file %s: %v", filePath, err)
	}
	defer file.Close()

	var tokens []string
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		tokens = append(tokens, line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading blocklist file %s at line %d: %v", filePath, lineNumber, err)
	}

	// Load all tokens into the blocklist
	globalTokenBlocklist.LoadTokensFromSlice(tokens)

	// Log the number of tokens loaded
	tokenCount := globalTokenBlocklist.Count()
	fmt.Printf("Loaded %d blocked tokens from %s\n", tokenCount, filePath)

	return nil
}
