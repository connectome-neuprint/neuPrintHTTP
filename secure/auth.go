package secure

import (
	"io"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v4"
)

// dsgLoginHandler redirects the user to DatasetGateway's OAuth entry point.
// It passes service and dataset params so DSG can present any pending TOS.
func dsgLoginHandler(dsgURL, serviceName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		redirect := c.QueryParam("redirect")
		if redirect == "" {
			redirect = "/"
		}
		target := dsgURL + "/api/v1/authorize?redirect=" + url.QueryEscape(redirect)
		if serviceName != "" {
			target += "&service=" + url.QueryEscape(serviceName)
		}
		if dataset := c.QueryParam("dataset"); dataset != "" {
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
func dsgProfileHandler(c echo.Context) error {
	user := c.Get("dsg_user").(*DSGUserCache)
	level, _ := c.Get("level").(string)
	if level == "" {
		level = "noauth"
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"Email":      user.Email,
		"AuthLevel":  level,
		"TOSRequired": len(user.MissingTOS) > 0,
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
