module github.com/connectome-neuprint/neuPrintHTTP

go 1.23.0

toolchain go1.24.1

// Note: This go version is required by github.com/apache/arrow-go/v18
// To use a lower Go version, you would need to use a lower version of Arrow

require (
	github.com/apache/arrow-go/v18 v18.1.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/dgraph-io/badger/v3 v3.2103.2
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/gorilla/sessions v1.2.1
	github.com/knightjdr/hclust v1.0.2
	github.com/labstack/echo-contrib v0.12.0
	github.com/labstack/echo/v4 v4.9.0
	github.com/labstack/gommon v0.3.1
	github.com/neo4j/neo4j-go-driver/v5 v5.27.0
	github.com/satori/go.uuid v1.2.0
	github.com/valyala/fasttemplate v1.2.1
	golang.org/x/crypto v0.35.0
	golang.org/x/net v0.36.0
	golang.org/x/oauth2 v0.23.0
	google.golang.org/grpc v1.69.2
	gopkg.in/confluentinc/confluent-kafka-go.v1 v1.8.2
)

require (
	cloud.google.com/go/compute v1.6.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/confluentinc/confluent-kafka-go v1.8.2 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/goccy/go-json v0.10.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang/glog v1.2.4 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v24.12.23+incompatible // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/klauspost/cpuid/v2 v2.2.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/exp v0.0.0-20240909161429-701f63a606c0 // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	golang.org/x/time v0.0.0-20220411224347-583f2d630306 // indirect
	golang.org/x/tools v0.29.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	google.golang.org/genproto v0.0.0-20220421151946-72621c1f0bd3 // indirect
	google.golang.org/protobuf v1.36.1 // indirect
)
