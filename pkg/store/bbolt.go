// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketName = []byte("opengslb")
)

// BboltStore implements Store using BoltDB.
type BboltStore struct {
	db       *bolt.DB
	watchers map[string][]chan WatchEvent
	mu       sync.RWMutex
	closed   bool
}

// NewBboltStore creates a new BboltStore.
func NewBboltStore(path string) (*BboltStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt db: %w", err)
	}

	// Initialize bucket
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	return &BboltStore{
		db:       db,
		watchers: make(map[string][]chan WatchEvent),
	}, nil
}

// Get retrieves the value for the given key.
func (s *BboltStore) Get(ctx context.Context, key string) ([]byte, error) {
	var val []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		v := b.Get([]byte(key))
		if v == nil {
			return ErrKeyNotFound
		}
		// Copy value to be safe outside transaction
		val = make([]byte, len(v))
		copy(val, v)
		return nil
	})
	return val, err
}

// Set sets the value for the given key.
func (s *BboltStore) Set(ctx context.Context, key string, value []byte) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.Put([]byte(key), value)
	})
	if err != nil {
		return err
	}

	s.notifyWatchers(EventPut, key, value)
	return nil
}

// Delete removes the given key.
func (s *BboltStore) Delete(ctx context.Context, key string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.Delete([]byte(key))
	})
	if err != nil {
		return err
	}

	s.notifyWatchers(EventDelete, key, nil)
	return nil
}

// List returns all key-value pairs where the key starts with the given prefix.
func (s *BboltStore) List(ctx context.Context, prefix string) ([]KVPair, error) {
	var pairs []KVPair
	prefixBytes := []byte(prefix)

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		c := b.Cursor()

		for k, v := c.Seek(prefixBytes); k != nil && bytes.HasPrefix(k, prefixBytes); k, v = c.Next() {
			val := make([]byte, len(v))
			copy(val, v)
			pairs = append(pairs, KVPair{
				Key:   string(k),
				Value: val,
			})
		}
		return nil
	})
	return pairs, err
}

// Watch monitors keys with the given prefix for changes.
func (s *BboltStore) Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, ErrClosed
	}

	ch := make(chan WatchEvent, 10)
	s.watchers[prefix] = append(s.watchers[prefix], ch)

	// Handle context cancellation to remove watcher
	go func() {
		<-ctx.Done()
		s.removeWatcher(prefix, ch)
	}()

	return ch, nil
}

func (s *BboltStore) removeWatcher(prefix string, ch chan WatchEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels := s.watchers[prefix]
	for i, c := range channels {
		if c == ch {
			// Remove element
			s.watchers[prefix] = append(channels[:i], channels[i+1:]...)
			close(ch)
			break
		}
	}
	if len(s.watchers[prefix]) == 0 {
		delete(s.watchers, prefix)
	}
}

func (s *BboltStore) notifyWatchers(eventType EventType, key string, value []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
					// Drop event if channel is full to prevent blocking
				}
			}
		}
	}
}

// Close closes the store.
func (s *BboltStore) Close() error {
	s.mu.Lock()
	s.closed = true
	// Close all watcher channels
	for _, channels := range s.watchers {
		for _, ch := range channels {
			close(ch)
		}
	}
	s.watchers = nil
	s.mu.Unlock()

	return s.db.Close()
}
