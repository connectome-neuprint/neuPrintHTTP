# Migrate neuprintHTTP auth to DatasetGateway

## Context

neuprintHTTP is a Go/Echo REST API serving connectomics data from Neo4j.
It currently authenticates users via its own Google OAuth flow + Gorilla
session cookies, generates self-contained JWTs (HS256, 50,000hr expiry),
and authorizes via either a JSON file (`authorized.json`) mapping
`email → "admin"|"readwrite"|"readonly"` or a Google Datastore HTTP
endpoint.

We want to replace all of this with DatasetGateway (DSG), the same
auth service already used by clio-store and celltyping-light. Tokens
become `dsg_token` API keys (no more neuprint JWTs). Clean cutover,
no dual-mode transition.

## Decisions

- **Token type**: dsg_tokens (DSG API keys). neuprint's `/token`
  endpoint now proxies to DSG's `/api/v1/create_token`.
- **Migration**: Clean cutover. Only dsg_tokens accepted.
- **Import**: `--datasets` flag specifies which DSG datasets to grant
  on. Idempotent. Detailed per-dataset log of new vs existing users.

---

## Part 1: DatasetGateway changes

Only one change needed: a new management command.

### New file: `dsg/core/management/commands/import_neuprint_auth.py`

Follows the pattern of `import_clio_auth.py`.

**Arguments:**
- `json_file` — path to neuprint's `authorized.json`
- `--datasets DATASET [DATASET ...]` — required; DSG dataset slug(s)
  to create grants for
- `--dry-run` — preview without writing to DB

**Input format** (`authorized.json`):
```json
{
  "alice@gmail.com": "admin",
  "bob@gmail.com": "readwrite",
  "carol@gmail.com": "readonly"
}
```

**Permission mapping:**

| authorized.json level | DSG permission | Notes |
|---|---|---|
| `"readonly"` | `view` grant on each `--datasets` | |
| `"readwrite"` | `edit` grant on each `--datasets` | DSG hierarchy: edit implies view |
| `"admin"` | Sets `user.admin = True` | Global admin across all datasets |

**Logic:**
1. Load JSON, ensure `view`/`edit`/`admin` Permission objects exist
2. Ensure `user` Group exists
3. Create/get each dataset from `--datasets`
4. For each email in JSON:
   - `get_or_create` User (set unusable password, defaults)
   - Add to `user` group
   - If `"admin"` → set `user.admin = True`
   - Else create Grant (view or edit) on each dataset via `get_or_create`
   - Track whether user/grant was created or already existed
5. Print per-dataset summary log:
   ```
   Dataset: hemibrain
     Added: alice@gmail.com (admin), bob@gmail.com (edit)
     Already existed: carol@gmail.com (view)
   ```

**Reference:** `dsg/core/management/commands/import_clio_auth.py`
(same model imports, same `get_or_create` idempotency pattern)

### Documentation update

Add `import_neuprint_auth` to the management commands table in
`docs/user-manual.md` (line ~427).

---

## Part 2: neuprintHTTP changes

### 2.1 Config changes (`config/config.go`)

**Add fields:**
```go
DSGUrl       string            `json:"dsg-url,omitempty"`
DSGCacheTTL  int               `json:"dsg-cache-ttl,omitempty"`  // seconds, default 300
DatasetMap   map[string]string `json:"dataset-map,omitempty"`    // neuprint DB → DSG slug
```

**Remove fields:**
- `ClientID`, `ClientSecret` (Google OAuth — DSG handles this)
- `Secret` (JWT signing key)
- `AuthFile` (authorized.json path)
- `AuthDatastore`, `AuthToken` (Datastore backend)
- `ProxyAuth`, `ProxyInsecure` (proxy workaround)
- `TokenBlocklist` (DSG handles revocation)

**Keep:** `DisableAuth`, `Hostname`, `CertPEM`, `KeyPEM`, SSL/TLS fields

**New config example:**
```json
{
  "dsg-url": "https://dsg.janelia.org",
  "dsg-cache-ttl": 300,
  "dataset-map": {
    "hemibrain:v1.2.1": "hemibrain",
    "manc:v1.0": "manc"
  },
  "hostname": "neuprint.janelia.org"
}
```

### 2.2 New DSG auth module (`secure/dsg.go`)

**DSGUserCache struct** — mirrors DSG's `/api/v1/user/cache` response:
```go
type DSGUserCache struct {
    ID            int                 `json:"id"`
    Email         string              `json:"email"`
    Name          string              `json:"name"`
    Admin         bool                `json:"admin"`
    Groups        []string            `json:"groups"`
    PermissionsV2 map[string][]string `json:"permissions_v2"`
    DatasetsAdmin []string            `json:"datasets_admin"`
}
```

**DSGClient** — HTTP client with in-memory token cache:
```go
type DSGClient struct {
    BaseURL    string
    CacheTTL   time.Duration
    DatasetMap map[string]string // neuprint DB name → DSG dataset slug
    cache      sync.Map          // token string → *cachedEntry
    client     *http.Client
}
```

**Key methods:**

`FetchUser(token string) (*DSGUserCache, error)`:
1. Check cache; return if within TTL
2. `GET {baseURL}/api/v1/user/cache` with `Authorization: Bearer {token}`
3. 200 → parse, cache, return
4. 401 → return nil (invalid token)
5. Network/other error → return error

`(d *DSGClient) DatasetLevel(user *DSGUserCache, neuprintDB string) AuthorizationLevel`:
1. If `user.Admin` → ADMIN
2. Map neuprintDB → DSG slug via `d.DatasetMap` (explicit map, else strip `:version` suffix)
3. Look up `user.PermissionsV2[slug]`
4. Contains `"admin"` → ADMIN, `"manage"` or `"edit"` → READWRITE, `"view"` → READ
5. Missing → NOAUTH

`ExtractToken(c echo.Context) string`:
1. `Authorization: Bearer {token}` header
2. `dsg_token` cookie
3. `dsg_token` query parameter
4. Return empty string if none found

### 2.3 Middleware and authorization helpers (`secure/dsg.go`)

The old `Authorizer` interface was deleted entirely. Instead,
`secure/dsg.go` provides:

- `DSGAuthMiddleware(client)` — authentication-only middleware that
  validates the token and stores `*DSGUserCache` on the context
- `DSGAdminMiddleware()` — requires `user.Admin == true` (must run
  after `DSGAuthMiddleware`)
- `RequireDatasetAccess(c, dataset, level)` — per-dataset authorization
  check called from individual handlers

Because DSG permissions are **per-dataset** while the old system was
per-user-global, authorization moved from the middleware layer to
individual handler functions.

### 2.4 Middleware changes (`secure/secure.go`)

**Current flow:**
1. Extract Bearer JWT OR session cookie → get email
2. `Authorizer.Authorize(email, routeLevel)` → global check

**New flow:**
1. Extract dsg_token from header/cookie/query param
2. Call `DSGClient.FetchUser(token)` (cached)
3. Store `*DSGUserCache` in echo context as `"dsg_user"`
4. Store email in context as `"email"` (for logging compatibility)
5. **No authorization check at middleware level** — just authentication
6. Return 401 if no token or invalid token; 502 if DSG unreachable

**Per-dataset authorization moves to handlers.** The helper reads the
`dsg_client` and `dsg_user` from the echo context (set by middleware):

```go
func RequireDatasetAccess(c echo.Context, dataset string,
    level AuthorizationLevel) error {
    user := c.Get("dsg_user").(*DSGUserCache)
    client := c.Get("dsg_client").(*DSGClient)
    actual := client.DatasetLevel(user, dataset)
    if actual < level {
        return echo.NewHTTPError(http.StatusForbidden,
            "insufficient permissions for dataset")
    }
    c.Set("level", StringFromLevel(actual))
    return nil
}
```

**Route group changes in `main.go`:**
- `readGrp` middleware: authentication only (token valid?)
- Admin routes: use `DSGAdminMiddleware()` via `SetAdminRoute`

For the `/api/custom/custom` endpoint (and similar), add the
per-dataset check at the top of the handler:

```go
// In getCustom handler, after binding req:
if err := secure.RequireDatasetAccess(c, req.Dataset, secure.READ); err != nil {
    return err
}
```

For admin-only routes (raw cypher), `DSGAdminMiddleware` checks
`user.Admin`, and handlers additionally call `RequireDatasetAccess`
with `secure.ADMIN` for per-dataset checks.

### 2.5 Replace OAuth login flow (`secure/auth.go`)

**Remove:**
- Google OAuth configuration (`configureOAuthClient`)
- `loginHandler` (direct Google OAuth flow)
- `oauthCallbackHandler`
- `fetchProfile`, `fetchProxyProfile`
- JWT generation (`tokenHandler`)
- Session management (Gorilla sessions)
- Token blocklist code

**Replace with:**

`GET /login`:
```go
func dsgLoginHandler(dsgURL string) echo.HandlerFunc {
    return func(c echo.Context) error {
        redirect := c.QueryParam("redirect")
        if redirect == "" {
            redirect = "/profile"
        }
        return c.Redirect(http.StatusFound,
            dsgURL+"/api/v1/authorize?redirect="+url.QueryEscape(redirect))
    }
}
```

`POST /logout`:
```go
func dsgLogoutHandler(dsgURL string) echo.HandlerFunc {
    return func(c echo.Context) error {
        return c.Redirect(http.StatusFound, dsgURL+"/api/v1/logout")
    }
}
```

`GET /profile` — fetch from DSG user cache already in context:
```go
func dsgProfileHandler(c echo.Context) error {
    user := c.Get("dsg_user").(*DSGUserCache)
    return c.JSON(http.StatusOK, map[string]interface{}{
        "Email":     user.Email,
        "AuthLevel": "...",  // derive from highest permission
    })
}
```

`GET /token` — proxies to DSG's token creation endpoint:
```go
func dsgTokenHandler(dsgURL string) echo.HandlerFunc {
    return func(c echo.Context) error {
        token := ExtractToken(c)
        req, _ := http.NewRequest("POST", dsgURL+"/api/v1/create_token", nil)
        req.Header.Set("Authorization", "Bearer "+token)
        resp, err := http.DefaultClient.Do(req)
        // ... forward response body and status code to caller
    }
}
```

### 2.6 Remove old auth code

**Delete entirely:**
- `secure/authorize.go` — `FileAuthorize`, `DatastoreAuthorize`,
  `Authorizer` interface (replaced by DSGClient)
- Token blocklist (`TokenBlocklist` struct, `globalTokenBlocklist`,
  `LoadBlockedTokensFromFile`)

**Remove Go dependencies:**
- `github.com/golang-jwt/jwt/v5` (no more JWT generation/validation)
- `github.com/gorilla/sessions` (no more session cookies)
- `github.com/labstack/echo-contrib/session`
- `golang.org/x/oauth2` (DSG handles OAuth)
- `github.com/satori/go.uuid` (OAuth state parameter)

### 2.7 Files summary

| File | Action | Details |
|------|--------|---------|
| `secure/dsg.go` | **NEW** | DSGClient, FetchUser, DatasetLevel, ExtractToken, RequireDatasetAccess, DSGAuthMiddleware, DSGAdminMiddleware |
| `secure/secure.go` | **REWRITE** | EchoSecure for TLS startup + route registration; remove JWT parsing, blocklist, Gorilla sessions |
| `secure/auth.go` | **REWRITE** | DSG login/logout/profile/token handlers; remove Google OAuth, JWT generation |
| `secure/authorize.go` | **DELETE** | FileAuthorize + DatastoreAuthorize + Authorizer interface no longer needed |
| `secure/blocklist_test.go` | **DELETE** | Tests for removed JWT blocklist code |
| `config/config.go` | **MODIFY** | Add DSGUrl, DSGCacheTTL, DatasetMap; remove old auth fields |
| `main.go` | **MODIFY** | Initialize DSGClient; use DSGAuthMiddleware + DSGAdminMiddleware; update route setup |
| `api/custom/custom.go` | **MODIFY** | Add `RequireDatasetAccess(c, req.Dataset, READ)` call |
| `api/custom/arrow.go` | **MODIFY** | Same per-dataset check |
| `api/custom/arrow_test.go` | **MODIFY** | Add DSG user/client to test context |
| `api/raw/cypher/cypher.go` | **MODIFY** | Add per-dataset ADMIN check to execCypher and startTrans |
| `api/npexplorer/npexplorer.go` | **MODIFY** | Add per-dataset READ check to all 11 handlers |
| `go.mod` | **MODIFY** | Remove JWT, gorilla, echo-contrib, go.uuid, oauth2 deps |

---

## Part 3: Permission mapping summary

| neuprint level | DSG permission | `permissions_v2` contains | Middleware/handler check |
|---|---|---|---|
| NOAUTH (0) | — | — | Token valid but no dataset check |
| READ (1) | `view` | `["view"]` | `RequireDatasetAccess(c, ds, READ)` |
| READWRITE (2) | `edit` or `manage` | `["view", "edit"]` or `["view", "manage"]` | `RequireDatasetAccess(c, ds, READWRITE)` |
| ADMIN (3) | `admin` or `user.Admin` | `["view","edit","manage","admin"]` | `DSGAdminMiddleware` or per-dataset admin |

---

## Part 4: Cookie sharing

If neuprint and DSG are on sibling subdomains (e.g.,
`neuprint.janelia.org` and `dsg.janelia.org`), configure
`AUTH_COOKIE_DOMAIN=.janelia.org` in DSG. The `dsg_token` cookie is
then automatically available to neuprint. Browser users who log in
via any DSG-integrated service get seamless access to neuprint.

For API/programmatic access, users pass the dsg_token via
`Authorization: Bearer {token}` header (same as before, just a
different token value).

---

## Verification

### DatasetGateway
```bash
cd ~/GitHub/DatasetGate/dsg
pixi run -e dev pytest core/tests/ -v -k import_neuprint
```

Test the import command with a sample `authorized.json`:
```bash
pixi run -e dev python manage.py import_neuprint_auth \
    sample_authorized.json --datasets hemibrain manc --dry-run
```

### neuprintHTTP
```bash
cd ~/GitHub/neuprintHTTP
go build ./...
go test ./secure/... ./api/...
```

Test with DSG running locally:
```json
{
  "dsg-url": "http://localhost:8000",
  "dsg-cache-ttl": 30,
  "dataset-map": {"hemibrain:v1.2.1": "hemibrain"},
  "disable-auth": false,
  "hostname": "localhost"
}
```
