package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Config struct {
	Engine        string      `json:"engine"`                   // name of backend
	EngineConfig  interface{} `json:"engine-config"`            // config for backend
	ClientID      string      `json:"oauthclient-id"`           // google oauth client id
	ClientSecret  string      `json:"oauthclient-secret"`       // google oauth client secret
	Secret        string      `json:"appsecret"`                // password for token and cookie generation
	Hostname      string      `json:"hostname"`                 // name of server
	SwaggerDir    string      `json:"swagger-docs"`             // static webpage
	AuthFile      string      `json:"auth-file,omitempty"`      // json authorization file
	CertPEM       string      `json:"ssl-cert,omitempty"`       // https certificate
	KeyPEM        string      `json:"ssl-key,omitempty"`        // https private key
	AuthToken     string      `json:"auth-token,omitempty"`     // token for authorization service
	AuthDatastore string      `json:"auth-datastore,omitempty"` // location of authorization service
	StaticDir     string      `json:"static-dir,omitempty"`     // static webpage
	LoggerFile    string      `json:"log-file,omitempty"`       // location for log file
}

// LoadConfig parses json configuration and loads options
func LoadConfig(configFile string) (config Config, err error) {
	// open json file
	jsonFile, err := os.Open(configFile)
	if err != nil {
		err = fmt.Errorf("%s cannot be read", configFile)
		return
	}
	byteData, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteData, &config)
	return
}
