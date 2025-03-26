# Apache Arrow Client Examples

This directory contains examples of how to use the Apache Arrow integrations in neuPrintHTTP. These examples demonstrate both the HTTP IPC Stream and Arrow Flight Protocol interfaces, providing efficient data exchange with neuPrintHTTP.

## Why Apache Arrow?

Apache Arrow provides significant advantages over traditional JSON responses:

- **Efficient Binary Format**: Arrow's columnar memory format is 15-20x more compact than JSON for numeric data
- **Zero-Copy Reading**: Client libraries can read Arrow data without deserializing
- **Type Preservation**: Native numeric/boolean/binary types are preserved (no string conversion as in JSON)
- **Direct Integration**: Arrow data can be directly used with data science tools like pandas, DataFusion, and more

## Included Examples

This directory contains client examples in multiple languages:

1. **Python**: 
   - `python_client.py` - Complete examples for both HTTP Arrow and Flight Protocol
   - Includes data visualization and pandas integration

2. **JavaScript**:
   - `javascript_client.js` - Example for HTTP Arrow using the arrow-js library
   - Works in both Node.js and browser environments

## HTTP Arrow IPC Stream Endpoint

The HTTP Arrow endpoint is fully functional at `/api/custom/arrow`. This is the simplest way to get started with Arrow in neuPrintHTTP.

### Basic Usage

1. Send a POST request to `/api/custom/arrow` with a JSON body containing:
   ```json
   {
     "cypher": "YOUR_CYPHER_QUERY",
     "dataset": "YOUR_DATASET"
   }
   ```

2. The response will be in Arrow IPC stream format with content type `application/vnd.apache.arrow.stream`.

3. Parse the response using an Arrow IPC reader (available in [many languages](https://arrow.apache.org/docs/status.html)).

### Python Example

```python
import requests
import pyarrow as pa
import io

# Make HTTP request
response = requests.post(
    "http://localhost:11000/api/custom/arrow",
    json={"cypher": "MATCH (n) RETURN n LIMIT 10", "dataset": "hemibrain"}
)

# Parse the Arrow IPC stream
reader = pa.ipc.open_stream(io.BytesIO(response.content))
table = reader.read_all()

# Convert to pandas if needed
df = table.to_pandas()
print(df)
```

### JavaScript Example

```javascript
import { tableFromIPC } from 'apache-arrow';

// Make HTTP request
const response = await fetch('http://localhost:11000/api/custom/arrow', {
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({
    cypher: "MATCH (n) RETURN n LIMIT 10",
    dataset: "hemibrain"
  })
});

// Parse the Arrow IPC stream
const arrayBuffer = await response.arrayBuffer();
const table = await tableFromIPC(arrayBuffer);
console.log(table.toString());
```

## Arrow Flight Protocol

The Arrow Flight Protocol is a higher-performance gRPC-based protocol for exchanging Arrow data. neuPrintHTTP exposes a Flight service on a separate port (default: 11001).

### Basic Usage

1. Connect to the Flight service using an Arrow Flight client library
2. Execute a query using the `ExecuteQuery` action with a JSON payload:
   ```json
   {
     "cypher": "YOUR_CYPHER_QUERY",
     "dataset": "YOUR_DATASET"
   }
   ```
3. Get the flight ticket from the action result
4. Use the ticket to fetch the data with `DoGet`

### Python Example

```python
import pyarrow.flight as flight
import json

# Connect to the Flight server
client = flight.FlightClient("grpc://localhost:11001")

# Create the query action
query = "MATCH (n) RETURN n LIMIT 10"
action = flight.Action(
    "ExecuteQuery", 
    json.dumps({"cypher": query, "dataset": "hemibrain"}).encode()
)

# Execute the action to get a ticket
results = list(client.do_action(action))
ticket_id = results[0].body.decode('utf-8')

# Use the ticket to retrieve data
ticket = flight.Ticket(ticket_id.encode())
reader = client.do_get(ticket)
table = reader.read_all()

# Convert to pandas
df = table.to_pandas()
print(df)
```

## Running the Examples

### Python

Requirements:
- Python 3.8+

Install the required packages using conda (recommended):

```bash
# Primary packages with Flight RPC support
conda install -c conda-forge pyarrow libarrow-flight

# Other dependencies 
conda install -c conda-forge requests flask waitress pandas
```

Then run the examples:

```bash
python python_client.py --http    # Run HTTP example only
python python_client.py --flight  # Run Flight example only
python python_client.py --all     # Run both examples
```

### JavaScript

Requirements:
- Node.js 14+
- Apache Arrow JS library

```bash
npm install apache-arrow
node --experimental-modules javascript_client.js
```

## Testing with Mock Server

This directory includes a mock server that simulates both the Arrow HTTP endpoint and the Flight service without requiring an actual Neo4j database. This is useful for:

- Testing client code when Neo4j isn't available
- Development without a full neuPrintHTTP server
- Integration testing in CI pipelines

### Running the Mock Server

The mock server provides both HTTP and Flight interfaces on the same ports as the real server:

```bash
# Start both HTTP and Flight servers
python mock_server.py

# Start only the HTTP server
python mock_server.py --http-only

# Start only the Flight server
python mock_server.py --flight-only

# Use custom ports
python mock_server.py --http-port 12000 --flight-port 12001
```

The mock server will return sample data regardless of the query sent. It will interpret the query text to determine what type of data to return:

- Queries containing "type" or "node" will return node type statistics
- Queries containing "connect" or "edge" will return connection data
- Other queries will return a default mixed-type dataset

### Running Tests Against the Mock Server

The included `test_clients.py` script verifies that the clients work correctly with the mock server.

First, make sure you have the required dependencies installed (same as above):

```bash
# Using conda (recommended)
conda install -c conda-forge pyarrow libarrow-flight requests flask waitress pandas
```

Then run the tests:

```bash
python test_clients.py
```

This will:
1. Start the mock server
2. Test both HTTP and Flight clients against it
3. Verify the results match expected formats
4. Report success or failure

## Authentication Support

Both examples support JWT-based authentication. You can pass a JWT token to the example clients:

```python
# HTTP IPC Stream with authentication
df = query_arrow_ipc_stream(
    "http://localhost:11000", 
    "hemibrain", 
    "MATCH (n) RETURN n LIMIT 10",
    jwt_token="your.jwt.token"
)

# Flight with authentication 
df = query_arrow_flight(
    "localhost", 
    11001, 
    "hemibrain", 
    "MATCH (n) RETURN n LIMIT 10",
    jwt_token="your.jwt.token"
)
```