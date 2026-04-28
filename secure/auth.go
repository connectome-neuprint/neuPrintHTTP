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
// Service and dataset params are passed when a dataset is known so DatasetGateway
// can intercept for service-specific TOS before returning to neuPrint.
func dsgLoginHandler(dsgURL string, client *DSGClient) echo.HandlerFunc {
	return func(c echo.Context) error {
		redirectPath := c.QueryParam("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}

		// If the caller supplied a dataset but the redirect path doesn't
		// already contain it, append it so the user lands back on the
		// correct dataset page after TOS acceptance.
		dataset := c.QueryParam("dataset")
		if dataset != "" {
			redirectPath = addDatasetQueryParam(redirectPath, dataset)
		}

		// Build absolute redirect URL from the incoming request.
		absoluteRedirect := redirectPath
		if u, err := url.Parse(redirectPath); err != nil || !u.IsAbs() {
			absoluteRedirect = requestBaseURL(c) + redirectPath
		}

		target := dsgURL + "/api/v1/authorize?redirect=" + url.QueryEscape(absoluteRedirect)

		if dataset != "" {
			if client.ServiceName != "" {
				target += "&service=" + url.QueryEscape(client.ServiceName)
			}
			target += "&dataset=" + url.QueryEscape(client.DatasetSlug(dataset))
		}
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

func addDatasetQueryParam(rawURL, dataset string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Query().Get("dataset") != "" {
		return rawURL
	}
	q := u.Query()
	q.Set("dataset", dataset)
	u.RawQuery = q.Encode()
	return u.String()
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
	return addDatasetQueryParam(next, dataset)
}

func tosServiceCheckURL(client *DSGClient, dsgDataset, next string) string {
	u, err := url.Parse(client.BaseURL + "/web/tos/service-check/")
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("service", client.ServiceName)
	q.Set("dataset", dsgDataset)
	q.Set("next", next)
	u.RawQuery = q.Encode()
	return u.String()
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
		"ImageURL":  user.PictureURL,
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
	dsgDataset := client.DatasetSlug(dataset)
	if level >= READ {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"access":       true,
			"tos_required": false,
			"dataset":      dataset,
			"dsg_dataset":  dsgDataset,
			"service":      client.ServiceName,
			"level":        StringFromLevel(level),
		})
	}

	// No access — check if TOS is the reason
	if client.HasMissingTOS(user, dataset) {
		next := serviceReturnURL(c, dataset)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"access":       false,
			"dataset":      dataset,
			"dsg_dataset":  dsgDataset,
			"service":      client.ServiceName,
			"tos_required": true,
			"tos_url":      tosServiceCheckURL(client, dsgDataset, next),
			"message":      "Terms of Service acceptance required for this dataset",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"access":       false,
		"dataset":      dataset,
		"dsg_dataset":  dsgDataset,
		"service":      client.ServiceName,
		"tos_required": false,
		"message":      "You do not have access to " + dataset + " dataset",
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
