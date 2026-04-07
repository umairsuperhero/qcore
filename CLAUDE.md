# QCore - Claude Code Context

## Project
QCore is an open-source 4G (LTE) core network written in Go. Phase 1 is the HSS (Home Subscriber Server).

## Build Commands
```bash
make build        # Build binary to bin/qcore-hss
make test         # Run all tests with race detector
make test-short   # Run tests without integration tests
make lint         # Run golangci-lint
make docker-up    # Start PostgreSQL + HSS via Docker Compose
make docker-down  # Stop Docker services
```

## Code Conventions
- Go 1.21+, standard Go conventions
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Table-driven tests with testify/assert
- API tests use httptest
- Structured logging via pkg/logger (never import logrus directly)
- Config via pkg/config (viper, YAML + env vars with QCORE_ prefix)

## Architecture
- `cmd/` — CLI entry points (cobra)
- `pkg/` — Public library code (logger, config, database, metrics, hss)
- `internal/` — Private code (models)
- `deployments/` — Docker, Kubernetes configs

## Key Files
- `pkg/hss/auth.go` — Milenage crypto (3GPP TS 35.206), must match TS 35.207 test vectors
- `pkg/hss/api.go` — REST API handlers
- `cmd/hss/main.go` — CLI wiring

## Module Path
`github.com/qcore-project/qcore`
