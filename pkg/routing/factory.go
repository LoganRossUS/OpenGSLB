// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

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
