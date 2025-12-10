// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"fmt"
)

// StoreType identifies the type of store backend.
type StoreType string

const (
	// StoreBBolt uses embedded bbolt for local persistence.
	StoreBBolt StoreType = "bbolt"
)

// Config holds configuration for creating a store.
type Config struct {
	// Type specifies the store backend type.
	// ADR-015: Only "bbolt" is supported (Raft store removed).
	Type StoreType

	// Path is the file path for bbolt database.
	Path string
}

// New creates a new store based on the provided configuration.
// ADR-015: Only bbolt store is supported. Raft store was removed.
func New(cfg Config) (Store, error) {
	switch cfg.Type {
	case StoreBBolt, "": // Empty defaults to bbolt
		return NewBboltStore(cfg.Path)
	default:
		return nil, fmt.Errorf("unsupported store type: %s (only 'bbolt' is supported)", cfg.Type)
	}
}
