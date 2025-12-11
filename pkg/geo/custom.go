// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package geo

import (
	"fmt"
	"log/slog"
	"net"
	"sort"
	"sync"

	"github.com/yl2chen/cidranger"
)

// CustomMapping represents a CIDR-to-region mapping.
type CustomMapping struct {
	CIDR    string
	Region  string
	Comment string
	Source  string // "config" or "api"
}

// regionEntry implements cidranger.RangerEntry for storing region mappings.
type regionEntry struct {
	network net.IPNet
	region  string
	comment string
	source  string
}

func (e regionEntry) Network() net.IPNet {
	return e.network
}

// CustomMappings manages CIDR-to-region mappings using a radix tree for efficient
// longest-prefix matching.
type CustomMappings struct {
	mu       sync.RWMutex
	ranger   cidranger.Ranger
	mappings map[string]*CustomMapping // key is CIDR string
	logger   *slog.Logger
}

// NewCustomMappings creates a new CustomMappings instance.
func NewCustomMappings(logger *slog.Logger) *CustomMappings {
	if logger == nil {
		logger = slog.Default()
	}

	return &CustomMappings{
		ranger:   cidranger.NewPCTrieRanger(),
		mappings: make(map[string]*CustomMapping),
		logger:   logger,
	}
}

// LoadFromConfig loads custom mappings from configuration.
func (c *CustomMappings) LoadFromConfig(mappings []CustomMapping) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create new ranger and mappings map
	newRanger := cidranger.NewPCTrieRanger()
	newMappings := make(map[string]*CustomMapping)

	for _, m := range mappings {
		_, network, err := net.ParseCIDR(m.CIDR)
		if err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", m.CIDR, err)
		}

		entry := regionEntry{
			network: *network,
			region:  m.Region,
			comment: m.Comment,
			source:  "config",
		}

		if err := newRanger.Insert(entry); err != nil {
			return fmt.Errorf("failed to insert CIDR %q: %w", m.CIDR, err)
		}

		mapping := &CustomMapping{
			CIDR:    m.CIDR,
			Region:  m.Region,
			Comment: m.Comment,
			Source:  "config",
		}
		newMappings[m.CIDR] = mapping
	}

	c.ranger = newRanger
	c.mappings = newMappings

	c.logger.Info("custom CIDR mappings loaded", "count", len(mappings))
	return nil
}

// Add adds or updates a custom mapping.
// If the mapping already exists, it will be updated.
func (c *CustomMappings) Add(mapping CustomMapping) error {
	_, network, err := net.ParseCIDR(mapping.CIDR)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", mapping.CIDR, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove existing entry if present
	if _, exists := c.mappings[mapping.CIDR]; exists {
		_, _ = c.ranger.Remove(*network)
	}

	entry := regionEntry{
		network: *network,
		region:  mapping.Region,
		comment: mapping.Comment,
		source:  mapping.Source,
	}

	if err := c.ranger.Insert(entry); err != nil {
		return fmt.Errorf("failed to insert CIDR %q: %w", mapping.CIDR, err)
	}

	c.mappings[mapping.CIDR] = &CustomMapping{
		CIDR:    mapping.CIDR,
		Region:  mapping.Region,
		Comment: mapping.Comment,
		Source:  mapping.Source,
	}

	c.logger.Debug("custom mapping added",
		"cidr", mapping.CIDR,
		"region", mapping.Region,
		"source", mapping.Source,
	)

	return nil
}

// Remove removes a custom mapping by CIDR.
func (c *CustomMappings) Remove(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.mappings[cidr]; !exists {
		return fmt.Errorf("mapping for CIDR %q not found", cidr)
	}

	_, err = c.ranger.Remove(*network)
	if err != nil {
		return fmt.Errorf("failed to remove CIDR %q: %w", cidr, err)
	}

	delete(c.mappings, cidr)

	c.logger.Debug("custom mapping removed", "cidr", cidr)
	return nil
}

// LookupResult contains the result of a custom mapping lookup.
type CustomLookupResult struct {
	// Region is the matched region
	Region string

	// CIDR is the matched CIDR
	CIDR string

	// Comment is the optional comment
	Comment string

	// Source indicates where the mapping came from
	Source string

	// Found indicates whether a match was found
	Found bool
}

// Lookup finds the most specific (longest prefix) custom mapping for an IP.
func (c *CustomMappings) Lookup(ip net.IP) *CustomLookupResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := c.ranger.ContainingNetworks(ip)
	if err != nil || len(entries) == 0 {
		return &CustomLookupResult{Found: false}
	}

	// cidranger returns entries in order from least specific to most specific,
	// so we want the last entry (most specific / longest prefix match)
	mostSpecific, ok := entries[len(entries)-1].(regionEntry)
	if !ok {
		return &CustomLookupResult{Found: false}
	}

	return &CustomLookupResult{
		Region:  mostSpecific.region,
		CIDR:    mostSpecific.network.String(),
		Comment: mostSpecific.comment,
		Source:  mostSpecific.source,
		Found:   true,
	}
}

// List returns all custom mappings.
func (c *CustomMappings) List() []*CustomMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*CustomMapping, 0, len(c.mappings))
	for _, m := range c.mappings {
		// Return a copy to prevent external modification
		result = append(result, &CustomMapping{
			CIDR:    m.CIDR,
			Region:  m.Region,
			Comment: m.Comment,
			Source:  m.Source,
		})
	}

	// Sort by CIDR for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].CIDR < result[j].CIDR
	})

	return result
}

// Get retrieves a specific mapping by CIDR.
func (c *CustomMappings) Get(cidr string) (*CustomMapping, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	m, exists := c.mappings[cidr]
	if !exists {
		return nil, false
	}

	return &CustomMapping{
		CIDR:    m.CIDR,
		Region:  m.Region,
		Comment: m.Comment,
		Source:  m.Source,
	}, true
}

// Count returns the number of custom mappings.
func (c *CustomMappings) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.mappings)
}

// Clear removes all custom mappings.
func (c *CustomMappings) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ranger = cidranger.NewPCTrieRanger()
	c.mappings = make(map[string]*CustomMapping)

	c.logger.Debug("custom mappings cleared")
}
