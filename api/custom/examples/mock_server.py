#!/usr/bin/env python3
"""
Mock server for testing Arrow HTTP and Flight clients.

This mock server implements:
1. HTTP endpoint that returns Arrow IPC stream data
2. Arrow Flight service with basic query functionality

Usage:
  python mock_server.py [--http-port PORT] [--flight-port PORT]

Requirements:
  pip install pyarrow flask waitress
"""

import os
import io
import sys
import json
import time
import argparse
import threading
from typing import Dict, Any, List, Optional

import pyarrow as pa
import pyarrow.flight as flight
from flask import Flask, request, Response, jsonify

# Define sample data that will be returned for any query
SAMPLE_DATA = {
    "node_types": [
        {"type": "Neuron", "count": 25000},
        {"type": "Synapse", "count": 125000},
        {"type": "PreSyn", "count": 62000},
        {"type": "PostSyn", "count": 63000},
        {"type": "SynapticConnection", "count": 1500000},
    ],
    "connections": [
        {"source": "Neuron1", "target": "Neuron2", "weight": 250},
        {"source": "Neuron1", "target": "Neuron3", "weight": 150},
        {"source": "Neuron2", "target": "Neuron4", "weight": 100},
        {"source": "Neuron3", "target": "Neuron5", "weight": 300},
        {"source": "Neuron4", "target": "Neuron5", "weight": 200},
    ]
}

def create_sample_arrow_table(query_type: str = "default") -> pa.Table:
    """
    Create a sample Arrow table based on the query type.
    
    Args:
        query_type: Type of query to simulate ('nodes', 'connections', or 'default')
        
    Returns:
        Arrow Table with sample data
    """
    if "type" in query_type.lower() or "node" in query_type.lower():
        # Return node type counts
        data = SAMPLE_DATA["node_types"]
        return pa.Table.from_pylist(
            data,
            schema=pa.schema([
                pa.field("type", pa.string()),
                pa.field("count", pa.int64())
            ])
        )
    elif "connect" in query_type.lower() or "edge" in query_type.lower():
        # Return connection data
        data = SAMPLE_DATA["connections"]
        return pa.Table.from_pylist(
            data,
            schema=pa.schema([
                pa.field("source", pa.string()),
                pa.field("target", pa.string()),
                pa.field("weight", pa.int64())
            ])
        )
    else:
        # Default data with mixed types
        return pa.Table.from_pylist([
            {"id": 1, "name": "item1", "value": 10.5, "active": True},
            {"id": 2, "name": "item2", "value": 20.1, "active": False},
            {"id": 3, "name": "item3", "value": 30.2, "active": True},
            {"id": 4, "name": "item4", "value": 40.7, "active": False},
            {"id": 5, "name": "item5", "value": 50.3, "active": True},
        ])

# ---------------- HTTP Arrow Server ----------------

app = Flask(__name__)

@app.route('/api/custom/arrow', methods=['POST'])
def arrow_endpoint():
    """Handle Arrow IPC requests over HTTP"""
    try:
        # Parse the request
        request_data = request.get_json()
        if not request_data:
            return jsonify({"error": "Invalid JSON request"}), 400
        
        cypher_query = request_data.get('cypher', '')
        dataset = request_data.get('dataset', 'default')
        
        print(f"[HTTP] Received query: {cypher_query}")
        print(f"[HTTP] Dataset: {dataset}")
        
        # Create a sample table based on the query
        table = create_sample_arrow_table(cypher_query)
        
        # Add a delay to simulate processing
        time.sleep(0.1)
        
        # Serialize to Arrow IPC format
        try:
            sink = pa.BufferOutputStream()
            writer = pa.ipc.new_stream(sink, table.schema)
            writer.write_table(table)
            writer.close()
            buf = sink.getvalue()
        except Exception as e:
            print(f"[HTTP] Error serializing Arrow data: {e}")
            return jsonify({"error": f"Error serializing Arrow data: {str(e)}"}), 500
        
        # Return the Arrow IPC stream
        response = Response(buf.to_pybytes())
        response.headers['Content-Type'] = 'application/vnd.apache.arrow.stream'
        return response
        
    except Exception as e:
        print(f"[HTTP] Error: {str(e)}")
        return jsonify({"error": str(e)}), 500

def start_http_server(port: int = 11000):
    """Start the HTTP server in a separate thread"""
    from waitress import serve
    print(f"[HTTP] Starting mock HTTP Arrow server on port {port}")
    serve(app, host='0.0.0.0', port=port)

# ---------------- Flight Server ----------------

class MockFlightServer(flight.FlightServerBase):
    def __init__(self, location="localhost", port=11001):
        """Initialize the Flight server"""
        super(MockFlightServer, self).__init__(location, port)
        self.flights = {}
        print(f"[Flight] Server initialized at grpc://{location}:{port}")
        
    def do_action(self, context, action):
        """
        Handle action requests like executing queries.
        """
        print(f"[Flight] Received action: {action.type}")
        if action.type == "ExecuteQuery":
            try:
                # Parse the query request
                request_json = json.loads(action.body.decode())
                cypher_query = request_json.get('cypher', '')
                dataset = request_json.get('dataset', 'default')
                
                print(f"[Flight] Query: {cypher_query}")
                print(f"[Flight] Dataset: {dataset}")
                
                # Generate a flight ID
                flight_id = f"query-{dataset}-{hash(cypher_query) % 10000}"
                
                # Store the query information
                self.flights[flight_id] = {
                    "query": cypher_query,
                    "dataset": dataset,
                    "timestamp": time.time()
                }
                
                # Return the flight ID
                yield flight.Result(flight_id.encode())
                
            except Exception as e:
                print(f"[Flight] Action error: {str(e)}")
                raise flight.FlightServerError(f"Failed to execute query: {str(e)}")
        else:
            raise flight.FlightServerError(f"Unknown action: {action.type}")
    
    def get_flight_info(self, context, descriptor):
        """Get information about a flight."""
        print(f"[Flight] GetFlightInfo request: {descriptor}")
        
        # For simplicity, we'll create a ticket from the descriptor
        ticket_bytes = descriptor.cmd
        location = flight.Location.for_grpc_tcp("localhost", self.port)
        endpoints = [flight.FlightEndpoint(ticket_bytes, [location])]
        
        # Create a dummy schema
        schema = pa.schema([
            pa.field("id", pa.int64()),
            pa.field("name", pa.string()),
        ])
        
        # Return flight info
        return flight.FlightInfo(schema,
                                descriptor,
                                endpoints,
                                -1,  # unknown size
                                -1)  # unknown records
    
    def do_get(self, context, ticket):
        """
        Return data for a ticket.
        """
        ticket_id = ticket.ticket.decode()
        print(f"[Flight] DoGet request with ticket: {ticket_id}")
        
        # Check if this is a known flight
        flight_info = self.flights.get(ticket_id)
        query_type = "default"
        
        if flight_info:
            query_type = flight_info["query"]
            
        # Create sample table
        table = create_sample_arrow_table(query_type)
        
        # Add a delay to simulate processing
        time.sleep(0.2)
        
        # Write the schema
        yield flight.RecordBatchStream(table)
    
    def list_flights(self, context, criteria):
        """List available flights."""
        print(f"[Flight] ListFlights request: {criteria}")
        for flight_id, info in self.flights.items():
            # Create a descriptor for the flight
            descriptor = flight.FlightDescriptor.for_command(flight_id.encode())
            
            # Create a dummy schema
            schema = pa.schema([pa.field("name", pa.string())])
            
            # Create endpoint
            location = flight.Location.for_grpc_tcp("localhost", self.port)
            endpoints = [flight.FlightEndpoint(flight_id.encode(), [location])]
            
            # Yield flight info
            yield flight.FlightInfo(schema, descriptor, endpoints, -1, -1)

def start_flight_server(port: int = 11001):
    """Start the Flight server"""
    location = "0.0.0.0"
    server = MockFlightServer(location, port)
    print(f"[Flight] Starting mock Flight server on port {port}")
    server.serve()

# ---------------- Main ----------------

def main():
    parser = argparse.ArgumentParser(description='Mock Arrow HTTP and Flight Server')
    parser.add_argument('--http-port', type=int, default=11000, help='HTTP server port')
    parser.add_argument('--flight-port', type=int, default=11001, help='Flight server port')
    parser.add_argument('--http-only', action='store_true', help='Start only the HTTP server')
    parser.add_argument('--flight-only', action='store_true', help='Start only the Flight server')
    
    args = parser.parse_args()
    
    start_http = not args.flight_only
    start_flight = not args.http_only
    
    try:
        threads = []
        
        if start_http:
            http_thread = threading.Thread(target=start_http_server, args=(args.http_port,), daemon=True)
            http_thread.start()
            threads.append(http_thread)
            print(f"HTTP Arrow server running at http://localhost:{args.http_port}/api/custom/arrow")
        
        if start_flight:
            flight_thread = threading.Thread(target=start_flight_server, args=(args.flight_port,), daemon=True)
            flight_thread.start()
            threads.append(flight_thread)
            print(f"Arrow Flight server running at grpc://localhost:{args.flight_port}")
        
        # Keep the main thread running
        for t in threads:
            t.join()
            
    except KeyboardInterrupt:
        print("\nServer shutdown requested. Exiting...")
    except Exception as e:
        print(f"Error: {str(e)}")

if __name__ == "__main__":
    main()