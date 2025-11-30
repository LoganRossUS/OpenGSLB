// Package routing provides server selection algorithms for DNS load balancing.
// The term "routing" refers to DNS response routing (selecting which server IP
// to return in DNS answers), not network traffic routing. See ADR-011.
package routing

import "errors"

// ErrNoHealthyServers is returned when Route is called with an empty server slice.
// This typically occurs when all backend servers have failed health checks.
var ErrNoHealthyServers = errors.New("no healthy servers available")
