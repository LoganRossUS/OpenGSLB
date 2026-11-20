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

The workflows take the Go version from the `go` line of `go.mod`. The lint job runs a pinned golangci-lint v2 release (see `.github/workflows/ci.yml`); use the same version locally, because golangci-lint must be built with a Go release at least as new as the one in `go.mod`.

Security checks:

| Check | Workflow | When | Outcome |
|-------|----------|------|------------|
| `govulncheck ./...` | `security.yml` | PRs to `main` and `develop`, pushes to `main`, weekly | Fails when the code calls a function that has a published Go advisory |
| CodeQL (Go) | `codeql.yml` | PRs, pushes to `main` and `develop`, weekly | Alerts appear under the repository's Security tab and on the pull request |
| Trivy image scan | `docker-build.yml` | Before the image is pushed | Fails, and the image is not pushed, when it has a HIGH or CRITICAL vulnerability |

Run the vulnerability check locally with:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

Dependabot (`.github/dependabot.yml`) opens weekly update pull requests against `develop` for Go modules, GitHub Actions and the Dockerfile base images.
