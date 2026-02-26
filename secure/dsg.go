package secure

import (
	"encoding/json"
	"fmt"
	"io"
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

// DSGUserCache mirrors the DatasetGateway /api/v1/user/cache response.
type DSGUserCache struct {
	ID            int                 `json:"id"`
	Email         string              `json:"email"`
	Name          string              `json:"name"`
	Admin         bool                `json:"admin"`
	Groups        []string            `json:"groups"`
	PermissionsV2 map[string][]string `json:"permissions_v2"`
	DatasetsAdmin []string            `json:"datasets_admin"`
}

type cachedEntry struct {
	data      *DSGUserCache
	fetchedAt time.Time
}

// DSGClient validates tokens against a DatasetGateway instance.
type DSGClient struct {
	BaseURL    string
	CacheTTL   time.Duration
	DatasetMap map[string]string // neuprint DB name → DSG dataset slug
	cache      sync.Map          // token string → *cachedEntry
	client     *http.Client
}

// NewDSGClient creates a DSGClient with sensible defaults.
func NewDSGClient(baseURL string, cacheTTLSeconds int, datasetMap map[string]string) *DSGClient {
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = 300
	}
	if datasetMap == nil {
		datasetMap = map[string]string{}
	}
	return &DSGClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		CacheTTL:   time.Duration(cacheTTLSeconds) * time.Second,
		DatasetMap: datasetMap,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchUser validates a token and returns the user cache, or nil if invalid.
func (d *DSGClient) FetchUser(token string) (*DSGUserCache, error) {
	// Check cache
	if val, ok := d.cache.Load(token); ok {
		entry := val.(*cachedEntry)
		if time.Since(entry.fetchedAt) < d.CacheTTL {
			return entry.data, nil
		}
		d.cache.Delete(token)
	}

	// Call DatasetGateway
	req, err := http.NewRequest("GET", d.BaseURL+"/api/v1/user/cache", nil)
	if err != nil {
		return nil, fmt.Errorf("dsg: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dsg: auth service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, nil // invalid token
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dsg: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var user DSGUserCache
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("dsg: failed to decode response: %w", err)
	}

	d.cache.Store(token, &cachedEntry{data: &user, fetchedAt: time.Now()})
	return &user, nil
}

// DatasetSlug maps a neuprint database name to a DSG dataset slug.
func (d *DSGClient) DatasetSlug(neuprintDB string) string {
	if slug, ok := d.DatasetMap[neuprintDB]; ok {
		return slug
	}
	// Strip version suffix: "hemibrain:v1.2" → "hemibrain"
	if idx := strings.Index(neuprintDB, ":"); idx >= 0 {
		return neuprintDB[:idx]
	}
	return neuprintDB
}

// DatasetLevel returns the neuprint AuthorizationLevel for a user on a dataset.
func (d *DSGClient) DatasetLevel(user *DSGUserCache, neuprintDB string) AuthorizationLevel {
	if user.Admin {
		return ADMIN
	}
	slug := d.DatasetSlug(neuprintDB)
	perms, ok := user.PermissionsV2[slug]
	if !ok {
		return NOAUTH
	}
	level := NOAUTH
	for _, p := range perms {
		switch p {
		case "admin":
			return ADMIN
		case "manage":
			if level < ADMIN {
				level = READWRITE
			}
		case "edit":
			if level < READWRITE {
				level = READWRITE
			}
		case "view":
			if level < READ {
				level = READ
			}
		}
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

// RequireDatasetAccess checks that the authenticated user has at least the
// given AuthorizationLevel on the specified neuprint dataset. Call this from
// API handlers after the authentication middleware has run.
func RequireDatasetAccess(c echo.Context, dataset string, level AuthorizationLevel) error {
	userVal := c.Get("dsg_user")
	if userVal == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	user := userVal.(*DSGUserCache)

	client := c.Get("dsg_client").(*DSGClient)
	actual := client.DatasetLevel(user, dataset)
	if actual < level {
		return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions for dataset")
	}
	c.Set("level", StringFromLevel(actual))
	return nil
}

// DSGAuthMiddleware validates the dsg_token and populates the echo context
// with the authenticated user. It performs authentication only — per-dataset
// authorization is done by handlers via RequireDatasetAccess.
func DSGAuthMiddleware(client *DSGClient) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := ExtractToken(c)
			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			user, err := client.FetchUser(token)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, "auth service unavailable")
			}
			if user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			c.Set("dsg_user", user)
			c.Set("dsg_client", client)
			c.Set("email", user.Email)

			return next(c)
		}
	}
}

// DSGAdminMiddleware requires the authenticated user to be a global admin.
// Must be used after DSGAuthMiddleware.
func DSGAdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userVal := c.Get("dsg_user")
			if userVal == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
			}
			user := userVal.(*DSGUserCache)
			if !user.Admin {
				return echo.NewHTTPError(http.StatusForbidden, "admin access required")
			}
			return next(c)
		}
	}
}
