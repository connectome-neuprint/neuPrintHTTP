package main

import (
        "encoding/json"
        "github.com/janelia-flyem/neuPrintHTTP/storage"
        "fmt"
        "os"
        "io/ioutil"
        "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)


var OAuthConfig *oauth2.Config

type configInfo struct {
    Engine  string `json:"engine"`
    EngineConfig interface{} `json:"engine-config"`
    AuthFile string `json:"auth-file"`
    CertPEM string `json:"ssl-cert,omitempty"`
    KeyPEM string `json:"ssl-key,omitempty"`
    ClientID string `json:"client-id"`
    ClientSecret string `json:"client-secret"`
    CookieSecret string `json:"cookie-secret"`
    JWTSecret string `json:"jwt-secret"`
}

type Config struct {
    Store storage.Store 
    AuthFile string
    CertPEM string
    KeyPEM string
    CookieSecret string
}

func loadConfig(configFile string) (config Config, err error) {
    // open json file
    jsonFile, err := os.Open(configFile)
    if err != nil {
        err = fmt.Errorf("%s cannot be read", configFile)
        return 
    }
    byteData, _ := ioutil.ReadAll(jsonFile) 
    var configRaw configInfo
    json.Unmarshal(byteData, &configRaw)
    
    config.AuthFile = configRaw.AuthFile
    config.CertPEM = configRaw.CertPEM
    config.KeyPEM = configRaw.KeyPEM
    config.CookieSecret = configRaw.CookieSecret
    config.Store, err = storage.ParseConfig(configRaw.Engine, configRaw.EngineConfig)
   
    // setup authentication information
    OAuthConfig = configureOAuthClient(configRaw.ClientID, configRaw.ClientSecret)

    return 
}

func configureOAuthClient(clientID, clientSecret string) *oauth2.Config {
    // TODO add option to set address
    redirectURL := "https://localhost:11000/oauth2callback"
    return &oauth2.Config{
            ClientID:     clientID,
            ClientSecret: clientSecret,
            RedirectURL:  redirectURL,
            Scopes:       []string{"email", "profile"},
            Endpoint:     google.Endpoint,
    }
}

