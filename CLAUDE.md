# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# neuPrintHTTP Coding Commands & Guidelines

## Build & Run Commands
- Build: `go build`
- Install: `go install github.com/connectome-neuprint/neuPrintHTTP@latest`
- Build with Kafka: `go install -tags kafka`
- Generate Swagger docs: `go generate`
- Run: `neuprintHTTP -port PORTNUM config.json`

## Testing
- All tests: `go test ./...`
- Specific package: `go test ./api/...`
- Single test: `go test -run TestName ./package/...`
- With verbose output: `go test -v ./...`

## Code Style
- **Formatting**: Use `gofmt` for consistent formatting
- **Structure**: Main package in root; supporting packages by functionality
- **Naming**: CamelCase for exported, lowerCamelCase for unexported items
- **Imports**: Standard lib first, third-party second, project imports last
- **Error Handling**: Return errors with `fmt.Errorf`, use pattern `if err != nil { return nil, err }`
- **Documentation**: Comment all exported functions; use Swagger annotations for API endpoints
- **Configuration**: JSON-based with sample in `sampleconfig.json`
- **Typing**: Use explicit types; interfaces end with 'er' (e.g., `Authorizer`)

## Dev Notes
- For cell type analysis: install scipy, scikit-learn, and pandas
- Run from top directory where Python scripts are located