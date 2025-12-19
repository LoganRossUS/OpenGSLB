// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Agent-side metrics for latency collection
var (
	// observationsTotal is the total number of RTT observations collected.
	observationsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_observations_total",
			Help: "Total RTT observations collected from TCP connections",
		},
	)

	// observationsDroppedTotal is the number of observations dropped due to full channel.
	observationsDroppedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_observations_dropped_total",
			Help: "Total RTT observations dropped due to full channel",
		},
	)

	// subnetsTracked is the number of client subnets currently being tracked.
	subnetsTracked = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "opengslb_agent_latency_subnets_tracked",
			Help: "Number of client subnets currently tracked",
		},
	)

	// subnetsPruned is the number of subnet entries removed due to TTL expiration.
	subnetsPruned = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_subnets_pruned_total",
			Help: "Total subnet entries removed due to TTL expiration",
		},
	)

	// subnetsEvicted is the number of subnet entries evicted due to capacity limits.
	subnetsEvicted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_subnets_evicted_total",
			Help: "Total subnet entries evicted due to capacity limits",
		},
	)

	// pollDuration is the time spent polling the OS for TCP statistics.
	pollDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "opengslb_agent_latency_poll_duration_seconds",
			Help:    "Time spent polling OS for TCP statistics",
			Buckets: prometheus.DefBuckets,
		},
	)

	// reportsSent is the total number of latency reports sent to overwatches.
	reportsSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_reports_sent_total",
			Help: "Total latency reports sent to overwatch nodes",
		},
	)

	// collectorErrors is the total number of collector errors encountered.
	collectorErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opengslb_agent_latency_collector_errors_total",
			Help: "Total errors encountered during latency collection",
		},
		[]string{"type"}, // "poll", "capability", "parse"
	)

	// rttHistogram is a histogram of observed RTT values.
	rttHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "opengslb_agent_latency_rtt_seconds",
			Help: "Histogram of observed TCP RTT values",
			// Buckets from 1ms to 1s
			Buckets: []float64{
				0.001, 0.002, 0.005, 0.010, 0.020, 0.050,
				0.100, 0.200, 0.500, 1.000,
			},
		},
	)
)

// RecordRTT records an RTT observation to the histogram.
func RecordRTT(rtt float64) {
	rttHistogram.Observe(rtt)
}

// RecordPollDuration records the time spent in a poll operation.
func RecordPollDuration(seconds float64) {
	pollDuration.Observe(seconds)
}

// RecordReportSent increments the reports sent counter.
func RecordReportSent() {
	reportsSent.Inc()
}

// RecordCollectorError records a collector error.
func RecordCollectorError(errType string) {
	collectorErrors.WithLabelValues(errType).Inc()
}
