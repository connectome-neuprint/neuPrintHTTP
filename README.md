# neuprintHTTP

Implements a connectomics REST interface that leverages the [neuprint](github.com/janelia-flyem/neuprint) data model.

## Installation

    % go get github.com/janelia-flyem/neuprintHTTP

## Running

    % neuprintHTTP -p |PORTNUM| config.json
 
The config file should contain information on the backend datastore that satisfies the connectomics REST API and the location for a file containing
a list of authorized users.
