# Contributing to H3 SDK for Go

Go SDK for building H3-compliant agent harnesses. Implements the harness side of the H3 protocol.

## Development Setup

```bash
cd sdk-go/
go mod download
```

## Package Structure

```
sdk-go/
├── protocol/
│   ├── types.go       # Go types (generated from protocol repo JSON Schema)
│   ├── validate.go    # Protocol-level validation
│   └── types_test.go
├── harness/
│   ├── interface.go   # Harness interface definition
│   ├── http.go        # HTTP server + router
│   ├── middleware.go   # Request logging middleware
│   └── harness_test.go
├── testbed/
│   ├── mock_hermes.go # MockHermes for unit testing
│   └── testbed_test.go
└── examples/
    ├── echo/          # Minimal echo harness
    ├── minimal/       # Bare-minimum example
    ├── conformance/   # Full conformance test harness
    └── consensus/     # Multi-model consensus demo
```

## Before Making Changes

### Run Tests

```bash
go test ./... -count=1
```

### Run Vet + Build

```bash
go vet ./...
go build ./...
```

### Run the Test Battery

```bash
# Start the echo example in one terminal:
go run ./examples/echo/

# In another terminal, run the compliance test battery:
h3-test --endpoint http://localhost:9191
# 43 compliance tests, exit code 0 = compliant
```

### Sync Protocol Types

If the upstream protocol changed:

```bash
# Regenerate types from get-h3/protocol schemas
go run ./scripts/sync_protocol/
```

Never hand-edit generated types in `protocol/types.go`.

## Making Changes

### Harness Interface

- `harness/interface.go` defines the `Harness` interface that all harnesses implement
- Changes to the interface are MAJOR — they break all existing harnesses
- New optional methods should use a separate interface with type assertion

### HTTP Server

- `harness/http.go` handles request routing, JSON serialization, error responses
- Must follow the H3 protocol exactly — see `get-h3/protocol/h3-protocol.yaml`
- All endpoints log METHOD /path STATUS DURATION

### Middleware

- `harness/middleware.go` provides request logging
- Must not leak credentials or sensitive data in logs

### Protocol Validation

- `protocol/validate.go` enforces required fields and format constraints
- Must match the JSON Schema definitions from `get-h3/protocol/schemas/v1/`

## Quality Gates

### Pre-Commit

```bash
go vet ./...        # Static analysis
go test ./...       # All tests
gofmt -s -w .       # Format
```

### CI Pipeline

GitHub Actions runs on every PR:
1. `go vet ./...`
2. `go test ./... -race -count=1`
3. `gofmt -s -d .` (must be clean)
4. `h3-test --endpoint http://localhost:9191` (against echo example)

All must pass.

## Release

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Review Checklist

- [ ] `go test ./...` passes
- [ ] `go vet ./...` clean
- [ ] `h3-test --endpoint http://localhost:9191` passes against echo example
- [ ] New features have tests
- [ ] Protocol changes regenerated from upstream
- [ ] No hand-edits to generated types

## Questions?

See the umbrella project at [get-h3/h3](https://github.com/get-h3/h3) for architecture, specs, and the cross-repo task board.
