package custom

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// MockCypher is reused from arrow_test.go
// It provides a simple implementation of the Cypher interface for testing

// mockFlightServer creates a local Flight server for testing
func setupMockFlightServer(t *testing.T) (string, func()) {
	// Create a mock store
	mockCypherStore := MockCypher{}
	mockStore := &mockStoreImpl{
		cypherStore: mockCypherStore,
	}

	// We don't actually use the FlightService from our implementation
	// because we're directly setting up the test server

	// Start the server
	server := flight.NewFlightServer()
	err := server.Init("localhost:0") // Use port 0 to automatically select an available port
	if err != nil {
		t.Fatalf("Failed to initialize flight server: %v", err)
	}

	// Initialize the server implementation
	svc := &neuPrintFlightServer{
		store:     mockStore,
		allocator: memory.DefaultAllocator,
	}

	// Register the service
	server.RegisterFlightService(svc)

	// Start the server
	go server.Serve()

	// Get the server address
	addr := server.Addr().String()

	// Return the server address and a cleanup function
	return addr, func() {
		server.Shutdown()
	}
}

// Test DoAction functionality
func TestFlightDoAction(t *testing.T) {
	// Set up the flight server
	addr, cleanup := setupMockFlightServer(t)
	defer cleanup()

	// Create a client
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := flight.NewFlightServiceClient(conn)

	// Create a test request
	query := "MATCH (n) RETURN n.id LIMIT 10"
	dataset := "test"
	reqBody, _ := json.Marshal(map[string]string{
		"cypher":  query,
		"dataset": dataset,
	})
	actionRequest := &flight.Action{
		Type: "ExecuteQuery",
		Body: reqBody,
	}

	// Call DoAction
	stream, err := client.DoAction(context.Background(), actionRequest)
	if err != nil {
		t.Fatalf("DoAction failed: %v", err)
	}

	// Read the response
	result, err := stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive action result: %v", err)
	}

	// Verify the response contains a ticket ID
	ticketID := string(result.Body)
	expectedPrefix := "query-" + dataset
	if len(ticketID) <= len(expectedPrefix) || ticketID[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected ticket ID to start with %q, got %q", expectedPrefix, ticketID)
	}
}

// Test GetFlightInfo functionality
func TestFlightGetFlightInfo(t *testing.T) {
	// Set up the flight server
	addr, cleanup := setupMockFlightServer(t)
	defer cleanup()

	// Create a client
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := flight.NewFlightServiceClient(conn)

	// Create a test descriptor
	descriptor := &flight.FlightDescriptor{
		Type: 2, // CMD type
		Cmd:  []byte("MATCH (n) RETURN n.id LIMIT 10"),
	}

	// Call GetFlightInfo
	info, err := client.GetFlightInfo(context.Background(), descriptor)
	if err != nil {
		t.Fatalf("GetFlightInfo failed: %v", err)
	}

	// Verify the response
	if info == nil {
		t.Fatalf("Expected FlightInfo, got nil")
	}

	if len(info.Endpoint) == 0 {
		t.Errorf("Expected endpoints, got none")
	}
}

// Test DoGet functionality
func TestFlightDoGet(t *testing.T) {
	// Set up the flight server
	addr, cleanup := setupMockFlightServer(t)
	defer cleanup()

	// Create a client
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := flight.NewFlightServiceClient(conn)

	// Create a test ticket
	ticket := &flight.Ticket{
		Ticket: []byte("query-test-MATCH (n) RETURN n.id LIMIT 10"),
	}

	// Call DoGet
	stream, err := client.DoGet(context.Background(), ticket)
	if err != nil {
		t.Fatalf("DoGet failed: %v", err)
	}

	// Read the schema
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("Failed to receive schema message: %v", err)
	}

	// Verify we got a schema
	if len(msg.DataHeader) == 0 {
		t.Errorf("Expected schema header, got empty data")
	}

	// We're just checking if we got a response with data
	// The exact schema validation depends on the implementation details
}