// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"fmt"
	"path/filepath"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

// StoreType identifies the type of store to use.
type StoreType string

const (
	StoreTypeBbolt StoreType = "bbolt"
	StoreTypeRaft  StoreType = "raft"
)

// Config holds configuration for the store factory.
type Config struct {
	Type     StoreType
	DataDir  string
	RaftNode *cluster.RaftNode
}

// NewStore creates a new store based on the configuration.
func NewStore(cfg Config) (Store, error) {
	switch cfg.Type {
	case StoreTypeBbolt:
		if cfg.DataDir == "" {
			return nil, fmt.Errorf("DataDir is required for bbolt store")
		}
		dbPath := filepath.Join(cfg.DataDir, "kv.db")
		return NewBboltStore(dbPath)

	case StoreTypeRaft:
		if cfg.RaftNode == nil {
			return nil, fmt.Errorf("RaftNode is required for raft store")
		}
		return NewRaftStore(cfg.RaftNode), nil

	default:
		return nil, fmt.Errorf("unknown store type: %s", cfg.Type)
	}
}
