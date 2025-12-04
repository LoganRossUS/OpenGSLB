package routing

import (
	"context"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// Ensure Failover implements dns.Router interface.
var _ dns.Router = (*Failover)(nil)

// Failover implements active/standby routing with ordered failover.
// Servers are tried in the order they appear in the configuration.
// The first healthy server is always selected, providing automatic
// failover to secondary servers when the primary is unhealthy,
// and automatic return to primary when it recovers.
type Failover struct{}

// NewFailover creates a new failover router.
func NewFailover() *Failover {
	return &Failover{}
}

// Route selects the first healthy server from the slice.
// Servers are evaluated in order, so the first server in the slice
// is considered the primary, the second is the first fallback, etc.
//
// This provides:
//   - Automatic failover: if primary is unhealthy, secondary is used
//   - Automatic recovery: when primary becomes healthy, traffic returns to it
//
// Returns ErrNoHealthyServers if no healthy servers are available.
func (f *Failover) Route(_ context.Context, _ string, servers []dns.ServerInfo) (*dns.ServerInfo, error) {
	if len(servers) == 0 {
		return nil, ErrNoHealthyServers
	}

	// Servers are pre-filtered to healthy only by the DNS handler,
	// so we just return the first one (highest priority healthy server)
	return &servers[0], nil
}

// Algorithm returns the name of this routing algorithm.
func (f *Failover) Algorithm() string {
	return "failover"
}
