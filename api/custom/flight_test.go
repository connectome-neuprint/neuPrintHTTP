package custom

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"google.golang.org/grpc/metadata"
)

// These are direct service tests rather than loopback-network tests. That
// keeps the Flight contract testable in restricted build sandboxes where
// binding even an ephemeral localhost port is forbidden.
type fakeServerStream struct{}

func (fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (fakeServerStream) SetTrailer(metadata.MD)       {}
func (fakeServerStream) Context() context.Context     { return context.Background() }
func (fakeServerStream) SendMsg(interface{}) error    { return nil }
func (fakeServerStream) RecvMsg(interface{}) error    { return io.EOF }

type fakeDoActionStream struct {
	fakeServerStream
	results []*flight.Result
}

func (s *fakeDoActionStream) Send(result *flight.Result) error {
	s.results = append(s.results, result)
	return nil
}

type fakeDoGetStream struct {
	fakeServerStream
	data []*flight.FlightData
}

func (s *fakeDoGetStream) Send(data *flight.FlightData) error {
	s.data = append(s.data, data)
	return nil
}

func testFlightService() *neuPrintFlightServer {
	return &neuPrintFlightServer{
		store:     &mockStoreImpl{cypherStore: MockCypher{}},
		allocator: memory.DefaultAllocator,
	}
}

func TestFlightDoAction(t *testing.T) {
	query := "MATCH (n) RETURN n.id LIMIT 10"
	dataset := "test"
	reqBody, _ := json.Marshal(map[string]string{"cypher": query, "dataset": dataset})
	stream := &fakeDoActionStream{}

	err := testFlightService().DoAction(&flight.Action{Type: "ExecuteQuery", Body: reqBody}, stream)
	if err != nil {
		t.Fatalf("DoAction failed: %v", err)
	}
	if len(stream.results) != 1 {
		t.Fatalf("results=%d, want 1", len(stream.results))
	}
	ticketID := string(stream.results[0].Body)
	expectedPrefix := "query-" + dataset
	if len(ticketID) <= len(expectedPrefix) || ticketID[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("expected ticket ID to start with %q, got %q", expectedPrefix, ticketID)
	}
}

func TestFlightGetFlightInfo(t *testing.T) {
	descriptor := &flight.FlightDescriptor{Type: 2, Cmd: []byte("MATCH (n) RETURN n.id LIMIT 10")}
	info, err := testFlightService().GetFlightInfo(context.Background(), descriptor)
	if err != nil {
		t.Fatalf("GetFlightInfo failed: %v", err)
	}
	if info == nil || len(info.Endpoint) == 0 {
		t.Fatalf("unexpected FlightInfo: %v", info)
	}
}

func TestFlightDoGet(t *testing.T) {
	stream := &fakeDoGetStream{}
	ticket := &flight.Ticket{Ticket: []byte("query-test-MATCH (n) RETURN n.id LIMIT 10")}
	if err := testFlightService().DoGet(ticket, stream); err != nil {
		t.Fatalf("DoGet failed: %v", err)
	}
	if len(stream.data) != 1 || len(stream.data[0].DataHeader) == 0 {
		t.Fatalf("expected one schema message, got %+v", stream.data)
	}
}
