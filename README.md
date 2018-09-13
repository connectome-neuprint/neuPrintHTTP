# neuPrintHTTP

Implements a connectomics REST interface that leverages the [neuprint](https://github.com/janelia-flyem/neuPrint) data model.

## Installation

    % go get github.com/janelia-flyem/neuPrintHTTP

## Running

    % neuprintHTTP -p |PORTNUM| config.json
 
The config file should contain information on the backend datastore that satisfies the connectomics REST API and the location for a file containing
a list of authorized users.  To test https locally and generate the necessary certificates, run:

    % go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost
