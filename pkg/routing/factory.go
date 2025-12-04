package routing

import (
	"fmt"

	"github.com/loganrossus/OpenGSLB/pkg/dns"
)

// NewRouter creates a router based on the algorithm name.
// Supported algorithms:
//   - "round-robin" (default): Equal distribution across servers
//   - "weighted": Proportional distribution based on server weights
func NewRouter(algorithm string) (dns.Router, error) {
	switch algorithm {
	case "round-robin", "roundrobin", "":
		return NewRoundRobin(), nil
	case "weighted", "weighted-round-robin":
		return NewWeighted(), nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}
