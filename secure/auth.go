package secure

import (
	"io"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

// dsgLoginHandler redirects the user to DatasetGateway's OAuth entry point.
// The redirect param from the caller is a path; we build an absolute URL
// so DSG knows where to send the user back.
func dsgLoginHandler(dsgURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		redirectPath := c.QueryParam("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}

		// Build absolute redirect URL from the incoming request.
		absoluteRedirect := redirectPath
		if u, err := url.Parse(redirectPath); err != nil || !u.IsAbs() {
			absoluteRedirect = requestBaseURL(c) + redirectPath
		}

		target := dsgURL + "/api/v1/authorize?redirect=" + url.QueryEscape(absoluteRedirect)
		return c.Redirect(http.StatusFound, target)
	}
}

func requestBaseURL(c echo.Context) string {
	scheme := "https"
	if c.Request().TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + c.Request().Host
}

func serviceReturnURL(c echo.Context, dataset string) string {
	next := c.QueryParam("next")
	if next == "" {
		next = c.Request().Header.Get("Referer")
	}
	if next == "" {
		next = "/"
	}
	u, err := url.Parse(next)
	if err == nil && !u.IsAbs() {
		next = requestBaseURL(c) + next
	}
	u, err = url.Parse(next)
	if err != nil || u.Query().Get("dataset") != "" {
		return next
	}
	q := u.Query()
	q.Set("dataset", dataset)
	u.RawQuery = q.Encode()
	return u.String()
}

// dsgLogoutHandler redirects to DatasetGateway's logout endpoint.
func dsgLogoutHandler(dsgURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.Redirect(http.StatusFound, dsgURL+"/api/v1/logout")
	}
}

// dsgProfileHandler returns the authenticated identity profile already stored
// in the echo context by DSGAuthMiddleware.
// This is an app-level auth check only. Per-dataset authorization and
// TOS checks are handled by RequireDatasetAccess.
func dsgProfileHandler(c echo.Context) error {
	identity := c.Get("dsg_identity").(*DSGIdentity)

	// Authenticated users get at least readwrite; admins get admin.
	level := "readwrite"
	if identity.Admin {
		level = "admin"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"Email":     identity.Email,
		"AuthLevel": level,
		"ImageURL":  identity.PictureURL,
	})
}

// dsgDatasetAccessHandler checks whether the authenticated user can access
// a specific dataset, returning TOS status if applicable. The frontend
// calls this when the user selects a dataset from the dropdown.
func dsgDatasetAccessHandler(c echo.Context) error {
	dataset := c.QueryParam("dataset")
	if dataset == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "dataset parameter required")
	}

	identity := c.Get("dsg_identity").(*DSGIdentity)
	client := c.Get("dsg_client").(*DSGClient)

	if identity.Admin {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"access":       true,
			"tos_required": false,
			"dataset":      dataset,
			"service":      client.ServiceName,
			"level":        StringFromLevel(ADMIN),
		})
	}

	token, _ := c.Get("dsg_token").(string)
	if token == "" {
		token = ExtractToken(c)
	}
	if token == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	decision, err := client.DatasetDecision(token, dataset, serviceReturnURL(c, dataset), true)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "auth service unavailable")
	}
	level := decision.Level()
	if level >= READ {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"access":       true,
			"tos_required": false,
			"dataset":      dataset,
			"service":      client.ServiceName,
			"level":        StringFromLevel(level),
		})
	}

	if decision.TOSRequired() {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"access":       false,
			"dataset":      dataset,
			"service":      client.ServiceName,
			"level":        StringFromLevel(level),
			"tos_required": true,
			"tos_url":      decision.TOSURL,
			"message":      "Terms of Service acceptance required for this dataset",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"access":       false,
		"dataset":      dataset,
		"service":      client.ServiceName,
		"level":        StringFromLevel(level),
		"tos_required": false,
		"message":      "You do not have access to " + dataset + " dataset",
	})
}

// dsgTokenHandler proxies a token request to DatasetGateway's
// long_lived_token endpoint and returns the user's stable bearer token to
// the caller. DSG's long_lived_token is idempotent: it returns the same
// token row on every call, so the displayed token is safe to paste into
// neuprint-python configs and stays valid across browser refreshes.
func dsgTokenHandler(dsgURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := ExtractToken(c)

		req, err := http.NewRequest("GET", dsgURL+"/api/v1/long_lived_token", nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to build token request")
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, "token service unreachable")
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		return c.JSONBlob(resp.StatusCode, body)
	}
}
