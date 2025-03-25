package custom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
)

// FlightService implements the Arrow Flight service for neuPrintHTTP
type FlightService struct {
	Port  int
	Store storage.Store
}

// neuPrintFlightServer implements the FlightServiceServer interface
type neuPrintFlightServer struct {
	flight.BaseFlightServer
	store storage.Store
	allocator memory.Allocator
}

// Start begins the Arrow Flight service
func (fs *FlightService) Start() error {
	// Set up the server location
	address := fmt.Sprintf("localhost:%d", fs.Port)
	
	// Create a Flight server
	flightServer := flight.NewFlightServer()
	err := flightServer.Init(address)
	if err != nil {
		return fmt.Errorf("failed to initialize Flight server on %s: %v", address, err)
	}
	
	// Initialize our server implementation
	svc := &neuPrintFlightServer{
		store: fs.Store,
		allocator: memory.DefaultAllocator,
	}
	
	// Register the service
	flightServer.RegisterFlightService(svc)
	
	// Start the server in a goroutine
	go func() {
		fmt.Printf("Starting Arrow Flight service on %s\n", address)
		if err := flightServer.Serve(); err != nil {
			fmt.Printf("Arrow Flight server error: %v\n", err)
		}
	}()
	
	return nil
}

// DoAction handles Flight actions like executing queries
func (s *neuPrintFlightServer) DoAction(action *flight.Action, stream flight.FlightService_DoActionServer) error {
	switch action.Type {
	case "ExecuteQuery":
		// Parse the request
		var req struct {
			Cypher  string `json:"cypher"`
			Dataset string `json:"dataset"`
			Version string `json:"version,omitempty"`
		}

		if err := json.Unmarshal(action.Body, &req); err != nil {
			return fmt.Errorf("invalid request format: %v", err)
		}

		// Generate a unique ticket ID for this query
		ticketID := fmt.Sprintf("query-%s-%s", req.Dataset, req.Cypher)

		// Send the ticket ID back to the client
		result := flight.Result{Body: []byte(ticketID)}
		if err := stream.Send(&result); err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("unknown action: %s", action.Type)
}

// GetFlightInfo returns info about a particular flight
func (s *neuPrintFlightServer) GetFlightInfo(ctx context.Context, descriptor *flight.FlightDescriptor) (*flight.FlightInfo, error) {
	// Create flight info with appropriate fields for v18
	info := &flight.FlightInfo{
		Schema:       []byte{},
		FlightDescriptor: descriptor,
		Endpoint: []*flight.FlightEndpoint{
			{
				Ticket: &flight.Ticket{Ticket: descriptor.Cmd},
				Location: []*flight.Location{{
					Uri: "localhost",
				}},
			},
		},
		TotalRecords: -1,
		TotalBytes:   -1,
	}
	return info, nil
}

// DoGet retrieves a dataset as specified by a ticket
func (s *neuPrintFlightServer) DoGet(ticket *flight.Ticket, stream flight.FlightService_DoGetServer) error {
	// Parse the ticket ID which should contain query details
	ticketID := string(ticket.Ticket)

	// This is where you would parse the ticket to extract dataset and query
	// For now, we'll just return an empty result
	schema := arrow.NewSchema([]arrow.Field{
		{Name: "message", Type: arrow.BinaryTypes.String},
	}, nil)

	// Send schema
	if err := stream.Send(&flight.FlightData{
		DataHeader: flight.SerializeSchema(schema, s.allocator),
	}); err != nil {
		return err
	}

	// In a real implementation, you would execute the query here
	// and stream the results as batches
	fmt.Printf("DoGet called with ticket: %s\n", ticketID)

	return nil
}

// ListFlights implements the Arrow Flight ListFlights method
// Note the signature has changed in v18 to use Criteria instead of FlightDescriptor
func (s *neuPrintFlightServer) ListFlights(criteria *flight.Criteria, stream flight.FlightService_ListFlightsServer) error {
	// For now we'll just return a simple unimplemented message
	return fmt.Errorf("ListFlights not implemented")
}