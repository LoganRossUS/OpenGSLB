// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dns_test

import (
	"fmt"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/metrics"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"
)

// The "domain" label of the query metrics must not take one value per name a
// client asks for. Uses newFuzzHandler and fuzzResponseWriter from
// handler_fuzz_test.go.

func metricSeries(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric, 1<<16)
	go func() {
		c.Collect(ch)
		close(ch)
	}()
	n := 0
	for range ch {
		n++
	}
	return n
}

// The query name is chosen by whoever sends the query. If it is used as a
// metric label as it is, every distinct name adds time series that are never
// removed, so a random-subdomain flood grows the process without bound.
func TestHandler_QueryNamesDoNotCreateUnboundedMetricSeries(t *testing.T) {
	h := newFuzzHandler(t, true)
	metrics.DNSQueriesTotal.Reset()
	metrics.DNSQueryDuration.Reset()
	t.Cleanup(metrics.DNSQueriesTotal.Reset)
	t.Cleanup(metrics.DNSQueryDuration.Reset)

	const names = 500
	for i := 0; i < names; i++ {
		for _, name := range []string{
			fmt.Sprintf("r%d.gslb.example.com.", i), // not registered, inside a served zone
			fmt.Sprintf("r%d.example.org.", i),      // outside every zone
		} {
			q := new(dns.Msg)
			q.SetQuestion(name, dns.TypeA)
			h.ServeDNS(&fuzzResponseWriter{}, q)
		}
	}

	// Nine domains are registered. Whatever labels the handler uses for names
	// it does not serve, their number must not follow the number of names
	// queried.
	const limit = 50
	if got := metricSeries(metrics.DNSQueriesTotal); got > limit {
		t.Errorf("dns_queries_total has %d series after queries for %d unregistered names, want at most %d", got, 2*names, limit)
	}
	if got := metricSeries(metrics.DNSQueryDuration); got > limit {
		t.Errorf("dns_query_duration_seconds has %d series after queries for %d unregistered names, want at most %d", got, 2*names, limit)
	}
}
