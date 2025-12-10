// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package store provides embedded key-value storage for OpenGSLB runtime state.
//
// The store is used for:
//   - Agent registrations received via gossip
//   - Health state persistence
//   - Weight overrides set via API
//   - DNSSEC keys (in Overwatch mode)
//   - Pinned agent certificates (TOFU)
//
// Each Overwatch node maintains its own independent store (bbolt).
// There is no cross-node replication - Overwatches operate independently
// per ADR-015.
package store

import (
	"context"
	"errors"
)

// Common errors returned by store implementations.
var (
	// ErrKeyNotFound is returned when a requested key doesn't exist.
	ErrKeyNotFound = errors.New("key not found")

	// ErrClosed is returned when operations are attempted on a closed store.
	ErrClosed = errors.New("store is closed")
)

// EventType represents the type of watch event.
type EventType int

const (
	// EventPut indicates a key was created or updated.
	EventPut EventType = iota
	// EventDelete indicates a key was deleted.
	EventDelete
)

// WatchEvent represents a change to a watched key.
type WatchEvent struct {
	Type  EventType
	Key   string
	Value []byte // nil for delete events
}

// KVPair represents a key-value pair returned by List operations.
type KVPair struct {
	Key   string
	Value []byte
}

// Store defines the interface for key-value storage operations.
// Implementations must be safe for concurrent use.
type Store interface {
	// Get retrieves a value by key.
	// Returns ErrKeyNotFound if the key doesn't exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value by key.
	Set(ctx context.Context, key string, value []byte) error

	// Delete removes a key.
	// Returns nil if the key doesn't exist (idempotent).
	Delete(ctx context.Context, key string) error

	// List returns all key-value pairs with keys starting with prefix.
	// Returns empty slice if no keys match.
	List(ctx context.Context, prefix string) ([]KVPair, error)

	// Watch monitors keys with the given prefix for changes.
	// Returns a channel that receives events until context is canceled.
	Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)

	// Close closes the store and releases resources.
	Close() error
}

// Well-known key prefixes used throughout OpenGSLB.
const (
	// PrefixAgents is the prefix for agent registration data.
	// Key format: "agents/{agent_id}"
	PrefixAgents = "agents/"

	// PrefixBackends is the prefix for backend health state.
	// Key format: "backends/{service}/{address}:{port}"
	PrefixBackends = "backends/"

	// PrefixOverrides is the prefix for health overrides set via API.
	// Key format: "overrides/{service}/{address}:{port}"
	PrefixOverrides = "overrides/"

	// PrefixDNSSEC is the prefix for DNSSEC keys.
	// Key format: "dnssec/{zone}"
	PrefixDNSSEC = "dnssec/"

	// PrefixPinnedCerts is the prefix for TOFU-pinned agent certificates.
	// Key format: "pinned_certs/{agent_id}"
	PrefixPinnedCerts = "pinned_certs/"
)
