# neuPrintHTTP


[![GitHub issues](https://img.shields.io/github/issues/connectome-neuprint/neuPrintHTTP.svg)](https://GitHub.com/connectome-neuprint/neuPrintHTTP/issues/)

Implements a connectomics REST interface that leverages the [neuprint](https://github.com/janelia-flyem/neuPrint) data model.  neuPrintHTTP can be run in a user authenticated mode or without any authentication.  Note: that the authenticated mode (which requires more configuration and setup) is needed to use with neuPrintExplorer web application.  The un-authenticated mode is the ideal way to access the neuPrint data programmatically.

## Installation

Go must be installed and GOPATH must be set to a location to store the application.  neuPrintHTTP supports both file-based logging and Apache Kafka.  For details on kafka, see below.  For basic installation:

    % go get github.com/connectome-neuprint/neuPrintHTTP

neuprintHTTP uses a python script to support cell type analysis.  To use this script, install scipy, scikit-learn, and pandas
and make sure to run neuprint HTTP in the top directory where the python script is located. 

### developers

If modifying the source code and updating the swagger inline comments, update the documentation with:

    % go generate

### using Apache Kafka for logging

To use Kafka for logging, one must install librdkafka and build neuprint http with the kafka option.

See installation instructions
for [librdkafka](https://github.com/confluentinc/confluent-kafka-go#installing-librdkafka).

And then:

    % go install -tags kafka


## Running

    % neuprintHTTP -port |PORTNUM| config.json
 
This launches the server at the specified port with the provided configuration file.  A sample 'shell' config file can be found in 'sample_config.json' in this repo and is show below with some markup.   More description of all possible options are available at 'config/config.go'.

```
{
    "engine": "neuPrint-neo4j",
    "engine-config": {
	    "server": "<NEO4-SERVER>:7474", # location of neo4j
	    "user": "neo4j",
	    "password": "<PASSWORD>"
    },
    "datatypes": {  # optional but configuring "skeletons" allows user to access skeletons through the API
	"skeletons" : [ # examples of two different ways to link to skeletons currently, only link one backend to a given dataset
		{
		"instance": "<UNIQUE NAME>", # any unique name
		"engine": "dvidkv", # supports DVID as a back-end
		"engine-config": {
			"dataset": "hemibrain",
			"server": "http://<DVIDADDR>",
			"branch": "<UUID>",
			"instance": "segmentation_skeletons"
		}
		},
		{
		"instance": "<UNIQUE NAME>", # different name
		"engine": "badger", # also supports embedded keyvalue Badger
		"engine-config": {
			"dataset": "hemibrain",
			"location": "<DIRECTORY LOCATION>"
		}
		}
	]
    },
    "disable-auth": true, # to run no auth mode
    "swagger-docs": "<NEUPRINT_HTTP_LOCATION>/swaggerdocs", # contains swagger documentation
    "log-file": "log.json"
}
```


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
