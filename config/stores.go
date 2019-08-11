package config

// loads all storage plugins
import (
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	_ "github.com/connectome-neuprint/neuPrintHTTP/storage/badger"
	_ "github.com/connectome-neuprint/neuPrintHTTP/storage/dvid"
	_ "github.com/connectome-neuprint/neuPrintHTTP/storage/neuprintneo4j"
)

// CreateStore creates a datastore from the engine specified by the configuration
func CreateStore(config Config) (storage.Store, error) {
	return storage.ParseConfig(config.Engine, config.EngineConfig, config.DataTypes)
}
