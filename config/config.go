package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Config struct {
	Engine        string        `json:"engine"`                           // name of backend
	EngineConfig  interface{}   `json:"engine-config"`                    // config for backend
	DataTypes     interface{}   `json:"datatypes,omitempty"`              // contains configuration for different datatypes
	SwaggerDir    string        `json:"swagger-docs"`                     // static webpage
	MainStores    []interface{} `json:"mainstore-alternatives,omitempty"` // contains configuration for alternative main stores
	KafkaServers  []string      `json:"kafka-servers,omitempty"`          // kafka servers for logging -- must build with kafka flag
	LoggerFile    string        `json:"log-file,omitempty"`               // location for log file
	Timeout       int           `json:"timeout,omitempty"`                // timeout in seconds for neo4j requeests (default 60 seconds)
	DisableAuth   bool          `json:"disable-auth,omitempty"`           // set true to disable auth (can ignore flags below)
	ClientID      string        `json:"oauthclient-id,omitempty"`         // google oauth client id
	ClientSecret  string        `json:"oauthclient-secret,omitempty"`     // google oauth client secret
	Secret        string        `json:"appsecret,omitempty"`              // password for token and cookie generation
	Hostname      string        `json:"hostname,omitempty"`               // name of server
	AuthFile      string        `json:"auth-file,omitempty"`              // json authorization file
	CertPEM       string        `json:"ssl-cert,omitempty"`               // https certificate
	KeyPEM        string        `json:"ssl-key,omitempty"`                // https private key
	AuthToken     string        `json:"auth-token,omitempty"`             // token for authorization service
	AuthDatastore string        `json:"auth-datastore,omitempty"`         // location of authorization service
	StaticDir     string        `json:"static-dir,omitempty"`             // static webpage
	NgDir         string        `json:"ng-dir,omitempty"`                 // directory for neuroglancer layers config
	ProxyAuth     string        `json:"proxy-auth,omitempty"`             // remote proxy for authentication
	ProxyInsecure bool          `json:"proxy-insecure,omitempty"`         // if true, disable https secure check
	VimoServer    string        `json:"vimo-server,omitempty"`            // url for the vimo server
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
