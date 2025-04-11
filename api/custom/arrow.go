package custom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/labstack/echo/v4"
)

// debugValue prints details about a value for debugging
func debugValue(val interface{}) string {
	return fmt.Sprintf("Value: %v, Type: %T", val, val)
}

// CypherArrowData holds the Arrow representation of a Neo4j query result
type CypherArrowData struct {
	Schema  *arrow.Schema
	Records []arrow.Record
}

// ConvertCypherToArrow converts Neo4j query results to Arrow format
func ConvertCypherToArrow(result storage.CypherResult, allocator memory.Allocator) (*CypherArrowData, error) {
	if allocator == nil {
		allocator = memory.DefaultAllocator
	}

	// No data to convert
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no data to convert: empty result set")
	}

	// Create schema from column names and inferred types
	fields := make([]arrow.Field, len(result.Columns))
	for i, colName := range result.Columns {
		// Infer type from first row (not ideal but simple)
		var dataType arrow.DataType = arrow.BinaryTypes.String
		if len(result.Data) > 0 {
			val := result.Data[0][i]
			if storage.VerboseNumeric {
				fmt.Printf("Column %s type inference: %s\n", colName, debugValue(val))
			}
			
			// For numeric operations, prefer Int64 when possible
			preferInt64 := true
			
			switch v := val.(type) {
			case int, int64:
				dataType = arrow.PrimitiveTypes.Int64
			case json.Number:
				// Try to parse as int64 first, which is generally preferred for numeric data
				if _, err := v.Int64(); err == nil {
					dataType = arrow.PrimitiveTypes.Int64
				} else {
					dataType = arrow.PrimitiveTypes.Float64
				}
			case float64:
				// If the float64 can be exactly represented as an int64, prefer that type
				if preferInt64 {
					intVal := int64(v)
					if float64(intVal) == v {
						dataType = arrow.PrimitiveTypes.Int64
					} else {
						dataType = arrow.PrimitiveTypes.Float64
					}
				} else {
					dataType = arrow.PrimitiveTypes.Float64
				}
			case bool:
				dataType = arrow.FixedWidthTypes.Boolean
			default:
				dataType = arrow.BinaryTypes.String
			}
		}
		fields[i] = arrow.Field{Name: colName, Type: dataType}
	}
	schema := arrow.NewSchema(fields, nil)

	// Build Arrow record batch
	rowCount := len(result.Data)
	colCount := len(result.Columns)

	// Create builders for each column
	builders := make([]array.Builder, colCount)
	for i, field := range schema.Fields() {
		builders[i] = array.NewBuilder(allocator, field.Type)
		// Make sure to release builders when done
		defer builders[i].Release()
	}

	// Add data to builders
	for _, row := range result.Data {
		for colIdx, val := range row {
			// Convert json.Number to int64 if possible, otherwise preserve as is
			if num, ok := val.(json.Number); ok {
				// Try to convert to int64 first
				if intVal, err := num.Int64(); err == nil {
					if builders[colIdx].Type().ID() == arrow.INT64 {
						builders[colIdx].(*array.Int64Builder).Append(intVal)
					} else {
						builders[colIdx].AppendNull()
					}
				} else {
					// If not a valid int64, try float64
					if floatVal, err := num.Float64(); err == nil {
						// Check if this float can be represented exactly as an int64
						if builders[colIdx].Type().ID() == arrow.INT64 {
							intVal := int64(floatVal)
							if float64(intVal) == floatVal {
								builders[colIdx].(*array.Int64Builder).Append(intVal)
							} else {
								builders[colIdx].AppendNull()
							}
						} else if builders[colIdx].Type().ID() == arrow.FLOAT64 {
							builders[colIdx].(*array.Float64Builder).Append(floatVal)
						} else {
							builders[colIdx].AppendNull()
						}
					} else {
						// If neither, keep as string
						if builders[colIdx].Type().ID() == arrow.STRING {
							builders[colIdx].(*array.StringBuilder).Append(num.String())
						} else {
							builders[colIdx].AppendNull()
						}
					}
				}
				continue
			}

			switch builders[colIdx].(type) {
			case *array.Int64Builder:
				if val == nil {
					builders[colIdx].(*array.Int64Builder).AppendNull()
				} else {
					switch v := val.(type) {
					case int:
						builders[colIdx].(*array.Int64Builder).Append(int64(v))
					case int64:
						builders[colIdx].(*array.Int64Builder).Append(v)
					case float64:
						builders[colIdx].(*array.Int64Builder).Append(int64(v))
					default:
						builders[colIdx].(*array.Int64Builder).AppendNull()
					}
				}
			case *array.Float64Builder:
				if val == nil {
					builders[colIdx].(*array.Float64Builder).AppendNull()
				} else {
					switch v := val.(type) {
					case float64:
						builders[colIdx].(*array.Float64Builder).Append(v)
					case int:
						builders[colIdx].(*array.Float64Builder).Append(float64(v))
					case int64:
						builders[colIdx].(*array.Float64Builder).Append(float64(v))
					default:
						builders[colIdx].(*array.Float64Builder).AppendNull()
					}
				}
			case *array.BooleanBuilder:
				if val == nil {
					builders[colIdx].(*array.BooleanBuilder).AppendNull()
				} else if v, ok := val.(bool); ok {
					builders[colIdx].(*array.BooleanBuilder).Append(v)
				} else {
					builders[colIdx].(*array.BooleanBuilder).AppendNull()
				}
			case *array.StringBuilder:
				if val == nil {
					builders[colIdx].(*array.StringBuilder).AppendNull()
				} else {
					switch v := val.(type) {
					case string:
						builders[colIdx].(*array.StringBuilder).Append(v)
					default:
						builders[colIdx].(*array.StringBuilder).Append(fmt.Sprintf("%v", v))
					}
				}
			default:
				// Handle unexpected types by converting to string
				if val == nil {
					builders[colIdx].AppendNull()
				} else {
					// This is a fallback that might not work for all builder types
					if sb, ok := builders[colIdx].(*array.StringBuilder); ok {
						sb.Append(fmt.Sprintf("%v", val))
					} else {
						return nil, fmt.Errorf("unable to convert value to appropriate Arrow type for column %s", result.Columns[colIdx])
					}
				}
			}
		}
	}

	// Create arrays from builders
	arrays := make([]arrow.Array, colCount)
	for i, builder := range builders {
		arrays[i] = builder.NewArray()
	}

	// Create record from arrays and ensure proper cleanup
	record := array.NewRecord(schema, arrays, int64(rowCount))
	
	// Set up proper release of arrays after record is created
	for _, arr := range arrays {
		defer arr.Release()
	}
	
	// Clone the record to prevent it from being released when arrays are released
	recordClone := record.NewSlice(0, record.NumRows())
	defer record.Release()

	records := []arrow.Record{recordClone}

	return &CypherArrowData{
		Schema:  schema,
		Records: records,
	}, nil
}

// getCustomArrow handles requests for Arrow format
// swagger:operation GET /api/custom/arrow arrow getArrow
//
// Execute Cypher query and return results in Apache Arrow IPC format
//
// Executes the provided Cypher query against the specified dataset and returns
// the results in Apache Arrow IPC stream format. This is useful for efficient
// data transfer and integration with Arrow-based data processing libraries.
//
// ---
// tags:
// - arrow
// parameters:
// - in: "body"
//   name: "body"
//   required: true
//   schema:
//     type: "object"
//     required: ["cypher", "dataset"]
//     properties:
//       dataset:
//         type: "string"
//         description: "dataset name"
//         example: "hemibrain"
//       cypher:
//         type: "string"
//         description: "cypher statement (read only)"
//         example: "MATCH (n) RETURN n limit 1"
//       version:
//         type: "string"
//         description: "specify a neuprint model version for explicit check"
//         example: "0.5.0"
// produces:
// - application/vnd.apache.arrow.stream
// responses:
//   200:
//     description: "successful operation - data in Arrow IPC stream format"
//     schema:
//       $ref: "#/definitions/ArrowResponse"
//   400:
//     description: "bad request - invalid parameters or query"
//     schema:
//       $ref: "#/definitions/ErrorInfo"
//   404:
//     description: "dataset not found"
//     schema:
//       $ref: "#/definitions/ErrorInfo"
// security:
// - Bearer: []
func (ca cypherAPI) getCustomArrow(c echo.Context) error {
	var req customReq
	if err := c.Bind(&req); err != nil {
		errJSON := map[string]string{"error": "request object not formatted correctly: " + err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// Validate request parameters
	if req.Cypher == "" {
		errJSON := map[string]string{"error": "missing required field 'cypher'"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	if req.Dataset == "" {
		errJSON := map[string]string{"error": "missing required field 'dataset'"}
		return c.JSON(http.StatusBadRequest, errJSON)
	}

	// Set cypher for debugging
	c.Set("debug", req.Cypher)

	// Version check if specified
	if req.Version != "" {
		sstore, ok := ca.Store.(storage.SimpleStore)
		if !ok {
			errJSON := map[string]string{"error": "store does not implement SimpleStore interface"}
			return c.JSON(http.StatusInternalServerError, errJSON)
		}
		sversion, err := sstore.GetVersion()
		if err != nil {
			errJSON := map[string]string{"error": "failed to get store version: " + err.Error()}
			return c.JSON(http.StatusInternalServerError, errJSON)
		}
		if !strings.Contains(sversion, req.Version) {
			errJSON := map[string]string{"error": fmt.Sprintf("neo4j data model version incompatible: required '%s', got '%s'", req.Version, sversion)}
			return c.JSON(http.StatusBadRequest, errJSON)
		}
	}

	// Set dataset for logging
	c.Set("dataset", req.Dataset)

	// Get dataset
	cypher, err := ca.Store.GetDataset(req.Dataset)
	if err != nil {
		errJSON := map[string]string{"error": "dataset not found: " + err.Error()}
		return c.JSON(http.StatusNotFound, errJSON)
	}

	// Execute Cypher query
	data, err := cypher.CypherRequest(req.Cypher, true)
	if err != nil {
		errJSON := map[string]string{"error": "cypher query failed: " + err.Error()}
		return c.JSON(http.StatusBadRequest, errJSON)
	}
	
	// Debug the received data
	if storage.Verbose {
		fmt.Printf("data: %v\n", data)
	}
	
	// Additional numeric debugging if enabled
	if storage.VerboseNumeric && len(data.Data) > 0 && len(data.Data[0]) > 0 {
		fmt.Printf("First value: %s\n", debugValue(data.Data[0][0]))
		
		// Add more detailed logging for value debugging
		fmt.Printf("\n=== DETAILED VALUE ANALYSIS ===\n")
		for i, row := range data.Data {
			for j, val := range row {
				fmt.Printf("Row %d, Col %d: %s\n", i, j, debugValue(val))
				
				// If it's a json.Number, let's see what it parses as
				if num, ok := val.(json.Number); ok {
					fmt.Printf("  - As json.Number string: %s\n", num.String())
					
					// Try int64 conversion
					if intVal, err := num.Int64(); err == nil {
						fmt.Printf("  - Converts to int64: %d\n", intVal)
					} else {
						fmt.Printf("  - Does NOT convert to int64: %v\n", err)
					}
					
					// Try float64 conversion
					if floatVal, err := num.Float64(); err == nil {
						fmt.Printf("  - Converts to float64: %f (scientific: %e)\n", floatVal, floatVal)
					} else {
						fmt.Printf("  - Does NOT convert to float64: %v\n", err)
					}
				}
			}
		}
		fmt.Printf("=== END ANALYSIS ===\n\n")
	}

	// Convert to Arrow format
	arrowData, err := ConvertCypherToArrow(data, memory.DefaultAllocator)
	if err != nil {
		errJSON := map[string]string{"error": "error converting to Arrow format: " + err.Error()}
		return c.JSON(http.StatusInternalServerError, errJSON)
	}

	// Set the content type for Arrow IPC stream format
	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.apache.arrow.stream")

	// Create IPC writer for Arrow flight data
	writer := ipc.NewWriter(c.Response().Writer, ipc.WithSchema(arrowData.Schema))
	defer writer.Close()

	// Write each record to the IPC stream
	recordCount := 0
	for _, record := range arrowData.Records {
		if record == nil {
			continue // Skip nil records
		}
		
		if err := writer.Write(record); err != nil {
			errMsg := fmt.Sprintf("error writing Arrow record %d: %v", recordCount, err)
			// We've already started sending response, so we can't send JSON error
			// Log the error and return it
			fmt.Println(errMsg)
			return fmt.Errorf(errMsg)
		}
		
		// Properly release each record after writing
		defer record.Release()
		recordCount++
	}

	return nil
}