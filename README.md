# neuPrintHTTP


[![GitHub issues](https://img.shields.io/github/issues/connectome-neuprint/neuPrintHTTP.svg)](https://GitHub.com/connectome-neuprint/neuPrintHTTP/issues/)

Implements a connectomics REST interface that leverages the [neuprint](https://github.com/janelia-flyem/neuPrint) data model.

## Installation

Go must be installed and GOPATH must be set to a location to store the spplication.

By default, neuprint http builds logging support using kafka.  See installation instructions
for [librdkafka](https://github.com/confluentinc/confluent-kafka-go#installing-librdkafka).

After installing librdfkafka, install neuprint http:

    % go get github.com/connectome-neuprint/neuPrintHTTP

For developers: if modifying the swagger inline comments, update the documentation with:

    % go generate

## Running

    % neuprintHTTP -p |PORTNUM| config.json
 
The config file should contain information on the backend datastore that satisfies the connectomics REST API and the location for a file containing
a list of authorized users.  To test https locally and generate the necessary certificates, run:

    % go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost

## Installing without kafka support

If you are having trouble building the server, because librdkafka is missing and you don't need to send log messages to a kafka server, then try this build.

    %  go get -tags nokafka github.com/connectome-neuprint/neuPrintHTTP
