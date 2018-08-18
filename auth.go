package main

import (
	"encoding/gob"
	"errors"
	"net/http"
	"net/url"

	plus "google.golang.org/api/plus/v1"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
        "github.com/gorilla/sessions"

	uuid "github.com/satori/go.uuid"
        "github.com/labstack/echo-contrib/session"
        "github.com/labstack/echo"
        "fmt"
)


const (
	defaultSessionID = "neuPrintHTTP"
	// The following keys are used for the default session. For example:
	//  session, _ := bookshelf.SessionStore.New(r, defaultSessionID)
	//  session.Values[oauthTokenSessionKey]
	googleProfileSessionKey = "google_profile"
	oauthTokenSessionKey    = "oauth_token"

	// This key is used in the OAuth flow session to store the URL to redirect the
	// user to after the OAuth flow is complete.
	oauthFlowRedirectKey = "redirect"
)

func init() {
	// Gob encoding for gorilla/sessions
	gob.Register(&oauth2.Token{})
	gob.Register(&Profile{})
}

// loginHandler initiates an OAuth flow to authenticate the user.
func loginHandler(c echo.Context) error {
        // ?! ?? will this auto login if signed in (how do I not require auth to call)

	sessionID := uuid.Must(uuid.NewV4()).String()
        r := c.Request()
        w := c.Response()

        oauthFlowSession, err  := session.Get(sessionID, c)
	if err != nil {
		return fmt.Errorf("could not create oauth session: %v", err)
	}
        oauthFlowSession.Options = &sessions.Options{
            MaxAge:   10 * 60,
            HttpOnly: true,
        }

	redirectURL, err := validateRedirectURL(c.FormValue("redirect"))
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %v", err)
	}
	oauthFlowSession.Values[oauthFlowRedirectKey] = redirectURL

	if err := oauthFlowSession.Save(r, w); err != nil {
		return fmt.Errorf("could not save session: %v", err)
	}

	// Use the session ID for the "state" parameter.
	// This protects against CSRF (cross-site request forgery).
	// See https://godoc.org/golang.org/x/oauth2#Config.AuthCodeURL for more detail.
	url := OAuthConfig.AuthCodeURL(sessionID, oauth2.AccessTypeOnline)
	return c.Redirect(http.StatusFound, url)
}

// validateRedirectURL checks that the URL provided is valid.
// If the URL is missing, redirect the user to the application's root.
// The URL must not be absolute (i.e., the URL must refer to a path within this
// application).
func validateRedirectURL(path string) (string, error) {
	if path == "" {
		return "/", nil
	}

	// Ensure redirect URL is valid and not pointing to a different server.
	parsedURL, err := url.Parse(path)
	if err != nil {
		return "/", err
	}
	if parsedURL.IsAbs() {
		return "/", errors.New("URL must not be absolute")
	}
	return path, nil
}

// oauthCallbackHandler completes the OAuth flow, retreives the user's profile
// information and stores it in a session.
func oauthCallbackHandler(c echo.Context) error {
        oauthFlowSession, err  := session.Get(c.FormValue("state"), c)
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
	if err := sessionNew.Save(c.Request(), c.Response()); err != nil {
		return fmt.Errorf("could not save session: %v", err)
	}

	return c.Redirect(http.StatusFound, redirectURL)
}

// fetchProfile retrieves the Google+ profile of the user associated with the
// provided OAuth token.
func fetchProfile(ctx context.Context, tok *oauth2.Token) (*plus.Person, error) {
	client := oauth2.NewClient(ctx, OAuthConfig.TokenSource(ctx, tok))
	plusService, err := plus.New(client)
	if err != nil {
		return nil, err
	}
	return plusService.People.Get("me").Do()
}

func authMiddleWare(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
            _, err  := session.Get(defaultSessionID, c)
            if err != nil {
	        return c.Redirect(http.StatusFound, c.Request().URL.Path)
            }
            
            return next(c)
	}
}

// logoutHandler clears the default session.
func logoutHandler(c echo.Context) error {
        currSession, err  := session.Get(defaultSessionID, c)
	if err != nil {
		return fmt.Errorf("could not get default session: %v", err)
	}
	currSession.Options.MaxAge = -1 // Clear session.
	if err := currSession.Save(c.Request(), c.Response()); err != nil {
		return fmt.Errorf("could not save session: %v", err)
	}
	redirectURL := c.FormValue("redirect")
	if redirectURL == "" {
		redirectURL = "/"
	}
	
        // ?! ?? do I need to logout from service
        
	return c.Redirect(http.StatusFound, redirectURL)
}

// profileFromSession retreives the Google+ profile from the default session.
// Returns nil if the profile cannot be retreived (e.g. user is logged out).
func profileFromSession(c echo.Context) *Profile {
        currSession, err  := session.Get(defaultSessionID, c)
	if err != nil {
		return nil
	}
	tok, ok := currSession.Values[oauthTokenSessionKey].(*oauth2.Token)
	if !ok || !tok.Valid() {
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
    return c.JSON(http.StatusOK, profile)
}

type Profile struct {
	ID, DisplayName, ImageURL, Email string
}

// stripProfile returns a subset of a plus.Person.
func stripProfile(p *plus.Person) *Profile {
	return &Profile{
		ID:          p.Id,
		DisplayName: p.DisplayName,
		ImageURL:    p.Image.Url,
                Email:       p.Emails[0].Value,
	}
}

