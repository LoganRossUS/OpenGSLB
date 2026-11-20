# Testing Guide

## Overview

OpenGSLB uses two test levels: unit tests and integration tests. The parsers that take input from the network also have fuzz tests.

## Unit Tests

Run unit tests (no external dependencies):

```bash
make test
```

With coverage:

```bash
make test-coverage
```

## Fuzz Tests

The code that parses input from the network has Go native fuzz targets. Each target states the invariants it checks in a comment at the top of its file.

| Target | Package | Input |
|--------|---------|-------|
| `FuzzHandlerServeDNS` | `pkg/dns` | Raw DNS message bytes, unpacked as the server unpacks them and passed to `Handler.ServeDNS` over UDP and TCP, with ECS honored and ignored |
| `FuzzReceiverNotifyMsg` | `pkg/gossip` | Raw gossip message bytes passed to the memberlist delegate of the Overwatch receiver |
| `FuzzReceiverNodeEvents` | `pkg/gossip` | Node name, address and metadata of a remote cluster member on join, update and leave |
| `FuzzGossipHandleMessage` | `pkg/overwatch` | Raw gossip message bytes decoded like the receiver and handled against a real backend registry, DNS registry, latency table and agent auth, followed by DNS queries for every configured domain |
| `FuzzGossipPayloadRoundTrip` | `pkg/overwatch` | Typed payloads of every message type, encoded like the agent and decoded like the Overwatch, which must give back what was sent |

The seed corpus of every target runs as an ordinary test in `make test`, so a regression in a seed fails the normal build. To fuzz, run one target at a time (`go test` accepts one `-fuzz` target per package):

```bash
go test -run='^$' -fuzz='^FuzzHandlerServeDNS$' -fuzztime=60s ./pkg/dns/
go test -run='^$' -fuzz='^FuzzReceiverNotifyMsg$' -fuzztime=60s ./pkg/gossip/
go test -run='^$' -fuzz='^FuzzReceiverNodeEvents$' -fuzztime=60s ./pkg/gossip/
go test -run='^$' -fuzz='^FuzzGossipHandleMessage$' -fuzztime=60s ./pkg/overwatch/
go test -run='^$' -fuzz='^FuzzGossipPayloadRoundTrip$' -fuzztime=60s ./pkg/overwatch/
```

When a target fails, Go writes the input to `testdata/fuzz/<Target>/` in the package directory and prints the command that replays it. Fix the defect, keep the file and commit it with the fix: it then runs with the seed corpus on every `go test`.

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
