// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"fmt"
	"log/slog"
)

// StoreType defines the type of KV store to use.
type StoreType string

const (
	// TypeBBolt uses the embedded bbolt database.
	// This is the only store type in ADR-015 architecture.
	TypeBBolt StoreType = "bbolt"
)

// Config holds configuration for creating a store.
type Config struct {
	// Type specifies the store implementation.
	// Only "bbolt" is supported in ADR-015 architecture.
	Type StoreType

	// Path is the filesystem path for the database file.
	// Required for bbolt.
	Path string

	// Logger is used for store operations logging.
	Logger *slog.Logger
}

// New creates a new Store based on the configuration.
// ADR-015: Only bbolt is supported. Raft-replicated store was removed.
func New(cfg Config) (Store, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	switch cfg.Type {
	case TypeBBolt, "": // Default to bbolt
		if cfg.Path == "" {
			return nil, fmt.Errorf("path is required for bbolt store")
		}
		return NewBBoltStore(cfg.Path)
	default:
		return nil, fmt.Errorf("unsupported store type: %s (only 'bbolt' is supported)", cfg.Type)
	}
}

// NewDefault creates a bbolt store at the default path.
func NewDefault(path string) (Store, error) {
	return New(Config{
		Type: TypeBBolt,
		Path: path,
	})
}