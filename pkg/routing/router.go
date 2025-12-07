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

// Package routing provides server selection algorithms for DNS load balancing.
// The term "routing" refers to DNS response routing (selecting which server IP
// to return in DNS answers), not network traffic routing. See ADR-011.
package routing

import "errors"

// ErrNoHealthyServers is returned when Route is called with an empty server slice.
// This typically occurs when all backend servers have failed health checks.
var ErrNoHealthyServers = errors.New("no healthy servers available")
