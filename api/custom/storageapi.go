package custom

// StorageAPI specifies the interface that backend  engine needs to satisfy
type StorageAPI interface {
	CustomRequest(map[string]interface{}) (interface{}, error)
}
