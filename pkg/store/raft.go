// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"context"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

// RaftStore implements Store using the Raft cluster.
type RaftStore struct {
	node     *cluster.RaftNode
	watchers map[string][]chan WatchEvent
	mu       sync.RWMutex
	closed   bool
}

// NewRaftStore creates a new RaftStore.
func NewRaftStore(node *cluster.RaftNode) *RaftStore {
	store := &RaftStore{
		node:     node,
		watchers: make(map[string][]chan WatchEvent),
	}

	// Subscribe to FSM updates
	node.FSM().AddWatcher(store.handleFSMEvent)
	return store
}

// Get retrieves the value for the given key.
// Note: This performs a local read from the FSM. Consistency depends on Raft state.
// For strong consistency, one might want to use Barrier(), but straightforward Get is usually acceptable for extensive reads.
func (s *RaftStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, exists := s.node.FSM().Get(key)
	if !exists {
		return nil, ErrKeyNotFound
	}
	return val, nil
}

// Set sets the value for the given key.
func (s *RaftStore) Set(ctx context.Context, key string, value []byte) error {
	return s.node.ApplyCommand(ctx, cluster.CommandSet, key, value)
}

// Delete removes the given key.
func (s *RaftStore) Delete(ctx context.Context, key string) error {
	return s.node.ApplyCommand(ctx, cluster.CommandDelete, key, nil)
}

// List returns all key-value pairs where the key starts with the given prefix.
func (s *RaftStore) List(ctx context.Context, prefix string) ([]KVPair, error) {
	data := s.node.FSM().List(prefix)
	pairs := make([]KVPair, 0, len(data))
	for k, v := range data {
		pairs = append(pairs, KVPair{
			Key:   k,
			Value: v,
		})
	}
	return pairs, nil
}

// Watch monitors keys with the given prefix for changes.
func (s *RaftStore) Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrClosed
	}

	ch := make(chan WatchEvent, 10)
	s.watchers[prefix] = append(s.watchers[prefix], ch)

	go func() {
		<-ctx.Done()
		s.removeWatcher(prefix, ch)
	}()

	return ch, nil
}

func (s *RaftStore) removeWatcher(prefix string, ch chan WatchEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels := s.watchers[prefix]
	for i, c := range channels {
		if c == ch {
			s.watchers[prefix] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}
	if len(s.watchers[prefix]) == 0 {
		delete(s.watchers, prefix)
	}
}

// handleFSMEvent is the callback from the Raft FSM.
func (s *RaftStore) handleFSMEvent(key string, value []byte, isDelete bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	eventType := EventPut
	if isDelete {
		eventType = EventDelete
	}

	for prefix, channels := range s.watchers {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			event := WatchEvent{
				Type:  eventType,
				Key:   key,
				Value: value,
			}
			for _, ch := range channels {
				select {
				case ch <- event:
				default:
				}
			}
		}
	}
}

// Close closes the store.
func (s *RaftStore) Close() error {
	s.mu.Lock()
	s.closed = true
	for _, channels := range s.watchers {
		for _, ch := range channels {
			close(ch)
		}
	}
	s.watchers = nil
	s.mu.Unlock()
	return nil
}
