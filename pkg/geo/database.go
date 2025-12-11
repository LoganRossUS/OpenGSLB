// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package geo provides geolocation functionality for IP-based routing.
package geo

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// Database provides GeoIP lookup functionality with hot-reload support.
type Database struct {
	mu      sync.RWMutex
	reader  *geoip2.Reader
	path    string
	logger  *slog.Logger
	modTime int64
}

// NewDatabase creates a new GeoIP database instance.
// The database is loaded from the specified path.
func NewDatabase(path string, logger *slog.Logger) (*Database, error) {
	if logger == nil {
		logger = slog.Default()
	}

	db := &Database{
		path:   path,
		logger: logger,
	}

	if err := db.load(); err != nil {
		return nil, err
	}

	return db, nil
}

// load loads the GeoIP database from disk.
func (d *Database) load() error {
	info, err := os.Stat(d.path)
	if err != nil {
		return fmt.Errorf("failed to stat GeoIP database %q: %w", d.path, err)
	}

	reader, err := geoip2.Open(d.path)
	if err != nil {
		return fmt.Errorf("failed to open GeoIP database %q: %w", d.path, err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Close old reader if exists
	if d.reader != nil {
		d.reader.Close()
	}

	d.reader = reader
	d.modTime = info.ModTime().Unix()

	d.logger.Info("GeoIP database loaded",
		"path", d.path,
		"type", reader.Metadata().DatabaseType,
	)

	return nil
}

// Reload reloads the database from disk if it has changed.
// Returns true if the database was reloaded, false if unchanged.
func (d *Database) Reload() (bool, error) {
	info, err := os.Stat(d.path)
	if err != nil {
		return false, fmt.Errorf("failed to stat GeoIP database: %w", err)
	}

	d.mu.RLock()
	unchanged := info.ModTime().Unix() == d.modTime
	d.mu.RUnlock()

	if unchanged {
		return false, nil
	}

	if err := d.load(); err != nil {
		return false, err
	}

	d.logger.Info("GeoIP database reloaded", "path", d.path)
	return true, nil
}

// LookupResult contains the result of a GeoIP lookup.
type LookupResult struct {
	// Country is the ISO 3166-1 alpha-2 country code (e.g., "US")
	Country string

	// Continent is the continent code (e.g., "NA" for North America)
	Continent string

	// Found indicates whether the lookup was successful
	Found bool
}

// Lookup performs a GeoIP lookup for the given IP address.
// Returns the country and continent codes.
func (d *Database) Lookup(ip net.IP) (*LookupResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.reader == nil {
		return &LookupResult{Found: false}, fmt.Errorf("GeoIP database not loaded")
	}

	record, err := d.reader.Country(ip)
	if err != nil {
		return &LookupResult{Found: false}, err
	}

	return &LookupResult{
		Country:   record.Country.IsoCode,
		Continent: record.Continent.Code,
		Found:     record.Country.IsoCode != "" || record.Continent.Code != "",
	}, nil
}

// Close closes the GeoIP database.
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.reader != nil {
		err := d.reader.Close()
		d.reader = nil
		return err
	}
	return nil
}

// Path returns the database file path.
func (d *Database) Path() string {
	return d.path
}

// DatabaseType returns the database type string (e.g., "GeoLite2-Country").
func (d *Database) DatabaseType() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.reader == nil {
		return ""
	}
	return d.reader.Metadata().DatabaseType
}
