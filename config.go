package main

import (
        "encoding/json"
        "github.com/janelia-flyem/neuprintHTTP/storage"
        "fmt"
        "os"
        "io/ioutil"
)


type configInfo struct {
    Engine  string `json:"engine"`
    EngineConfig interface{} `json:"engine-config"`
    AuthFile string `json:"auth-file"`
}

type Config struct {
    Store storage.Store 
    AuthFile string
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
    config.Store, err = storage.ParseConfig(configRaw.Engine, configRaw.EngineConfig)
    return 
}

