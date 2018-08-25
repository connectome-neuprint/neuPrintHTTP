package dbmeta

// api that engine needs to impelment
type StorageAPI interface {
	GetVersion() (string, error)
	GetDatabase() (string, string, error)
	GetDatasets() ([]string, error)
}
