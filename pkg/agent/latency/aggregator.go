// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"net/netip"
	"sort"
	"sync"
	"time"
)

// subnetEntry is the internal tracking structure for a subnet.
type subnetEntry struct {
	stats       SubnetStats
	lastUpdated time.Time
}

// Aggregator collects observations and aggregates them by subnet.
// Thread-safe for concurrent use.
type Aggregator struct {
	mu      sync.RWMutex
	subnets map[netip.Prefix]*subnetEntry
	config  AggregatorConfig
}

// NewAggregator creates a new subnet aggregator.
func NewAggregator(cfg AggregatorConfig) *Aggregator {
	// Apply defaults
	if cfg.IPv4Prefix == 0 {
		cfg.IPv4Prefix = DefaultAggregatorConfig().IPv4Prefix
	}
	if cfg.IPv6Prefix == 0 {
		cfg.IPv6Prefix = DefaultAggregatorConfig().IPv6Prefix
	}
	if cfg.EWMAAlpha == 0 {
		cfg.EWMAAlpha = DefaultAggregatorConfig().EWMAAlpha
	}
	if cfg.MaxSubnets == 0 {
		cfg.MaxSubnets = DefaultAggregatorConfig().MaxSubnets
	}
	if cfg.SubnetTTL == 0 {
		cfg.SubnetTTL = DefaultAggregatorConfig().SubnetTTL
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = DefaultAggregatorConfig().MinSamples
	}

	return &Aggregator{
		subnets: make(map[netip.Prefix]*subnetEntry),
		config:  cfg,
	}
}

// Record adds an observation to the aggregator.
// The observation's remote IP is bucketed into the appropriate subnet.
func (a *Aggregator) Record(obs Observation) {
	subnet := a.toSubnet(obs.RemoteAddr)

	a.mu.Lock()
	defer a.mu.Unlock()

	entry, exists := a.subnets[subnet]
	if !exists {
		// Check if we're at capacity
		if len(a.subnets) >= a.config.MaxSubnets {
			// Drop oldest entry to make room
			a.evictOldest()
		}

		entry = &subnetEntry{
			stats: SubnetStats{
				Subnet:      subnet,
				EWMA:        obs.RTT,
				MinRTT:      obs.RTT,
				MaxRTT:      obs.RTT,
				SampleCount: 0,
				LastUpdated: obs.Timestamp,
			},
			lastUpdated: obs.Timestamp,
		}
		a.subnets[subnet] = entry
	}

	// Update EWMA: smoothed = alpha * measured + (1 - alpha) * previous
	alpha := a.config.EWMAAlpha
	if entry.stats.SampleCount == 0 {
		// First sample, use directly
		entry.stats.EWMA = obs.RTT
	} else {
		entry.stats.EWMA = time.Duration(
			alpha*float64(obs.RTT) + (1-alpha)*float64(entry.stats.EWMA),
		)
	}

	entry.stats.SampleCount++
	entry.stats.LastUpdated = obs.Timestamp
	entry.lastUpdated = obs.Timestamp

	// Update min/max
	if obs.RTT < entry.stats.MinRTT {
		entry.stats.MinRTT = obs.RTT
	}
	if obs.RTT > entry.stats.MaxRTT {
		entry.stats.MaxRTT = obs.RTT
	}

	// Update metrics
	observationsTotal.Inc()
}

// GetReportable returns subnet stats that meet the minimum sample threshold.
func (a *Aggregator) GetReportable() []SubnetStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []SubnetStats
	for _, entry := range a.subnets {
		if entry.stats.SampleCount >= uint64(a.config.MinSamples) {
			result = append(result, entry.stats)
		}
	}
	return result
}

// GetAll returns all subnet stats regardless of sample count.
func (a *Aggregator) GetAll() []SubnetStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]SubnetStats, 0, len(a.subnets))
	for _, entry := range a.subnets {
		result = append(result, entry.stats)
	}
	return result
}

// GetSubnet returns stats for a specific subnet, if tracked.
func (a *Aggregator) GetSubnet(prefix netip.Prefix) (SubnetStats, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entry, exists := a.subnets[prefix]
	if !exists {
		return SubnetStats{}, false
	}
	return entry.stats, true
}

// SubnetCount returns the number of tracked subnets.
func (a *Aggregator) SubnetCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.subnets)
}

// Prune removes expired entries and enforces memory limits.
// This should be called periodically (e.g., every hour).
func (a *Aggregator) Prune() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	var toDelete []netip.Prefix

	for prefix, entry := range a.subnets {
		if now.Sub(entry.lastUpdated) > a.config.SubnetTTL {
			toDelete = append(toDelete, prefix)
		}
	}

	for _, prefix := range toDelete {
		delete(a.subnets, prefix)
		subnetsPruned.Inc()
	}

	// Update metrics
	subnetsTracked.Set(float64(len(a.subnets)))
}

// Clear removes all tracked subnets.
func (a *Aggregator) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.subnets = make(map[netip.Prefix]*subnetEntry)
	subnetsTracked.Set(0)
}

// toSubnet converts an IP address to its aggregated subnet prefix.
func (a *Aggregator) toSubnet(addr netip.Addr) netip.Prefix {
	var bits int
	if addr.Is4() {
		bits = a.config.IPv4Prefix
	} else {
		bits = a.config.IPv6Prefix
	}

	prefix, err := addr.Prefix(bits)
	if err != nil {
		// Should not happen with valid addresses
		return netip.PrefixFrom(addr, addr.BitLen())
	}
	return prefix
}

// evictOldest removes the oldest entry to make room for new ones.
// Called when at capacity. Must be called with lock held.
func (a *Aggregator) evictOldest() {
	if len(a.subnets) == 0 {
		return
	}

	var oldest netip.Prefix
	var oldestTime time.Time
	first := true

	for prefix, entry := range a.subnets {
		if first || entry.lastUpdated.Before(oldestTime) {
			oldest = prefix
			oldestTime = entry.lastUpdated
			first = false
		}
	}

	delete(a.subnets, oldest)
	subnetsEvicted.Inc()
}

// ToReport converts the aggregator's data to a LatencyReport for gossip.
func (a *Aggregator) ToReport(agentID, backend, region string) LatencyReport {
	stats := a.GetReportable()

	subnets := make([]SubnetLatency, 0, len(stats))
	for _, s := range stats {
		subnets = append(subnets, SubnetLatency{
			Subnet:      s.Subnet.String(),
			EWMA:        int64(s.EWMA),
			SampleCount: s.SampleCount,
			LastSeen:    s.LastUpdated,
		})
	}

	// Sort by subnet for deterministic output
	sort.Slice(subnets, func(i, j int) bool {
		return subnets[i].Subnet < subnets[j].Subnet
	})

	return LatencyReport{
		AgentID:   agentID,
		Backend:   backend,
		Region:    region,
		Timestamp: time.Now(),
		Subnets:   subnets,
	}
}
