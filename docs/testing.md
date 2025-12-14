# Testing Guide

## Overview

OpenGSLB uses two test levels: unit tests and integration tests.

## Unit Tests

Run unit tests (no external dependencies):

```bash
make test
```

With coverage:

```bash
make test-coverage
```

### Test Coverage by Package

| Package | Coverage | Description |
|---------|----------|-------------|
| pkg/config | 92% | Configuration loading and validation |
| pkg/dns | 87% | DNS server and record handling |
| pkg/health | 90% | Health check framework |
| pkg/routing | 93% | Routing algorithms |
| pkg/agent | ~90% | Agent mode functionality |
| pkg/overwatch | ~88% | Overwatch mode functionality |
| pkg/api/dashboard | ~60% | Overlord dashboard API |
| pkg/metrics | 85% | Prometheus metrics |

**Target Coverage:** 89%

### Dashboard API Tests

The Overlord dashboard API has comprehensive unit tests covering:

- **Handler Tests** (`handlers_*_test.go`): Tests for each API endpoint
- **Server Tests** (`server_test.go`): Server initialization, CORS, ACL middleware
- **Audit Tests** (`audit_test.go`): Audit logging functionality
- **Type Tests**: Response type validation

Run Dashboard API tests specifically:

```bash
go test -v ./pkg/api/dashboard/...
```

With coverage:

```bash
go test -cover ./pkg/api/dashboard/...
```

## Integration Tests

Integration tests require Docker and validate the full system against mock services.

### Test Environment

The test environment (`docker-compose.test.yml`) includes:
- **backend1, backend2**: nginx containers simulating upstream servers
- **dns-mock**: CoreDNS container for DNS resolution testing

### Running Locally

```bash
# Full cycle (start env, test, stop env)
make test-integration

# Or manually:
make test-env-up
make test-integration-only
make test-env-down
```

### Test Network

Services run on a Docker network (`172.28.0.0/16`):
- DNS mock: `172.28.0.100`
- Backends: assigned dynamically by Docker

### Writing Integration Tests

Integration tests use build tags:

```go
//go:build integration

package integration

import "testing"

func TestSomething(t *testing.T) {
    // test code
}
```

Place tests in `test/integration/`.

## CI

Both unit and integration tests run automatically on PRs to `main` and `develop`. Integration tests run in GitHub Actions using the same Docker Compose setup.
