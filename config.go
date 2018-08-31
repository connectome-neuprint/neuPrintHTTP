package main

import (
	"encoding/json"
	"fmt"
	"github.com/janelia-flyem/neuPrintHTTP/storage"
	"io/ioutil"
	"os"
)

type configInfo struct {
	Engine        string      `json:"engine"`
	EngineConfig  interface{} `json:"engine-config"`
	AuthFile      string      `json:"auth-file"`
	CertPEM       string      `json:"ssl-cert,omitempty"`
	KeyPEM        string      `json:"ssl-key,omitempty"`
	ClientID      string      `json:"client-id"`
	ClientSecret  string      `json:"client-secret"`
	Secret        string      `json:"secret"`
	Hostname      string      `json:"hostname"`
	AuthToken     string      `json:"auth-token"`
	AuthDatastore string      `json:"auth-datastore"`
	StaticDir     string      `json:"static-dir"`
}

type Config struct {
	Store         storage.Store
	AuthFile      string
	CertPEM       string
	KeyPEM        string
	Secret        string
	Hostname      string
	ClientID      string
	ClientSecret  string
	AuthToken     string
	AuthDatastore string
	StaticDir     string
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

	// TODO create store and load config separately

	config.AuthFile = configRaw.AuthFile
	config.AuthDatastore = configRaw.AuthDatastore
	config.StaticDir = configRaw.StaticDir
	config.AuthToken = configRaw.AuthToken
	config.CertPEM = configRaw.CertPEM
	config.KeyPEM = configRaw.KeyPEM
	config.Secret = configRaw.Secret
	config.Hostname = configRaw.Hostname
	config.Store, err = storage.ParseConfig(configRaw.Engine, configRaw.EngineConfig)
	config.ClientID = configRaw.ClientID
	config.ClientSecret = configRaw.ClientSecret

	return
}
