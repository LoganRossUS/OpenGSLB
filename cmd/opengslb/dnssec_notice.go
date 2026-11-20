// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"log/slog"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// warnDNSSECNotActive logs once, at WARN, when the configuration asks for
// DNSSEC. The signing code in pkg/dnssec is not connected to the DNS server
// in this build: no keys are created, responses carry no RRSIG records and
// the DNSSEC API endpoints are not served. An operator who set
// overwatch.dnssec.enabled must not be left believing the zones are signed.
func warnDNSSECNotActive(logger *slog.Logger, cfg *config.Config) {
	if cfg == nil || !cfg.Overwatch.DNSSEC.Enabled {
		return
	}
	logger.Warn("DNSSEC signing is not active in this build: overwatch.dnssec.enabled is true, but responses are served unsigned (no RRSIG or DNSKEY records) and the DNSSEC API endpoints are not available. Do not publish DS records for these zones.")
}
