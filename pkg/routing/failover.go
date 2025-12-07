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
