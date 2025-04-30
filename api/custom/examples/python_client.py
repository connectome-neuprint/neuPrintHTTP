#!/usr/bin/env python3
"""
Python client examples for neuPrintHTTP Arrow format and Flight support.

This example demonstrates:
1. Using HTTP Arrow IPC format for direct query results
2. Using Arrow Flight for query execution and streaming results

Requirements:
- pip install pyarrow requests pandas
"""

import os
import io
import json
import argparse
import requests
import pyarrow as pa
import pyarrow.flight as flight
import pandas as pd
from typing import Dict, Any


def query_arrow_ipc_stream(server: str, dataset: str, cypher_query: str, jwt_token: str = None) -> pd.DataFrame:
    """
    Execute a query and receive the results as an Arrow IPC stream over HTTP.
    
    Args:
        server: Base URL of the neuPrintHTTP server (e.g., 'http://localhost:11000')
        dataset: Dataset name (e.g., 'hemibrain')
        cypher_query: The Cypher query to execute
        jwt_token: Optional JWT token for authenticated access
        
    Returns:
        A pandas DataFrame containing the query results
    """
    # Prepare the request
    url = f"{server}/api/custom/arrow"
    headers = {"Content-Type": "application/json"}
    
    # Add authorization if token provided
    if jwt_token:
        headers["Authorization"] = f"Bearer {jwt_token}"
    
    data = {
        "cypher": cypher_query,
        "dataset": dataset
    }
    
    # Make the request
    print(f"Sending request to {url}")
    response = requests.post(url, headers=headers, json=data)
    
    if response.status_code != 200:
        raise Exception(f"Query failed with status {response.status_code}: {response.text}")
    
    content_type = response.headers.get('Content-Type', '')
    if 'application/vnd.apache.arrow.stream' not in content_type:
        raise Exception(f"Expected Arrow stream content type but got: {content_type}")
    
    # Process the Arrow IPC stream
    reader = pa.ipc.open_stream(io.BytesIO(response.content))
    table = reader.read_all()
    
    print(f"Received Arrow table with {table.num_rows} rows and {table.num_columns} columns")
    print(f"Schema: {table.schema}")
    
    # Convert to pandas DataFrame
    df = table.to_pandas()
    
    # Handle Neo4j node objects (which are represented as Arrow Maps)
    def convert_mapvalue_to_dict(val):
        """Convert Arrow Map objects to Python dictionaries"""
        if val is not None:
            # Handle list of tuples format (most common in pandas conversion)
            if isinstance(val, list) and all(isinstance(item, tuple) and len(item) == 2 for item in val):
                return {item[0]: item[1] for item in val}
            
            # Handle tuple/list of tuples format that some PyArrow versions use
            if isinstance(val, (list, tuple)) or (hasattr(val, 'tolist') and callable(val.tolist)):
                try:
                    # Convert tolist if it's an array type
                    items = val.tolist() if hasattr(val, 'tolist') else val
                    
                    # If it's a list of key-value tuples, convert to dict
                    if all(isinstance(item, tuple) and len(item) == 2 for item in items):
                        return {item[0]: item[1] for item in items}
                except Exception:
                    pass
            
            # Check if the object has the 'items' method - MapArray in newer PyArrow versions
            if hasattr(val, 'items') and callable(val.items):
                try:
                    return {k.as_py() if hasattr(k, 'as_py') else k: 
                            v.as_py() if hasattr(v, 'as_py') else v 
                            for k, v in val.items()}
                except AttributeError:
                    pass
                    
            # Some versions represent maps differently
            if hasattr(val, 'keys') and callable(val.keys) and hasattr(val, '__getitem__'):
                try:
                    return {k.as_py() if hasattr(k, 'as_py') else k: 
                            val[k].as_py() if hasattr(val[k], 'as_py') else val[k]
                            for k in val.keys()}
                except AttributeError:
                    pass
        return val
    
    # Process each column - convert Map types to Python dictionaries
    for col in df.columns:
        # Apply the conversion function to each value in the column
        df[col] = df[col].map(lambda x: convert_mapvalue_to_dict(x) if x is not None else None)
    
    return df


def query_arrow_flight(host: str, port: int, dataset: str, cypher_query: str, jwt_token: str = None) -> pd.DataFrame:
    """
    Execute a query using the Arrow Flight service.
    
    Args:
        host: Host of the Arrow Flight service (e.g., 'localhost')
        port: Port of the Arrow Flight service (e.g., 11001)
        dataset: Dataset name (e.g., 'hemibrain')
        cypher_query: The Cypher query to execute
        jwt_token: Optional JWT token for authenticated access
        
    Returns:
        A pandas DataFrame containing the query results
    """
    # Set up connection options
    options = []
    if jwt_token:
        # Add JWT token as authorization header
        options.append(("authorization", f"Bearer {jwt_token}"))

    # Connect to the Flight server
    location = f"grpc://{host}:{port}"
    print(f"Connecting to Flight server at {location}")
    client = flight.FlightClient(location)
    
    # Execute the query using DoAction
    action_type = "ExecuteQuery"
    action_body = json.dumps({
        "cypher": cypher_query,
        "dataset": dataset
    }).encode('utf-8')
    
    print(f"Executing Flight action: {action_type}")
    
    # Send the action and get results
    action = flight.Action(action_type, action_body)
    results = list(client.do_action(action))
    if not results:
        raise Exception("No results returned from DoAction")
    
    # Get the flight ID from the result
    flight_id = results[0].body.decode('utf-8')
    print(f"Received flight ID: {flight_id}")
    
    # Create a ticket to retrieve the data
    ticket = flight.Ticket(flight_id.encode('utf-8'))
    
    # Retrieve the data stream
    reader = client.do_get(ticket)
    
    # Read all batches
    print("Retrieving data...")
    table = reader.read_all()
    
    print(f"Received Arrow table with {table.num_rows} rows and {table.num_columns} columns")
    print(f"Schema: {table.schema}")
    
    # Convert to pandas DataFrame
    df = table.to_pandas()
    
    # Handle Neo4j node objects (which are represented as Arrow Maps)
    def convert_mapvalue_to_dict(val):
        """Convert Arrow Map objects to Python dictionaries"""
        if val is not None:
            # Handle list of tuples format (most common in pandas conversion)
            if isinstance(val, list) and all(isinstance(item, tuple) and len(item) == 2 for item in val):
                return {item[0]: item[1] for item in val}
            
            # Handle tuple/list of tuples format that some PyArrow versions use
            if isinstance(val, (list, tuple)) or (hasattr(val, 'tolist') and callable(val.tolist)):
                try:
                    # Convert tolist if it's an array type
                    items = val.tolist() if hasattr(val, 'tolist') else val
                    
                    # If it's a list of key-value tuples, convert to dict
                    if all(isinstance(item, tuple) and len(item) == 2 for item in items):
                        return {item[0]: item[1] for item in items}
                except Exception:
                    pass
            
            # Check if the object has the 'items' method - MapArray in newer PyArrow versions
            if hasattr(val, 'items') and callable(val.items):
                try:
                    return {k.as_py() if hasattr(k, 'as_py') else k: 
                            v.as_py() if hasattr(v, 'as_py') else v 
                            for k, v in val.items()}
                except AttributeError:
                    pass
                    
            # Some versions represent maps differently
            if hasattr(val, 'keys') and callable(val.keys) and hasattr(val, '__getitem__'):
                try:
                    return {k.as_py() if hasattr(k, 'as_py') else k: 
                            val[k].as_py() if hasattr(val[k], 'as_py') else val[k]
                            for k in val.keys()}
                except AttributeError:
                    pass
        return val
    
    # Process each column - convert Map types to Python dictionaries
    for col in df.columns:
        # Apply the conversion function to each value in the column
        df[col] = df[col].map(lambda x: convert_mapvalue_to_dict(x) if x is not None else None)
    
    return df


def http_ipc_example():
    """Example of HTTP Arrow IPC stream usage"""
    # Configuration
    server = "http://localhost:11000"
    dataset = "hemibrain"
    
    # Example Cypher query - modify this to match your dataset
    cypher_query = """
    MATCH (n) 
    RETURN n.type AS type, count(*) AS count 
    ORDER BY count DESC 
    LIMIT 10
    """
    
    try:
        print("\n===== HTTP Arrow IPC Stream Example =====")
        print(f"Query: {cypher_query}")
        
        df = query_arrow_ipc_stream(server, dataset, cypher_query)
        
        print("\nResults as DataFrame:")
        print(df)
        
        # Example data manipulation with pandas
        if not df.empty and len(df.columns) >= 2:
            print("\nExample data analysis:")
            print(f"Total count: {df['count'].sum()}")
            
            # Create bar chart in text mode for console output
            max_width = 40
            max_count = df['count'].max()
            
            print("\nDistribution:")
            for _, row in df.iterrows():
                label = row['type']
                count = row['count']
                bar_width = int((count / max_count) * max_width)
                bar = "█" * bar_width
                print(f"{label[:20]:<20} | {count:>6} | {bar}")
                
        return True
    except Exception as e:
        print(f"HTTP IPC stream error: {e}")
        return False


def flight_example():
    """Example of Arrow Flight usage"""
    # Configuration
    host = "localhost"
    port = 11001
    dataset = "hemibrain"
    
    # Example Cypher query - modify this to match your dataset
    cypher_query = """
    MATCH (n)-[e:ConnectsTo]->(m)
    RETURN n.type AS source, m.type AS target, count(*) AS connections
    ORDER BY connections DESC
    LIMIT 10
    """
    
    try:
        print("\n===== Arrow Flight Example =====")
        print(f"Query: {cypher_query}")
        
        df = query_arrow_flight(host, port, dataset, cypher_query)
        
        print("\nResults as DataFrame:")
        print(df)
        
        # Example data manipulation with pandas
        if not df.empty and len(df.columns) >= 3:
            print("\nExample data analysis:")
            total = df['connections'].sum()
            print(f"Total connections: {total}")
            
            # Calculate percentages
            df['percentage'] = (df['connections'] / total * 100).round(2)
            
            print("\nTop connections by percentage:")
            for _, row in df.iterrows():
                source = row['source']
                target = row['target']
                connections = row['connections']
                percentage = row['percentage']
                print(f"{source} → {target}: {connections} connections ({percentage}%)")
                
        return True
    except Exception as e:
        print(f"Arrow Flight error: {e}")
        return False


def main():
    """Run the examples"""
    parser = argparse.ArgumentParser(description="neuPrintHTTP Arrow Client Examples")
    parser.add_argument('--http', action='store_true', help='Run the HTTP Arrow IPC example')
    parser.add_argument('--flight', action='store_true', help='Run the Arrow Flight example')
    parser.add_argument('--all', action='store_true', help='Run all examples')
    
    args = parser.parse_args()
    
    # Default to all if no specific example is selected
    if not (args.http or args.flight):
        args.all = True
    
    if args.http or args.all:
        http_ipc_example()
        
    if args.flight or args.all:
        flight_example()


if __name__ == "__main__":
    main()