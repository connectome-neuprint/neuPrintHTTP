package secure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// AuthorizationLevel indicates the access level for a given API.
type AuthorizationLevel int

const (
	NOAUTH    AuthorizationLevel = iota
	READ                         // view
	READWRITE                    // edit
	ADMIN                        // admin
)

// AccessLevel is an alias for backward compatibility.
type AccessLevel = AuthorizationLevel

// StringFromLevel returns the string name of a level.
func StringFromLevel(level AuthorizationLevel) string {
	switch level {
	case READ:
		return "readwrite" // keep neuprint-compatible string
	case READWRITE:
		return "readwrite"
	case ADMIN:
		return "admin"
	default:
		return "noauth"
	}
}

// DSGIdentity is the DSG-native authenticated principal shape.
type DSGIdentity struct {
	ID             int    `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	PictureURL     string `json:"picture_url"`
	Admin          bool   `json:"admin"`
	ServiceAccount bool   `json:"service_account"`
}

// DSGDecision is a DSG-native authorization decision for one dataset entry.
type DSGDecision struct {
	Name     string   `json:"name"`
	Version  string   `json:"version,omitempty"`
	Decision string   `json:"decision"`
	Roles    []string `json:"roles"`
	TOSURL   string   `json:"tos_url,omitempty"`
}

func (d *DSGDecision) Level() AuthorizationLevel {
	if d == nil {
		return NOAUTH
	}
	switch d.Decision {
	case "allow":
		return levelFromRoles(d.Roles)
	case "service_eval":
		log.Printf("dsg: service_eval decision for %s:%s denied locally", d.Name, d.Version)
		return NOAUTH
	default:
		return NOAUTH
	}
}

func (d *DSGDecision) TOSRequired() bool {
	return d != nil && d.Decision == "tos_required"
}

type cachedIdentityEntry struct {
	data      *DSGIdentity
	fetchedAt time.Time
}

type cachedDecisionEntry struct {
	data      *DSGDecision
	fetchedAt time.Time
}

type decisionCacheKey struct {
	token   string
	name    string
	version string
}

type authorizeEntry struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Permission string `json:"permission"`
}

type authorizeRequest struct {
	Service   string           `json:"service"`
	ReturnURL string           `json:"return_url"`
	Entries   []authorizeEntry `json:"entries"`
}

type authorizeResponse struct {
	Entries []DSGDecision `json:"entries"`
}

// DSGClient validates tokens and dataset decisions against DatasetGateway.
type DSGClient struct {
	BaseURL       string
	CacheTTL      time.Duration
	ServiceName   string
	identityCache sync.Map
	decisionCache sync.Map
	client        *http.Client
}

// NewDSGClient creates a DSGClient with sensible defaults.
func NewDSGClient(baseURL string, cacheTTLSeconds int, serviceName string) *DSGClient {
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = 300
	}
	if serviceName == "" {
		serviceName = "neuprint"
	}
	return &DSGClient{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		CacheTTL:    time.Duration(cacheTTLSeconds) * time.Second,
		ServiceName: serviceName,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

// SetHTTPClient swaps the HTTP client. It is intended for tests.
func (d *DSGClient) SetHTTPClient(client *http.Client) {
	if client != nil {
		d.client = client
	}
}

func (d *DSGClient) fetchIdentity(token string, forceRefresh bool) (*DSGIdentity, error) {
	if token == "" {
		return nil, nil
	}
	if !forceRefresh {
		if val, ok := d.identityCache.Load(token); ok {
			entry := val.(*cachedIdentityEntry)
			if time.Since(entry.fetchedAt) < d.CacheTTL {
				return entry.data, nil
			}
			d.identityCache.Delete(token)
		}
	} else {
		d.identityCache.Delete(token)
	}

	req, err := http.NewRequest("GET", d.BaseURL+"/api/dsg/v1/user", nil)
	if err != nil {
		return nil, fmt.Errorf("dsg: failed to create identity request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dsg: auth service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dsg: unexpected identity status %d: %s", resp.StatusCode, string(body))
	}

	var identity DSGIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return nil, fmt.Errorf("dsg: failed to decode identity response: %w", err)
	}

	d.identityCache.Store(token, &cachedIdentityEntry{data: &identity, fetchedAt: time.Now()})
	return &identity, nil
}

// Identity validates a token and returns the principal identity, or nil if invalid.
func (d *DSGClient) Identity(token string) (*DSGIdentity, error) {
	return d.fetchIdentity(token, false)
}

// IdentityFresh validates a token while bypassing the local identity cache.
func (d *DSGClient) IdentityFresh(token string) (*DSGIdentity, error) {
	return d.fetchIdentity(token, true)
}

// DatasetDecision returns the DSG decision for one neuPrint dataset name.
func (d *DSGClient) DatasetDecision(token, dataset, returnURL string, forceRefresh bool) (*DSGDecision, error) {
	decisions, err := d.AuthorizeDatasets(token, []string{dataset}, returnURL, forceRefresh)
	if err != nil {
		return nil, err
	}
	return decisions[dataset], nil
}

// AuthorizeDatasets returns DSG decisions for the supplied neuPrint dataset names.
func (d *DSGClient) AuthorizeDatasets(token string, datasets []string, returnURL string, forceRefresh bool) (map[string]*DSGDecision, error) {
	results := make(map[string]*DSGDecision, len(datasets))
	if len(datasets) == 0 {
		return results, nil
	}

	type miss struct {
		dataset string
		key     decisionCacheKey
		entry   authorizeEntry
	}
	misses := make([]miss, 0, len(datasets))

	for _, dataset := range datasets {
		key, entry := decisionKeyAndEntry(token, dataset)
		if forceRefresh {
			d.decisionCache.Delete(key)
		}
		if !forceRefresh {
			if val, ok := d.decisionCache.Load(key); ok {
				cached := val.(*cachedDecisionEntry)
				if time.Since(cached.fetchedAt) < d.CacheTTL {
					results[dataset] = cached.data
					continue
				}
				d.decisionCache.Delete(key)
			}
		}
		misses = append(misses, miss{dataset: dataset, key: key, entry: entry})
	}

	if len(misses) == 0 {
		return results, nil
	}

	body := authorizeRequest{
		Service:   d.ServiceName,
		ReturnURL: returnURL,
		Entries:   make([]authorizeEntry, 0, len(misses)),
	}
	for _, miss := range misses {
		body.Entries = append(body.Entries, miss.entry)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("dsg: failed to encode authorize request: %w", err)
	}

	req, err := http.NewRequest("POST", d.BaseURL+"/api/dsg/v1/authorize", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("dsg: failed to create authorize request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dsg: authorize service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dsg: unexpected authorize status %d: %s", resp.StatusCode, string(body))
	}

	var decoded authorizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("dsg: failed to decode authorize response: %w", err)
	}
	if len(decoded.Entries) != len(misses) {
		return nil, fmt.Errorf("dsg: authorize returned %d entries for %d requests", len(decoded.Entries), len(misses))
	}

	now := time.Now()
	for i := range decoded.Entries {
		decision := decoded.Entries[i]
		decisionCopy := decision
		results[misses[i].dataset] = &decisionCopy
		if decision.Decision != "tos_required" {
			d.decisionCache.Store(
				misses[i].key,
				&cachedDecisionEntry{data: &decisionCopy, fetchedAt: now},
			)
		}
	}

	return results, nil
}

func decisionKeyAndEntry(token, dataset string) (decisionCacheKey, authorizeEntry) {
	name, version := splitNeuPrintDataset(dataset)
	key := decisionCacheKey{token: token, name: name, version: version}
	entry := authorizeEntry{Name: name, Permission: "view"}
	if version != "" {
		entry.Version = version
	}
	return key, entry
}

func splitNeuPrintDataset(dataset string) (string, string) {
	name, version, found := strings.Cut(dataset, ":")
	if !found {
		return dataset, ""
	}
	return name, version
}

func levelFromRoles(roles []string) AuthorizationLevel {
	level := NOAUTH
	for _, role := range roles {
		switch role {
		case "admin":
			return ADMIN
		case "manage", "edit":
			if level < READWRITE {
				level = READWRITE
			}
		case "view":
			if level < READ {
				level = READ
			}
		}
	}
	if level < READ {
		return READ
	}
	return level
}

// ExtractToken reads the dsg_token from the request in priority order:
// 1. Authorization: Bearer header
// 2. dsg_token cookie
// 3. dsg_token query parameter
func ExtractToken(c echo.Context) string {
	// Bearer header
	auth := c.Request().Header.Get(echo.HeaderAuthorization)
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):]
	}
	// Cookie
	if cookie, err := c.Cookie("dsg_token"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	// Query parameter
	if token := c.QueryParam("dsg_token"); token != "" {
		return token
	}
	return ""
}

// RequireDatasetAccess checks that the authenticated identity has at least the
// given AuthorizationLevel on the specified neuPrint dataset. Call this from
// API handlers after the authentication middleware has run.
func RequireDatasetAccess(c echo.Context, dataset string, level AuthorizationLevel) error {
	identityVal := c.Get("dsg_identity")
	if identityVal == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	identity := identityVal.(*DSGIdentity)
	if identity.Admin {
		c.Set("level", StringFromLevel(ADMIN))
		return nil
	}

	clientVal := c.Get("dsg_client")
	if clientVal == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	client := clientVal.(*DSGClient)

	token, _ := c.Get("dsg_token").(string)
	if token == "" {
		token = ExtractToken(c)
	}
	if token == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	decision, err := client.DatasetDecision(token, dataset, currentRequestURL(c), false)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "auth service unavailable")
	}
	actual := decision.Level()
	if actual < level {
		if decision.TOSRequired() {
			return c.JSON(http.StatusForbidden, map[string]interface{}{
				"message":      "Terms of Service acceptance required for this dataset",
				"tos_required": true,
				"tos_url":      decision.TOSURL,
				"dataset":      dataset,
			})
		}
		return echo.NewHTTPError(http.StatusForbidden, "You do not have access to "+dataset+" dataset")
	}
	c.Set("level", StringFromLevel(actual))
	return nil
}

func currentRequestURL(c echo.Context) string {
	req := c.Request()
	if req == nil {
		return "/"
	}
	scheme := "https"
	if req.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + req.Host + req.URL.RequestURI()
}

// DSGAuthMiddleware validates the dsg_token and populates the echo context
// with the authenticated identity. It performs authentication only —
// per-dataset authorization is done by handlers via RequireDatasetAccess.
func DSGAuthMiddleware(client *DSGClient) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := ExtractToken(c)
			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			var identity *DSGIdentity
			var err error
			if c.Path() == "/profile" {
				identity, err = client.IdentityFresh(token)
			} else {
				identity, err = client.Identity(token)
			}
			if err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, "auth service unavailable")
			}
			if identity == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			c.Set("dsg_identity", identity)
			c.Set("dsg_client", client)
			c.Set("dsg_token", token)
			c.Set("email", identity.Email)

			return next(c)
		}
	}
}

// DSGAdminMiddleware requires the authenticated identity to be a global admin.
// Must be used after DSGAuthMiddleware.
func DSGAdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			identityVal := c.Get("dsg_identity")
			if identityVal == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
			}
			identity := identityVal.(*DSGIdentity)
			if !identity.Admin {
				return echo.NewHTTPError(http.StatusForbidden, "admin access required")
			}
			return next(c)
		}
	}
}
