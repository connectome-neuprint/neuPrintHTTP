package storage

import "errors"

type Point struct {
	X int
	Y int
	Z int
}
type Compression int
type Scale int

type DataInstance struct {
	Instance string      `json:"instance"`
	Engine   string      `json:"engine"`
	Config   interface{} `json:"engine-config"`
}

var ErrKeyNotFound = errors.New("Key not found")
