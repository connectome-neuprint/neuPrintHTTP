package main

import (
        "encoding/json"
        "github.com/janelia-flyem/neuPrintHTTP/storage"
        "fmt"
        "os"
        "io/ioutil"
)


type configInfo struct {
    Engine  string `json:"engine"`
    EngineConfig interface{} `json:"engine-config"`
    AuthFile string `json:"auth-file"`
    CertPEM string `json:"ssl-cert,omitempty"`
    KeyPEM string `json:"ssl-key,omitempty"`
}

type Config struct {
    Store storage.Store 
    AuthFile string
    CertPEM string
    KeyPEM string
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
    config.Store, err = storage.ParseConfig(configRaw.Engine, configRaw.EngineConfig)
    return 
}

