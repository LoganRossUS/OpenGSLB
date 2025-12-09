// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"context"
	"errors"
)

// EventType represents the type of change in a watch event.
type EventType string

const (
	// EventPut indicates a key was created or updated.
	EventPut EventType = "put"
	// EventDelete indicates a key was deleted.
	EventDelete EventType = "delete"
)

// WatchEvent represents a single change to the key-value store.
type WatchEvent struct {
	Type  EventType
	Key   string
	Value []byte
}

// KVPair represents a key-value pair.
type KVPair struct {
	Key   string
	Value []byte
}

// Store defines the interface for key-value storage operations.
type Store interface {
	// Get retrieves the value for the given key.
	// Returns ErrKeyNotFound if the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set sets the value for the given key.
	// In cluster mode, this may return ErrNotLeader if this node is not the leader.
	Set(ctx context.Context, key string, value []byte) error

	// Delete removes the given key.
	// It is not an error if the key does not exist.
	// In cluster mode, this may return ErrNotLeader if this node is not the leader.
	Delete(ctx context.Context, key string) error

	// List returns all key-value pairs where the key starts with the given prefix.
	List(ctx context.Context, prefix string) ([]KVPair, error)

	// Watch monitors keys with the given prefix for changes.
	// Returns a channel that receives events. The channel is closed when
	// the context is canceled or the store is closed.
	Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)

	// Close closes the store and releases resources.
	// After Close is called, all Watch channels will be closed.
	Close() error
}

// Common errors
var (
	ErrKeyNotFound = errors.New("key not found")
	ErrStopped     = errors.New("store stopped")
	ErrClosed      = errors.New("store closed")
	ErrNotLeader   = errors.New("not leader")
)
