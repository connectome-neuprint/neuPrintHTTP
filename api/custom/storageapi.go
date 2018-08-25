package custom

// api that engine needs to impelment
type StorageAPI interface {
	CustomRequest(map[string]interface{}) (interface{}, error)
}
