package dbmeta

// StorageAPI specifies the interface that backend  engine needs to satisfy
type StorageAPI interface {
	GetVersion() (string, error)
	GetDatabase() (string, string, error)
	GetDatasets() ([]string, error)
}
