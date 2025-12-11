// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"fmt"
	"strings"
)

// Algorithm names.
const (
	AlgorithmRoundRobin = "round-robin"
	AlgorithmWeighted   = "weighted"
	AlgorithmFailover   = "failover"
)

// NewRouter creates a router based on the algorithm name.
// Supported algorithms: round-robin, weighted, failover.
func NewRouter(algorithm string) (Router, error) {
	switch strings.ToLower(algorithm) {
	case AlgorithmRoundRobin, "roundrobin", "rr":
		return NewRoundRobinRouter(), nil
	case AlgorithmWeighted, "weight":
		return NewWeightedRouter(), nil
	case AlgorithmFailover, "active-standby", "activestandby":
		return NewFailoverRouter(), nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", algorithm)
	}
}
