# Contributing to H3 SDK for Go

Thanks for contributing to the H3 Go SDK. This document covers the workflow, quality gates, and conventions.

## Development

```bash
go build ./...              # Build all packages
go vet ./...                # Static analysis
go test -race -count=1 ./...  # Run tests with race detector
golangci-lint run ./...     # Lint (0 issues required)
```

Requirements: Go 1.22+ (toolchain go1.26.5).

## Quality Gates

Every PR must pass:

| Gate | Command | Requirement |
|------|---------|-------------|
| Build | `go build ./...` | Zero errors |
| Vet | `go vet ./...` | Zero warnings |
| Lint | `golangci-lint run ./...` | 0 issues |
| Tests | `go test -race -count=1 ./...` | 100% pass, no data races |
| Coverage | `go test -cover ./...` | Protocol ≥ 90%, Harness ≥ 80% |
| GitReins | `gitreins guard` | Tier 1 pass (secrets, lint, tests) |

## Project Structure

```
sdk-go/
  protocol/     Core types + validation (JSON Schema → Go)
  harness/      HTTP handler + middleware (net/http)
  testbed/      MockHermes, assertions, conformance tester
  examples/     Runnable demos (echo, minimal, consensus, conformance)
  cmd/gen-types/  Code generator (reads schemas, writes Go types)
```

## Adding a Feature

1. Create a feature branch from `main`
2. Implement with tests (target ≥ 80% coverage on new code)
3. Run full quality gates locally
4. Open a PR — CI runs the same gates
5. Merge after review + green CI

## Commit Convention

Follow conventional commits: `type: description`

Types: `feat`, `fix`, `test`, `docs`, `chore`, `refactor`

Example: `feat: add streaming support to ProtocolRequest`

## Architecture Decisions

This SDK implements the [H3 specification](https://github.com/get-h3/h3). Protocol types are generated from JSON Schema (`schemas/v1/`). The `Harness` interface is the single integration point — harness implementors only need to satisfy 5 methods.

Design principles:
- Zero external dependencies (stdlib only, except `uuid` in examples)
- `http.Handler` compatibility (not a framework — plug into any Go HTTP server)
- Full JSON Schema round-trip fidelity

## Questions

Open a GitHub issue or discussion. For spec questions, see the [H3 repository](https://github.com/get-h3/h3).
