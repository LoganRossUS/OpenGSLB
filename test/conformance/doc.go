// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package conformance holds the DNS protocol conformance tests of OpenGSLB.
//
// The tests start the real DNS server (pkg/dns Server and Handler, built from
// a parsed and validated configuration the same way cmd/opengslb builds them)
// on a loopback port and query it over UDP and TCP with a DNS client. Each
// case states the response code, the AA flag and the exact answer and
// authority sections it expects, so a change in wire behavior shows up here
// as a failing case with the name of the rule it broke.
//
// The cases follow the behavior required of an authoritative-only server:
// RFC 1034 and 1035 (answers, NXDOMAIN, case-insensitive names), RFC 2308
// (negative answers carry the zone SOA), RFC 6891 (EDNS(0): OPT, UDP payload
// size, truncation), RFC 7871 (EDNS Client Subnet) and RFC 8020 (empty
// non-terminals).
//
// Run them with:
//
//	go test -race -count=1 ./test/conformance/...
//
// They need no network access beyond the loopback interface, no containers
// and no build tag, so they also run as part of "go test ./...".
package conformance
