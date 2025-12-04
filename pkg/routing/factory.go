package routing

import (
	"fmt"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// NewRouter creates a router based on the algorithm name.
// Supported algorithms:
//   - "round-robin" (default): Equal distribution across servers
//   - "weighted": Proportional distribution based on server weights
//   - "failover": Active/standby with ordered failover priority
func NewRouter(algorithm string) (dns.Router, error) {
	switch algorithm {
	case "round-robin", "roundrobin", "":
		return NewRoundRobin(), nil
	case "weighted", "weighted-round-robin":
		return NewWeighted(), nil
	case "failover", "active-standby", "priority":
		return NewFailover(), nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}
