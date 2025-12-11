// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package geo

import (
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// MatchType indicates how a region match was determined.
type MatchType string

const (
	MatchTypeCustomMapping MatchType = "custom_mapping"
	MatchTypeGeoIP         MatchType = "geoip"
	MatchTypeDefault       MatchType = "default"
)

// RegionMatch represents the result of resolving an IP to a region.
type RegionMatch struct {
	// Region is the matched region name
	Region string

	// MatchType indicates how the match was determined
	MatchType MatchType

	// Country is the ISO country code (only for GeoIP matches)
	Country string

	// Continent is the continent code (only for GeoIP matches)
	Continent string

	// MatchedCIDR is the matched CIDR (only for custom mapping matches)
	MatchedCIDR string

	// Comment is the mapping comment (only for custom mapping matches)
	Comment string
}

// RegionConfig defines geographic mapping for a region.
type RegionConfig struct {
	Name       string
	Countries  []string // ISO country codes
	Continents []string // Continent codes
}

// Resolver provides unified IP-to-region resolution using custom mappings
// and GeoIP database lookup.
type Resolver struct {
	mu                sync.RWMutex
	database          *Database
	customMappings    *CustomMappings
	regions           map[string]*RegionConfig
	countryToRegion   map[string]string // country code -> region name
	continentToRegion map[string]string // continent code -> region name
	defaultRegion     string
	logger            *slog.Logger
}

// ResolverConfig contains configuration for creating a Resolver.
type ResolverConfig struct {
	DatabasePath   string
	DefaultRegion  string
	CustomMappings []config.CustomMapping
	Regions        []config.Region
	Logger         *slog.Logger
}

// NewResolver creates a new geolocation Resolver.
func NewResolver(cfg ResolverConfig) (*Resolver, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Load GeoIP database
	db, err := NewDatabase(cfg.DatabasePath, logger)
	if err != nil {
		return nil, err
	}

	// Initialize custom mappings
	custom := NewCustomMappings(logger)
	var customMappings []CustomMapping
	for _, m := range cfg.CustomMappings {
		customMappings = append(customMappings, CustomMapping{
			CIDR:    m.CIDR,
			Region:  m.Region,
			Comment: m.Comment,
			Source:  "config",
		})
	}
	if len(customMappings) > 0 {
		if err := custom.LoadFromConfig(customMappings); err != nil {
			db.Close()
			return nil, err
		}
	}

	resolver := &Resolver{
		database:          db,
		customMappings:    custom,
		regions:           make(map[string]*RegionConfig),
		countryToRegion:   make(map[string]string),
		continentToRegion: make(map[string]string),
		defaultRegion:     cfg.DefaultRegion,
		logger:            logger,
	}

	// Build region mappings
	resolver.loadRegions(cfg.Regions)

	return resolver, nil
}

// loadRegions builds the country/continent to region lookup maps.
func (r *Resolver) loadRegions(regions []config.Region) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.regions = make(map[string]*RegionConfig)
	r.countryToRegion = make(map[string]string)
	r.continentToRegion = make(map[string]string)

	for _, region := range regions {
		cfg := &RegionConfig{
			Name:       region.Name,
			Countries:  region.Countries,
			Continents: region.Continents,
		}
		r.regions[region.Name] = cfg

		// Map countries to this region
		for _, country := range region.Countries {
			r.countryToRegion[strings.ToUpper(country)] = region.Name
		}

		// Map continents to this region
		for _, continent := range region.Continents {
			r.continentToRegion[strings.ToUpper(continent)] = region.Name
		}
	}

	r.logger.Info("region mappings loaded",
		"regions", len(r.regions),
		"countries", len(r.countryToRegion),
		"continents", len(r.continentToRegion),
	)
}

// ReloadRegions reloads region configurations.
func (r *Resolver) ReloadRegions(regions []config.Region) {
	r.loadRegions(regions)
}

// Resolve determines the appropriate region for an IP address.
// Resolution order:
// 1. Custom CIDR mappings (longest prefix match)
// 2. GeoIP database (country match takes precedence over continent)
// 3. Default region
func (r *Resolver) Resolve(ip net.IP) *RegionMatch {
	// 1. Check custom mappings first
	if result := r.customMappings.Lookup(ip); result.Found {
		return &RegionMatch{
			Region:      result.Region,
			MatchType:   MatchTypeCustomMapping,
			MatchedCIDR: result.CIDR,
			Comment:     result.Comment,
		}
	}

	// 2. Check GeoIP database
	r.mu.RLock()
	defer r.mu.RUnlock()

	geoResult, err := r.database.Lookup(ip)
	if err == nil && geoResult.Found {
		// Try country match first (more specific)
		if geoResult.Country != "" {
			if region, ok := r.countryToRegion[strings.ToUpper(geoResult.Country)]; ok {
				return &RegionMatch{
					Region:    region,
					MatchType: MatchTypeGeoIP,
					Country:   geoResult.Country,
					Continent: geoResult.Continent,
				}
			}
		}

		// Fall back to continent match
		if geoResult.Continent != "" {
			if region, ok := r.continentToRegion[strings.ToUpper(geoResult.Continent)]; ok {
				return &RegionMatch{
					Region:    region,
					MatchType: MatchTypeGeoIP,
					Country:   geoResult.Country,
					Continent: geoResult.Continent,
				}
			}
		}
	}

	// 3. Fall back to default region
	return &RegionMatch{
		Region:    r.defaultRegion,
		MatchType: MatchTypeDefault,
	}
}

// TestIP returns detailed resolution information for an IP address.
// This is useful for debugging and the API test endpoint.
func (r *Resolver) TestIP(ip net.IP) *RegionMatch {
	return r.Resolve(ip)
}

// GetCustomMappings returns the custom mappings manager.
func (r *Resolver) GetCustomMappings() *CustomMappings {
	return r.customMappings
}

// ReloadDatabase reloads the GeoIP database if it has changed.
func (r *Resolver) ReloadDatabase() (bool, error) {
	return r.database.Reload()
}

// ReloadCustomMappings reloads custom mappings from configuration.
func (r *Resolver) ReloadCustomMappings(mappings []config.CustomMapping) error {
	var customMappings []CustomMapping
	for _, m := range mappings {
		customMappings = append(customMappings, CustomMapping{
			CIDR:    m.CIDR,
			Region:  m.Region,
			Comment: m.Comment,
			Source:  "config",
		})
	}
	return r.customMappings.LoadFromConfig(customMappings)
}

// Close closes the resolver and releases resources.
func (r *Resolver) Close() error {
	return r.database.Close()
}

// DefaultRegion returns the configured default region.
func (r *Resolver) DefaultRegion() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultRegion
}

// GetRegion returns the configuration for a specific region.
func (r *Resolver) GetRegion(name string) (*RegionConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.regions[name]
	if !ok {
		return nil, false
	}
	return &RegionConfig{
		Name:       cfg.Name,
		Countries:  append([]string{}, cfg.Countries...),
		Continents: append([]string{}, cfg.Continents...),
	}, true
}

// ListRegions returns all configured region names.
func (r *Resolver) ListRegions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.regions))
	for name := range r.regions {
		names = append(names, name)
	}
	return names
}
