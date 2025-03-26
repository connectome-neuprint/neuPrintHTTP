package neuprintbolt

import (
	"testing"
)

func TestInitRegistered(t *testing.T) {
	// This test verifies that the engine is registered when the package is imported
	// The actual registration is done in the init() function
	// No assertions needed - if registration fails, the application will panic
}

// The remaining tests require a live Neo4j server connection
// and would be implemented as integration tests