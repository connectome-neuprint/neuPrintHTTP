This package was formerly separate and is not integrated for simplicity.

# echo-secure
Go library that uses Google oAuth2 authentication and custom authorization for the echo framework.

This library has the following features:

* Configures https for echo web framework.  It uses Let's Encrypt by default but allows manual specification of SSL certificate and key.
* REST endpoints for /login and /logout, as well as /profile which returns the email and profile image of the authenticated user (uses Google oauth2).  Authenticated state is saved in a cookie.
* Provides simple functionality to manage user authorization using a JSON file or an [http endpoint](https://github.com/janelia-flyem/appdata-store) that uses google datastore.  The file or datastore can be modified without requiring server restart.
* Provides middleware for authentication and authorization of echo REST endpoints.
* REST endpoint for /token that produces a JWT token for authentication.

## Usage

This library can be used without any configuration, which by default configures the echo framework to use https through Let's Encrypt without requiring
authentication or authorization.  To use authentication, Google oauth must be configured to recognize the web app.  You will need a client id and client secret.

Example:

    import  "github.com/janelia-flyem/echo-secure"
  
    // create echo object
    e := echo.New()
  
    // create JSON file authorize (file with JSON dictionary
    // with keys of "emailaddress" with values of "read", "readwrite", or "admin"
    authorizer, err := secure.NewFileAuthorizer(myJSONAuthFileName)

    // configuration for echo secure object
    sconfig := secure.SecureConfig{
                SSLCert:          certPEMFile,
                SSLKey:           keyPEMFile,
                ClientID:         googlecClientID,
                ClientSecret:     googleClientSecret,
                AuthorizeChecker: authorizer,
                Hostname:         myhostname,
    }
  
    // configures echo web framework and creates wrapper for starting web service
    secureAPI, err := secure.InitializeEchoSecure(e, sconfig, []byte("my secret password"))

    // add authentication and authorization rules for all endpoints starting with "/api"
    readGrp := e.Group("/api")
    readGrp.Use(secureAPI.AuthMiddleware(secure.READ))

    // start echo web framework at a given port
    secureAPI.StartEchoSecure(port)

## TODO

* Add unit tests
