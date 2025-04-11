package storage

var (
	availEngines map[string]Engine
)

// RegisterEngine associates a given storage backend with a name
func RegisterEngine(e Engine) {
	if availEngines == nil {
		availEngines = map[string]Engine{e.GetName(): e}
	} else {
		availEngines[e.GetName()] = e
	}
}

// Engine is the backend database that implements connectomics API
type Engine interface {
	GetName() string
	NewStore(interface{}, string, string) (SimpleStore, error)
}
