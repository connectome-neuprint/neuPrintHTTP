package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Engine          string            `json:"engine"`                           // name of backend
	EngineConfig    interface{}       `json:"engine-config"`                    // config for backend
	DataTypes       interface{}       `json:"datatypes,omitempty"`              // contains configuration for different datatypes
	SwaggerDir      string            `json:"swagger-docs"`                     // static webpage
	MainStores      []interface{}     `json:"mainstore-alternatives,omitempty"` // contains configuration for alternative main stores
	KafkaServers    []string          `json:"kafka-servers,omitempty"`          // kafka servers for logging -- must build with kafka flag
	LoggerFile      string            `json:"log-file,omitempty"`               // location for log file
	Timeout         int               `json:"timeout,omitempty"`                // timeout in seconds for neo4j requests (default 60 seconds)
	DisableAuth     bool              `json:"disable-auth,omitempty"`           // set true to disable auth
	Hostname        string            `json:"hostname,omitempty"`               // name of server
	CertPEM         string            `json:"ssl-cert,omitempty"`               // https certificate
	KeyPEM          string            `json:"ssl-key,omitempty"`                // https private key
	StaticDir       string            `json:"static-dir,omitempty"`             // static webpage
	NgDir           string            `json:"ng-dir,omitempty"`                 // directory for neuroglancer layers config
	VimoServer      string            `json:"vimo-server,omitempty"`            // url for the vimo server
	EnableArrow     bool              `json:"enable-arrow,omitempty"`           // enable Arrow format and Flight support
	ArrowFlightPort int               `json:"arrow-flight-port,omitempty"`      // port for Arrow Flight gRPC server
	DSGUrl          string            `json:"dsg-url,omitempty"`                // DatasetGateway base URL
	DSGCacheTTL     int               `json:"dsg-cache-ttl,omitempty"`          // seconds to cache DSG user/cache responses (default 300)
	DatasetMap      map[string]string `json:"dataset-map,omitempty"`            // neuprint DB name → DSG dataset slug
}

// LoadConfig parses json configuration and loads options
func LoadConfig(configFile string) (config Config, err error) {
	// open json file
	jsonFile, err := os.Open(configFile)
	if err != nil {
		err = fmt.Errorf("%s cannot be read", configFile)
		return
	}
	byteData, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(byteData, &config)
	return
}
