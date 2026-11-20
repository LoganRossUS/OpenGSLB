# Testing Guide

## Overview

OpenGSLB uses three test levels: unit tests, DNS conformance tests and integration tests.

## Unit Tests

Run unit tests (no external dependencies):

```bash
make test
```

With coverage:

```bash
make test-coverage
```

## DNS Conformance Tests

The conformance tests (`test/conformance/`) check what the DNS server puts on the wire. They start the real DNS server in the test process on a loopback port, built from a parsed and validated configuration the same way the `opengslb` binary builds it, and query it with a DNS client over UDP and TCP, without and with EDNS. They need no Docker, no network access beyond loopback and no build tag, so `make test` runs them too.

```bash
make test-conformance
# or
go test -race -count=1 ./test/conformance/...
```

Each row of the table in `conformance_test.go` is one query with the response it must get: the response code, the AA flag and the complete answer and authority sections. The table covers:

| Query | Response |
|-------|----------|
| A or AAAA for a name with a healthy server of that family | NOERROR, AA, one record. The owner name and the question keep the case the querier used. |
| A or AAAA for a name that has no server of that family | NOERROR, AA, empty answer, zone SOA in the authority section |
| A or AAAA when servers of that family are configured and none is healthy | SERVFAIL, AA clear (an answer instead when `dns.return_last_healthy` is true) |
| Any other type at a name that exists (TXT, MX, CNAME, HTTPS, ANY, DS, ...) | NOERROR, AA, empty answer, zone SOA in the authority section |
| Any type at a name that does not exist inside a served zone | NXDOMAIN, AA, zone SOA in the authority section |
| Any type at a name with nothing of its own but a name beneath it (empty non-terminal) | NOERROR, AA, empty answer, zone SOA in the authority section |
| Any type at a name outside every served zone | REFUSED, AA clear, no records |
| SOA and NS at a zone apex | NOERROR, AA, the zone's SOA or NS set |
| AXFR, IXFR | REFUSED |

Further tests in the package cover EDNS (an OPT record in the response exactly when the query has one, the advertised UDP size, BADVERS), UDP response size limits and truncation with the TC bit (and no truncation over TCP), EDNS Client Subnet echo and scope, `dns.return_last_healthy`, and requests that are not class IN queries.

When a change to the DNS server alters a response on purpose, change the matching row in the same commit and say so in the pull request.

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

Unit, conformance and integration tests run automatically on PRs to `main` and `develop`. Integration tests run in GitHub Actions using the same Docker Compose setup.

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
