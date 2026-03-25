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
//
// Service and dataset params are only passed when explicitly provided
// (e.g., for TOS acceptance on a specific dataset). The initial login
// should NOT pass service — just authenticate. TOS is per-dataset, not
// per-login.
func dsgLoginHandler(dsgURL, serviceName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		redirectPath := c.QueryParam("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}

		// If the caller supplied a dataset but the redirect path doesn't
		// already contain it, append it so the user lands back on the
		// correct dataset page after TOS acceptance.
		if dataset := c.QueryParam("dataset"); dataset != "" {
			u, err := url.Parse(redirectPath)
			if err == nil && u.Query().Get("dataset") == "" {
				q := u.Query()
				q.Set("dataset", dataset)
				u.RawQuery = q.Encode()
				redirectPath = u.String()
			}
		}

		// Build absolute redirect URL from the incoming request
		scheme := "https"
		if c.Request().TLS == nil {
			scheme = "http"
		}
		absoluteRedirect := scheme + "://" + c.Request().Host + redirectPath

		target := dsgURL + "/api/v1/authorize?redirect=" + url.QueryEscape(absoluteRedirect)

		// Only pass service and dataset when explicitly requested
		// (for TOS acceptance flows on a specific dataset)
		if dataset := c.QueryParam("dataset"); dataset != "" {
			if serviceName != "" {
				target += "&service=" + url.QueryEscape(serviceName)
			}
			target += "&dataset=" + url.QueryEscape(dataset)
		}
		return c.Redirect(http.StatusFound, target)
	}
}

// dsgLogoutHandler redirects to DatasetGateway's logout endpoint.
func dsgLogoutHandler(dsgURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.Redirect(http.StatusFound, dsgURL+"/api/v1/logout")
	}
}

// dsgProfileHandler returns the authenticated user's profile from the
// DSGUserCache already stored in the echo context by DSGAuthMiddleware.
// This is an app-level auth check only. Per-dataset authorization and
// TOS checks are handled by RequireDatasetAccess.
func dsgProfileHandler(c echo.Context) error {
	user := c.Get("dsg_user").(*DSGUserCache)

	// Authenticated users get at least readwrite; admins get admin.
	level := "readwrite"
	if user.Admin {
		level = "admin"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"Email":     user.Email,
		"AuthLevel": level,
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

	user := c.Get("dsg_user").(*DSGUserCache)
	client := c.Get("dsg_client").(*DSGClient)

	level := client.DatasetLevel(user, dataset)
	if level >= READ {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"dataset": dataset,
			"access":  StringFromLevel(level),
		})
	}

	// No access — check if TOS is the reason
	if client.HasMissingTOS(user, dataset) {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"dataset":      dataset,
			"tos_required": true,
			"message":      "Terms of Service acceptance required for this dataset",
		})
	}

	return c.JSON(http.StatusForbidden, map[string]interface{}{
		"dataset": dataset,
		"message": "You do not have access to " + dataset + " dataset",
	})
}

// dsgTokenHandler proxies a token-creation request to DatasetGateway and
// returns the new dsg_token to the caller.
func dsgTokenHandler(dsgURL string) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := ExtractToken(c)

		req, err := http.NewRequest("POST", dsgURL+"/api/v1/create_token", nil)
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
