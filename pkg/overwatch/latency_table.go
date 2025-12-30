// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"log/slog"
	"net/netip"
	"sync"
	"time"
)

// LearnedLatencyTable stores learned latency data from agents (ADR-017).
// Thread-safe for concurrent access.
type LearnedLatencyTable struct {
	mu     sync.RWMutex
	config LearnedLatencyConfig
	logger *slog.Logger

	// data maps subnet -> backendKey -> BackendLatency
	// backendKey is "backend|region" to track each region's latency separately
	data map[netip.Prefix]map[string]*BackendLatency
}

// LearnedLatencyConfig configures the learned latency table.
type LearnedLatencyConfig struct {
	// MaxEntries is the maximum number of subnet entries to store.
	// Default: 100000
	MaxEntries int

	// EntryTTL is how long to keep entries without updates.
	// Default: 168h (7 days)
	EntryTTL time.Duration

	// MinSamples is the minimum samples before using latency data for routing.
	// Default: 5
	MinSamples int

	// Logger for table operations.
	Logger *slog.Logger
}

// DefaultLearnedLatencyConfig returns sensible defaults.
func DefaultLearnedLatencyConfig() LearnedLatencyConfig {
	return LearnedLatencyConfig{
		MaxEntries: 100000,
		EntryTTL:   168 * time.Hour, // 7 days
		MinSamples: 5,
		Logger:     slog.Default(),
	}
}

// BackendLatency contains learned latency for a backend from a subnet.
type BackendLatency struct {
	// Backend is the backend service name.
	Backend string
	// Region is the agent's region.
	Region string
	// EWMA is the smoothed RTT.
	EWMA time.Duration
	// SampleCount is the number of samples.
	SampleCount uint64
	// LastUpdated is when this entry was last updated.
	LastUpdated time.Time
	// Source is the agent ID that reported this data.
	Source string
}

// NewLearnedLatencyTable creates a new learned latency table.
func NewLearnedLatencyTable(cfg LearnedLatencyConfig) *LearnedLatencyTable {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = DefaultLearnedLatencyConfig().MaxEntries
	}
	if cfg.EntryTTL == 0 {
		cfg.EntryTTL = DefaultLearnedLatencyConfig().EntryTTL
	}
	if cfg.MinSamples == 0 {
		cfg.MinSamples = DefaultLearnedLatencyConfig().MinSamples
	}

	return &LearnedLatencyTable{
		config: cfg,
		logger: cfg.Logger,
		data:   make(map[netip.Prefix]map[string]*BackendLatency),
	}
}

// Update processes a latency report from an agent.
// Implements the LatencyTable interface.
func (t *LearnedLatencyTable) Update(agentID, region, backend string, subnets []SubnetLatencyData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	updated := 0

	for _, s := range subnets {
		prefix, err := netip.ParsePrefix(s.Subnet)
		if err != nil {
			t.logger.Warn("invalid subnet in latency report",
				"subnet", s.Subnet,
				"agent_id", agentID,
				"error", err,
			)
			continue
		}

		// Get or create subnet entry
		backendMap, exists := t.data[prefix]
		if !exists {
			// Check capacity before adding new subnet
			if len(t.data) >= t.config.MaxEntries {
				t.evictOldest()
			}
			backendMap = make(map[string]*BackendLatency)
			t.data[prefix] = backendMap
		}

		// Update backend latency
		// Key by backend|region to track each region's latency separately
		backendKey := backend + "|" + region
		entry := backendMap[backendKey]
		if entry == nil {
			entry = &BackendLatency{
				Backend: backend,
				Region:  region,
				Source:  agentID,
			}
			backendMap[backendKey] = entry
		}

		entry.EWMA = time.Duration(s.EWMA)
		entry.SampleCount = s.SampleCount
		entry.LastUpdated = now
		entry.Source = agentID
		entry.Region = region
		updated++
	}

	// Update metrics
	latencyTableEntries.Set(float64(t.countEntries()))

	t.logger.Debug("processed latency report",
		"agent_id", agentID,
		"backend", backend,
		"updated", updated,
	)
}

// GetBestBackend returns the lowest-latency healthy backend for a client.
// Returns nil if no learned latency data is available for the client's subnet.
func (t *LearnedLatencyTable) GetBestBackend(clientIP netip.Addr, healthy []string) *BackendLatency {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Find the subnet that covers this client IP
	// First try exact match, then broader prefixes
	var bestBackend *BackendLatency
	var lowestLatency time.Duration

	// Try common prefix lengths (/24 for IPv4, /48 for IPv6)
	var prefixBits int
	if clientIP.Is4() {
		prefixBits = 24
	} else {
		prefixBits = 48
	}

	prefix, err := clientIP.Prefix(prefixBits)
	if err != nil {
		return nil
	}

	backendMap, exists := t.data[prefix]
	if !exists {
		return nil
	}

	// Create a set of healthy backends for fast lookup
	healthySet := make(map[string]bool)
	for _, b := range healthy {
		healthySet[b] = true
	}

	// Find the lowest latency healthy backend
	// Note: backendKey is "backend|region", but we check against entry.Backend
	for _, entry := range backendMap {
		if !healthySet[entry.Backend] {
			continue
		}

		// Skip entries with insufficient samples
		if entry.SampleCount < uint64(t.config.MinSamples) {
			continue
		}

		// Skip stale entries
		if time.Since(entry.LastUpdated) > t.config.EntryTTL {
			continue
		}

		if bestBackend == nil || entry.EWMA < lowestLatency {
			bestBackend = entry
			lowestLatency = entry.EWMA
		}
	}

	return bestBackend
}

// GetLatencyForBackendInRegion returns the learned latency for a specific client->backend->region triple.
// This is used by the LearnedLatencyRouter to match latency data to specific servers.
func (t *LearnedLatencyTable) GetLatencyForBackendInRegion(clientIP netip.Addr, backend, region string) (*BackendLatency, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Determine prefix length based on IP version
	var prefixBits int
	if clientIP.Is4() {
		prefixBits = 24
	} else {
		prefixBits = 48
	}

	prefix, err := clientIP.Prefix(prefixBits)
	if err != nil {
		return nil, false
	}

	backendMap, exists := t.data[prefix]
	if !exists {
		return nil, false
	}

	// Look up by the combined backend|region key
	backendKey := backend + "|" + region
	entry, exists := backendMap[backendKey]
	if !exists {
		return nil, false
	}

	// Check for staleness
	if time.Since(entry.LastUpdated) > t.config.EntryTTL {
		return nil, false
	}

	// Return a copy to prevent external modification
	entryCopy := *entry
	return &entryCopy, true
}

// GetLatencyForBackend returns the lowest learned latency for a specific client->backend pair.
// Since latency is tracked per region, this finds the best region for the given backend.
func (t *LearnedLatencyTable) GetLatencyForBackend(clientIP netip.Addr, backend string) (*BackendLatency, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Determine prefix length based on IP version
	var prefixBits int
	if clientIP.Is4() {
		prefixBits = 24
	} else {
		prefixBits = 48
	}

	prefix, err := clientIP.Prefix(prefixBits)
	if err != nil {
		return nil, false
	}

	backendMap, exists := t.data[prefix]
	if !exists {
		return nil, false
	}

	// Find the lowest latency entry for this backend (across all regions)
	var bestEntry *BackendLatency
	for _, entry := range backendMap {
		if entry.Backend != backend {
			continue
		}

		// Check for staleness
		if time.Since(entry.LastUpdated) > t.config.EntryTTL {
			continue
		}

		if bestEntry == nil || entry.EWMA < bestEntry.EWMA {
			bestEntry = entry
		}
	}

	if bestEntry == nil {
		return nil, false
	}

	// Return a copy to prevent external modification
	entryCopy := *bestEntry
	return &entryCopy, true
}

// SubnetCount returns the number of tracked subnets.
func (t *LearnedLatencyTable) SubnetCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.data)
}

// Prune removes expired entries.
func (t *LearnedLatencyTable) Prune() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	var subnetsToPrune []netip.Prefix

	for prefix, backendMap := range t.data {
		var backendsToPrune []string

		for backend, entry := range backendMap {
			if now.Sub(entry.LastUpdated) > t.config.EntryTTL {
				backendsToPrune = append(backendsToPrune, backend)
			}
		}

		for _, backend := range backendsToPrune {
			delete(backendMap, backend)
		}

		// Remove subnet if no backends left
		if len(backendMap) == 0 {
			subnetsToPrune = append(subnetsToPrune, prefix)
		}
	}

	for _, prefix := range subnetsToPrune {
		delete(t.data, prefix)
		latencyEntriesPruned.Inc()
	}

	// Update metrics
	latencyTableEntries.Set(float64(t.countEntries()))

	if len(subnetsToPrune) > 0 {
		t.logger.Debug("pruned latency table",
			"subnets_removed", len(subnetsToPrune),
		)
	}
}

// evictOldest removes the oldest entry to make room for new ones.
// Must be called with lock held.
func (t *LearnedLatencyTable) evictOldest() {
	var oldestPrefix netip.Prefix
	var oldestTime time.Time
	first := true

	for prefix, backendMap := range t.data {
		for _, entry := range backendMap {
			if first || entry.LastUpdated.Before(oldestTime) {
				oldestPrefix = prefix
				oldestTime = entry.LastUpdated
				first = false
			}
		}
	}

	if !first {
		delete(t.data, oldestPrefix)
		latencyEntriesEvicted.Inc()
	}
}

// countEntries returns the total number of backend entries across all subnets.
func (t *LearnedLatencyTable) countEntries() int {
	count := 0
	for _, backendMap := range t.data {
		count += len(backendMap)
	}
	return count
}

// Clear removes all entries from the table.
func (t *LearnedLatencyTable) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = make(map[netip.Prefix]map[string]*BackendLatency)
	latencyTableEntries.Set(0)
}

// LatencyEntry is a single entry in the latency table for API responses.
type LatencyEntry struct {
	Subnet      string `json:"subnet"`
	Backend     string `json:"backend"`
	Region      string `json:"region"`
	EWMA        int64  `json:"ewma_ms"`
	SampleCount uint64 `json:"sample_count"`
	LastUpdated string `json:"last_updated"`
	Source      string `json:"source"`
}

// GetAllEntries returns all entries in the latency table.
// Used for API/debugging purposes.
func (t *LearnedLatencyTable) GetAllEntries() []LatencyEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var entries []LatencyEntry
	for prefix, backendMap := range t.data {
		for _, entry := range backendMap {
			entries = append(entries, LatencyEntry{
				Subnet:      prefix.String(),
				Backend:     entry.Backend,
				Region:      entry.Region,
				EWMA:        entry.EWMA.Milliseconds(),
				SampleCount: entry.SampleCount,
				LastUpdated: entry.LastUpdated.Format(time.RFC3339),
				Source:      entry.Source,
			})
		}
	}
	return entries
}
