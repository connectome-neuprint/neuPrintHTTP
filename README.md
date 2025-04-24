# neuPrintHTTP


[![GitHub issues](https://img.shields.io/github/issues/connectome-neuprint/neuPrintHTTP.svg)](https://GitHub.com/connectome-neuprint/neuPrintHTTP/issues/)

Implements a connectomics REST interface that leverages the [neuprint](https://github.com/janelia-flyem/neuPrint) data model.  neuPrintHTTP can be run in a user authenticated mode or without any authentication.  Note: that the authenticated mode (which requires more configuration and setup) is needed to use with neuPrintExplorer web application.  The un-authenticated mode is the ideal way to access the neuPrint data programmatically.

## Dependencies
Since neuPrint is written in [golang](https://golang.org), you will need to [download](https://golang.org/dl) and install golang before you can build and run neuPrintHTTP. The build tools for golang are opinionated about the file structure and location of golang projects, but by default the tools will autogenerate the required folders when you `go get` a project.

## Installing

Go must be installed (version 1.16+). neuPrintHTTP supports both file-based logging and Apache Kafka. For basic installation:

### Option 1: Clone and build (recommended)

```bash
# Clone the repository
git clone https://github.com/connectome-neuprint/neuPrintHTTP.git
cd neuPrintHTTP

# Build the application
go build

# Or install it to your GOPATH's bin directory
go install
```

### Option 2: Direct install (requires Go modules)

```bash
# Install the latest version
go install github.com/connectome-neuprint/neuPrintHTTP@latest
```

To run tests:

    % go test ./...
    
To test a specific package:

    % go test ./api/...

neuprintHTTP uses a python script to support cell type analysis.  To use this script, install scipy, scikit-learn, and pandas
and make sure to run neuprint HTTP in the top directory where the python script is located.

## Data Access Endpoints

### Standard JSON Endpoint

The default endpoint for custom queries is `/api/custom/custom`, which returns results in JSON format:

```bash
curl -X POST "http://localhost:11000/api/custom/custom" \
  -H "Content-Type: application/json" \
  -d '{"cypher": "MATCH (n) RETURN n LIMIT 1", "dataset": "hemibrain"}'
```

The response will be JSON with this structure:
```json
{
  "columns": ["name", "size"],
  "data": [["t4", 323131], ["mi1", 232323]]
}
```

Where:
- `columns`: Array of column names from your query
- `data`: Array of rows, each row containing values that correspond to the columns

### Apache Arrow Support

neuPrintHTTP supports returning query results in Apache Arrow format via the `/api/custom/arrow` HTTP endpoint. This provides several advantages:

- Efficient binary serialization with low overhead
- Preservation of data types
- Native integration with data science tools
- Optimized memory layout for analytical workloads

neuPrintHTTP uses Arrow v18 for all Arrow-related functionality, including both the HTTP IPC stream format and the preliminary Flight implementation.

#### Using the Arrow Endpoint

To retrieve data in Arrow format, send a POST request to `/api/custom/arrow` with the same JSON body format as the regular custom endpoint:

```bash
curl -X POST "http://localhost:11000/api/custom/arrow" \
  -H "Content-Type: application/json" \
  -d '{"cypher": "MATCH (n) RETURN n LIMIT 1", "dataset": "hemibrain"}' \
  --output data.arrow
```

The response will be in Arrow IPC stream format with content type `application/vnd.apache.arrow.stream`. This is a standard way to transfer Arrow data over HTTP without requiring gRPC or Arrow Flight.

You can parse this with Arrow libraries available in multiple languages:

```python
# Python example - Standard HTTP with Arrow IPC format (No Flight required)
import os
import pyarrow as pa
import requests

# Get token from environment variable. Token can be found in neuPrintExplorer settings.
# Only necessary if authentication is turned on.
token = os.environ.get("NEUPRINT_APPLICATION_CREDENTIALS")

# Add the token to the headers
headers = {
    "Content-Type": "application/json",
    "Authorization": f"Bearer {token}"
}

resp = requests.post('http://localhost:11000/api/custom/arrow', 
                    headers=headers,
                    json={"cypher": "MATCH (n) RETURN n LIMIT 1", 
                          "dataset": "hemibrain"})

# Parse the Arrow IPC stream from the HTTP response
reader = pa.ipc.open_stream(pa.py_buffer(resp.content))
table = reader.read_all()
print(table)
```

```javascript
// JavaScript example with Arrow JS
const response = await fetch('http://localhost:11000/api/custom/arrow', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    cypher: "MATCH (n) RETURN n LIMIT 1",
    dataset: "hemibrain"
  })
});

// Get the binary data
const arrayBuffer = await response.arrayBuffer();
// Parse the Arrow IPC stream
const table = await arrow.tableFromIPC(arrayBuffer);
console.log(table.toString());
```

### developers

If modifying the source code and updating the swagger inline comments, update the documentation with:

    % go generate

### using Apache Kafka for logging

To use Kafka for logging, one must install librdkafka and build neuprint http with the kafka option.

See installation instructions
for [librdkafka](https://github.com/confluentinc/confluent-kafka-go#installing-librdkafka).

And then:

    % go install -tags kafka


## Installing without kafka support

If you are having trouble building the server, because librdkafka is missing and you don't need to send log messages to a kafka server, then try this build.

    %  go get -tags nokafka github.com/connectome-neuprint/neuPrintHTTP

## Running

    % neuPrintHTTP -port |PORTNUM| config.json
 
The config file should contain information on the backend datastore that satisfies the connectomics REST API and the location for a file containing
a list of authorized users.  To test https locally and generate the necessary certificates, run:

    % go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost

### Command Line Options

```bash
Usage: neuprintHTTP [OPTIONS] CONFIG.json
  -port int
        port to start server (default 11000)
  -arrow-flight-port int
        port for Arrow Flight gRPC server (default 11001)
  -disable-arrow
        disable Arrow format support (enabled by default)
  -public_read
        allow all users read access
  -proxy-port int
        proxy port to start server
  -pid-file string
        file for pid
  -verbose
        verbose mode
```

### Configuration

The server is configured using a JSON file. The configuration specifies database connections, authentication options, and other server settings.

#### Apache Arrow Configuration

The Arrow support in neuPrintHTTP includes:

1. **Arrow IPC HTTP endpoint**: Available at `/api/custom/arrow` on the main HTTP port
2. **Arrow Flight gRPC server**: Runs on a separate port (default: 11001)

To change the Arrow Flight port:

```bash
# Start with custom Flight port
neuprintHTTP -arrow-flight-port 12345 config.json
```

To disable Arrow support entirely:

```bash
# Disable all Arrow functionality
neuprintHTTP -disable-arrow config.json
```

#### Sample Configuration

A sample configuration file can be found in `config-examples/config.json` in this repo:

```json
{
    "engine": "neuPrint-bolt",
    "engine-config": {
        "server": "<NEO4-SERVER>:7687", 
        "user": "neo4j",
        "password": "<PASSWORD>"
    },
    "datatypes": {
        "skeletons": [
            {
                "instance": "<UNIQUE NAME>",
                "engine": "dvidkv",
                "engine-config": {
                    "dataset": "hemibrain",
                    "server": "http://<DVIDADDR>",
                    "branch": "<UUID>",
                    "instance": "segmentation_skeletons"
                }
            }
        ]
    },
    "disable-auth": true,
    "swagger-docs": "<NEUPRINT_HTTP_LOCATION>/swaggerdocs",
    "log-file": "log.json"
}
```

Note that the Bolt (optimized neo4j protocol) engine `neupPrint-bolt` is recommended while the 
older `neuPrint-neo4j` engine is deprecated. See below.

#### Neo4j Bolt Driver

neuPrintHTTP now supports the Neo4j Bolt protocol driver, which provides better performance and more accurate handling of large integer values (greater than 53 bits). To use the Bolt driver:

```json
{
    "engine": "neuPrint-bolt",
    "engine-config": {
        "server": "bolt://localhost:7687", 
        "user": "neo4j",
        "password": "password",
        "database": "neo4j"  // Optional: database name for Neo4j 4.0+ (omit for Neo4j 3.x)
    },
    "timeout": 600
}
```

The Bolt driver correctly preserves large integer values (including integers above 2^53) that would be truncated to floating-point by the HTTP JSON API. This is particularly important for precise integer operations on large IDs and counts.

For more detailed configuration options, refer to `config/config.go`.


### No Auth Mode

This is the easiest way to use neuprint http.  It launches an http server and does not require user authorization.  To use this, just set "disable-auth" to true as above.

### Auth mode

There are several options required to use authorization and authentication with Google.  Notably, the user must register
the application with Google to enable using google authentication.
Also, for authoriation one can either specify user information in a static json file (example in this repo)
or data can be extracted from Google's cloud datastore with a bit more configuration.  See more documentation in config/config.go.

If you're using Google Datastore to manage the list of authorized users,
you can use the Google Cloud Console or the Python API. (See below.)


One must also provide https credentials.  To get certificates for local testing, run and add the produced files into the config file.

    % go run $GOROOT/src/crypto/tls/generate_cert.go --host localhost

#### Update authorized users list with Google Cloud Console

For adding or removing a single user, it's most convenient to just use the [Google Cloud Console][gcp-console].

[gcp-console]: https://console.cloud.google.com/datastore/stats?project=dvid-em

1. Start on the "Dashboard" page
2. Click `neuprint_janelia`
3. Click "Query Entities"
4. Click `name=users`
5. Add or delete properties (one per user)
6. Click the "Save" button at the bottom of the screen.

#### Update authorized users list with Python

If you're using Google Datastore to manage the list of authorized users,
it's convenient to programmatically edit the list with the [Google Datastore Python API][datastore-api].

[datastore-api]: https://googleapis.dev/python/datastore/latest/index.html

Start by installing the `google-cloud-datastore` Python package.
Also make sure you've got the correct Google Cloud Project selected
(or configure `GOOGLE_APPLICATION_CREDENTIALS`).

```
conda install -c conda-forge google-cloud-datastore
gcloud config set project dvid-em
```

Here's an example:

```python
from google.cloud.datastore import Client, Key, Entity

client = Client()

# Fetch the list of users from the appropriate access list
key = client.key('neuprint_janelia', 'users')
r = client.query(kind='neuprint_janelia', ancestor=key).fetch(1000)

# Extract the "entity", which is dict-like
entity = list(r)[0]

# Remove a user
del entity['baduser@gmail.com']

# Add some new users
new_users = {
    'newuser1@gmail.com': 'readwrite',
    'newuser2@gmail.com': 'readwrite'
}
entity.update(new_users)

# Upload
client.put(entity)
```
