package neuprintbolt

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/connectome-neuprint/neuPrintHTTP/storage"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Transaction implements the storage.CypherTransaction interface
// for the Neo4j Bolt protocol
type Transaction struct {
	ctx        context.Context
	driver     neo4j.DriverWithContext
	session    neo4j.SessionWithContext
	tx         neo4j.ExplicitTransaction
	isExplicit bool
	database   string // The Neo4j database name (for Neo4j 4.0+)
}

// CypherRequest executes a Cypher query in the transaction
func (t *Transaction) CypherRequest(cypher string, readonly bool) (storage.CypherResult, error) {
	var result storage.CypherResult
	result.Debug = cypher

	// Create a session if one doesn't exist
	if t.session == nil {
		var accessMode neo4j.AccessMode
		if readonly {
			accessMode = neo4j.AccessModeRead
		} else {
			accessMode = neo4j.AccessModeWrite
		}

		config := neo4j.SessionConfig{
			AccessMode: accessMode,
		}

		// Add database name if specified
		if t.database != "" {
			config.DatabaseName = t.database
		}

		t.session = t.driver.NewSession(t.ctx, config)
	}

	// For explicit transactions
	if t.isExplicit && t.tx == nil {
		var err error
		if readonly {
			t.tx, err = t.session.BeginTransaction(t.ctx)
		} else {
			t.tx, err = t.session.BeginTransaction(t.ctx)
		}
		if err != nil {
			return result, fmt.Errorf("failed to begin transaction: %w", err)
		}
	}

	// Execute the query based on whether we have an explicit transaction or not
	var records []*neo4j.Record
	var keys []string

	if t.isExplicit && t.tx != nil {
		// Run in explicit transaction
		res, err := t.tx.Run(t.ctx, cypher, nil)
		if err != nil {
			return result, fmt.Errorf("failed to execute query: %w", err)
		}

		records, err = res.Collect(t.ctx)
		if err != nil {
			return result, fmt.Errorf("failed to collect results: %w", err)
		}
		keys, err = res.Keys()
		if err != nil {
			return result, fmt.Errorf("failed to get keys: %w", err)
		}
	} else {
		// Run in auto-commit transaction
		// Use database name if provided, otherwise use default database
		var runResult *neo4j.EagerResult
		var err error

		if t.database != "" {
			runResult, err = neo4j.ExecuteQuery(
				t.ctx,
				t.driver,
				cypher,
				nil,
				neo4j.EagerResultTransformer,
				neo4j.ExecuteQueryWithDatabase(t.database),
			)
		} else {
			runResult, err = neo4j.ExecuteQuery(
				t.ctx,
				t.driver,
				cypher,
				nil,
				neo4j.EagerResultTransformer,
			)
		}
		if err != nil {
			return result, fmt.Errorf("failed to execute query: %w", err)
		}
		records = runResult.Records
		keys = runResult.Keys
	}

	// Process the results
	result.Columns = keys
	result.Data = make([][]interface{}, len(records))

	// Convert Neo4j records to our format
	for i, record := range records {
		values := make([]interface{}, len(keys))
		for j, key := range keys {
			rawValue, found := record.Get(key)
			if found {
				// Convert Neo4j values to compatible types
				values[j] = convertNeo4jValue(rawValue)
			} else {
				values[j] = nil
			}
		}
		result.Data[i] = values
	}

	return result, nil
}

// convertNeo4jValue converts a Neo4j value to a compatible type for neuPrintHTTP
func convertNeo4jValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}

	// Log the value type if verbose numeric debugging is enabled
	if storage.VerboseNumeric {
		fmt.Printf("Neo4j value: %v, Type: %T\n", val, val)
	}

	// Handle different Neo4j types
	switch v := val.(type) {
	case neo4j.Point3D:
		// Convert Neo4j Point3D to a map with field "coordinates"
		href := fmt.Sprintf("http://spatialreference.org/ref/sr-org/%d/ogcwkt/", v.SpatialRefId)
		return map[string]interface{}{
			"coordinates": []float64{v.X, v.Y, v.Z},
			"crs": map[string]interface{}{
				"name": "cartesian-3d",
				"properties": map[string]interface{}{
					"href": href,
					"type": "ogcwkt",
				},
				"srid": v.SpatialRefId,
				"type": "link",
			},
			"type": "Point",
		}

	case neo4j.Node:
		// Convert Neo4j node to a map with its properties
		props := make(map[string]interface{})
		for key, value := range v.Props {
			props[key] = convertNeo4jValue(value)
		}
		return props

	case neo4j.Relationship:
		// Convert Neo4j relationship to a map with its properties
		props := make(map[string]interface{})
		props["id"] = v.Id
		props["type"] = v.Type
		props["startNodeId"] = v.StartId
		props["endNodeId"] = v.EndId
		for key, value := range v.Props {
			props[key] = convertNeo4jValue(value)
		}
		return props
	case neo4j.Path:
		// Convert Neo4j path to a representation we can use
		return map[string]interface{}{
			"nodes":         convertNeo4jValue(v.Nodes),
			"relationships": convertNeo4jValue(v.Relationships),
		}
	case []interface{}:
		// Convert slice elements
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = convertNeo4jValue(item)
		}
		return result
	case map[string]interface{}:
		// Convert map values
		result := make(map[string]interface{})
		for key, item := range v {
			result[key] = convertNeo4jValue(item)
		}
		return result
	case int64:
		// Preserve int64 values exactly as they are
		return v
	case float64:
		// Check if this float represents an integer exactly
		if float64(int64(v)) == v {
			return int64(v)
		}
		return v
	case string, bool:
		// Pass through simple scalar types
		return v
	case json.Number:
		// First try to parse as int64
		if i, err := v.Int64(); err == nil {
			return i
		}
		// Fallback to float64
		if f, err := v.Float64(); err == nil {
			return f
		}
		// If all else fails, use the string representation
		return v.String()
	default:
		// For any other type, convert to string
		return fmt.Sprintf("%v", v)
	}
}

// Kill aborts the transaction
func (t *Transaction) Kill() error {
	if t.isExplicit && t.tx != nil {
		err := t.tx.Rollback(t.ctx)
		t.tx = nil
		if err != nil {
			return fmt.Errorf("failed to rollback transaction: %w", err)
		}
	}

	if t.session != nil {
		err := t.session.Close(t.ctx)
		t.session = nil
		if err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
	}

	return nil
}

// Commit commits the transaction
func (t *Transaction) Commit() error {
	if t.isExplicit && t.tx != nil {
		err := t.tx.Commit(t.ctx)
		t.tx = nil
		if err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	if t.session != nil {
		err := t.session.Close(t.ctx)
		t.session = nil
		if err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
	}

	return nil
}
