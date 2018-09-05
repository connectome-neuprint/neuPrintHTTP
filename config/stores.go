package config

// loads all storage plugins
import (
	"github.com/janelia-flyem/neuPrintHTTP/storage"
	_ "github.com/janelia-flyem/neuPrintHTTP/storage/neuprintneo4j"
)

// LoadStore creates a datastore from the engine specified by the configuration
func CreateStore(config Config) (storage.Store, error) {
	return storage.ParseConfig(config.Engine, config.EngineConfig)
}
