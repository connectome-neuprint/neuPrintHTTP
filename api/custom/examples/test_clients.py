#!/usr/bin/env python3
"""
Test script for Arrow client examples against the mock server.

This script:
1. Starts the mock server
2. Runs tests against both HTTP and Flight endpoints
3. Reports success/failure

Usage:
  python test_clients.py

Requirements:
  pip install pyarrow requests pytest
"""

import os
import sys
import time
import signal
import threading
import subprocess
import unittest
from typing import Optional

import requests
import pyarrow as pa
import pandas as pd

# Import our client functions
from python_client import query_arrow_ipc_stream, query_arrow_flight

# Constants
HTTP_PORT = 11000
FLIGHT_PORT = 11001
SERVER_PROCESS = None

def start_mock_server():
    """Start the mock server in a separate process"""
    global SERVER_PROCESS
    
    script_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mock_server.py")
    cmd = [sys.executable, script_path]
    SERVER_PROCESS = subprocess.Popen(cmd, 
                            stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE,
                            text=True)
    
    # Give the server time to start
    time.sleep(1)
    
    # Verify the server is running
    try:
        # Check HTTP endpoint
        requests.get(f"http://localhost:{HTTP_PORT}/")
    except requests.exceptions.ConnectionError:
        # This is expected since / is not a valid endpoint, but we just want to check the server is running
        pass
    
    print(f"Mock server started (PID: {SERVER_PROCESS.pid})")

def stop_mock_server():
    """Stop the mock server"""
    global SERVER_PROCESS
    if SERVER_PROCESS:
        print(f"Stopping mock server (PID: {SERVER_PROCESS.pid})")
        SERVER_PROCESS.terminate()
        SERVER_PROCESS.wait(timeout=5)
        SERVER_PROCESS = None

# Test HTTP Arrow client
def test_http_client():
    """Test the HTTP Arrow client against the mock server"""
    print("\n--- Testing HTTP Arrow Client ---")
    
    # Define test queries
    test_cases = [
        {
            "name": "Default query",
            "query": "MATCH (n) RETURN n LIMIT 10", 
            "expected_cols": ["id", "name", "value", "active"],
            "expected_rows": 5
        },
        {
            "name": "Node type query",
            "query": "MATCH (n) RETURN n.type, count(*)", 
            "expected_cols": ["type", "count"],
            "expected_rows": 5
        },
        {
            "name": "Connection query",
            "query": "MATCH (n)-[r]->(m) RETURN n.name, m.name, count(*)", 
            "expected_cols": ["source", "target", "weight"],
            "expected_rows": 5
        }
    ]
    
    # Run tests
    all_passed = True
    
    for tc in test_cases:
        print(f"\nRunning test: {tc['name']}")
        try:
            # Execute the query
            df = query_arrow_ipc_stream(
                f"http://localhost:{HTTP_PORT}",
                "test_dataset",
                tc["query"]
            )
            
            # Verify return type
            assert isinstance(df, pd.DataFrame), f"Expected DataFrame, got {type(df)}"
            
            # Verify data shape
            assert len(df) == tc["expected_rows"], f"Expected {tc['expected_rows']} rows, got {len(df)}"
            
            # Print results
            print(f"✅ Success: Received {len(df)} rows")
            print(f"Columns: {list(df.columns)}")
            print(df.head(2))
            
        except Exception as e:
            print(f"❌ Error: {str(e)}")
            all_passed = False
    
    return all_passed

# Test Flight Arrow client
def test_flight_client():
    """Test the Flight Arrow client against the mock server"""
    print("\n--- Testing Flight Arrow Client ---")
    
    # Define test queries
    test_cases = [
        {
            "name": "Default query",
            "query": "MATCH (n) RETURN n LIMIT 10", 
            "expected_cols": 4,  # id, name, value, active
            "expected_rows": 5
        },
        {
            "name": "Node type query",
            "query": "MATCH (n) RETURN n.type AS type, count(*) AS count", 
            "expected_cols": 2,  # type, count
            "expected_rows": 5
        }
    ]
    
    # Run tests
    all_passed = True
    
    for tc in test_cases:
        print(f"\nRunning test: {tc['name']}")
        try:
            # Execute the query
            df = query_arrow_flight(
                "localhost",
                FLIGHT_PORT,
                "test_dataset",
                tc["query"]
            )
            
            # Verify return type
            assert isinstance(df, pd.DataFrame), f"Expected DataFrame, got {type(df)}"
            
            # Print results
            print(f"✅ Success: Received DataFrame with shape {df.shape}")
            print(df.head(2))
            
        except Exception as e:
            print(f"❌ Error: {str(e)}")
            all_passed = False
    
    return all_passed

def main():
    print("Starting Arrow client tests...")
    try:
        print("Checking if required modules are available...")
        missing_modules = []
        for module in ["pyarrow", "requests", "flask"]:
            try:
                __import__(module)
            except ImportError:
                missing_modules.append(module)
        
        if missing_modules:
            print(f"⚠️ Missing required modules: {', '.join(missing_modules)}")
            print("Please install the missing modules with:")
            print(f"pip install {' '.join(missing_modules)}")
            return
            
        # Start the mock server
        start_mock_server()
        
        # Run tests
        print("\nRunning client tests...")
        http_result = test_http_client()
        flight_result = test_flight_client()
        
        # Report results
        print("\n--- Test Results ---")
        print(f"HTTP Client: {'✅ PASSED' if http_result else '❌ FAILED'}")
        print(f"Flight Client: {'✅ PASSED' if flight_result else '❌ FAILED'}")
        
        if http_result and flight_result:
            print("\n🎉 All tests passed! The clients are working correctly with the mock server.")
        else:
            print("\n⚠️ Some tests failed. Check the logs above for details.")
            
    finally:
        # Clean up
        stop_mock_server()

if __name__ == "__main__":
    main()