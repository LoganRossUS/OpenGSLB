// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ADR-015: Removed Raft store tests - only bbolt store remains

func TestBboltStore_GetSet(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test Set
	err = store.Set(ctx, "test-key", []byte("test-value"))
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	// Test Get
	val, err := store.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if string(val) != "test-value" {
		t.Errorf("expected 'test-value', got '%s'", string(val))
	}
}

func TestBboltStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test Get non-existent key - should return ErrKeyNotFound
	_, err = store.Get(ctx, "non-existent")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestBboltStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Set a value
	err = store.Set(ctx, "delete-key", []byte("to-be-deleted"))
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	// Verify it exists
	val, err := store.Get(ctx, "delete-key")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if val == nil {
		t.Fatal("key should exist before delete")
	}

	// Delete
	err = store.Delete(ctx, "delete-key")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Verify it's gone
	_, err = store.Get(ctx, "delete-key")
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}

func TestBboltStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Set multiple keys with prefix
	testKeys := []string{
		"servers/us-east/server1",
		"servers/us-east/server2",
		"servers/us-west/server1",
		"domains/example.com",
	}

	for _, key := range testKeys {
		err = store.Set(ctx, key, []byte("value"))
		if err != nil {
			t.Fatalf("failed to set %s: %v", key, err)
		}
	}

	// List with prefix
	pairs, err := store.List(ctx, "servers/us-east/")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(pairs) != 2 {
		t.Errorf("expected 2 pairs with prefix 'servers/us-east/', got %d", len(pairs))
	}

	// List all servers
	pairs, err = store.List(ctx, "servers/")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	if len(pairs) != 3 {
		t.Errorf("expected 3 pairs with prefix 'servers/', got %d", len(pairs))
	}
}

func TestBboltStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store and write data
	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	err = store.Set(ctx, "persistent-key", []byte("persistent-value"))
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	// Close store
	err = store.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// Reopen store
	store2, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Verify data persisted
	val, err := store2.Get(ctx, "persistent-key")
	if err != nil {
		t.Fatalf("failed to get after reopen: %v", err)
	}
	if string(val) != "persistent-value" {
		t.Errorf("expected 'persistent-value', got '%s'", string(val))
	}
}

func TestBboltStore_Watch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	watchCh, err := store.Watch(ctx, "watch/")
	if err != nil {
		t.Fatalf("failed to watch: %v", err)
	}

	// Set a value that matches the prefix
	go func() {
		time.Sleep(50 * time.Millisecond)
		store.Set(context.Background(), "watch/key1", []byte("value1"))
	}()

	// Wait for event
	select {
	case event := <-watchCh:
		if event.Type != EventPut {
			t.Errorf("expected EventPut, got %v", event.Type)
		}
		if event.Key != "watch/key1" {
			t.Errorf("expected key 'watch/key1', got '%s'", event.Key)
		}
		if string(event.Value) != "value1" {
			t.Errorf("expected value 'value1', got '%s'", string(event.Value))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for watch event")
	}
}

func TestFactory_BBolt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Type: StoreBBolt,
		Path: dbPath,
	}

	store, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create store via factory: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.Set(ctx, "factory-key", []byte("factory-value"))
	if err != nil {
		t.Fatalf("failed to set via factory-created store: %v", err)
	}
}

func TestFactory_DefaultToBBolt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := Config{
		Type: "", // Empty should default to bbolt
		Path: dbPath,
	}

	store, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create store with empty type: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.Set(ctx, "default-key", []byte("default-value"))
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}
}

func TestFactory_UnsupportedType(t *testing.T) {
	cfg := Config{
		Type: "unsupported",
		Path: "/tmp/test.db",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for unsupported store type")
	}
}

func TestBboltStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	done := make(chan bool)

	// Concurrent writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := "concurrent-key"
				val := []byte("value")
				_ = store.Set(ctx, key, val)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("timeout waiting for concurrent operations")
		}
	}
}

func TestBboltStore_LargeValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create 1MB value
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	err = store.Set(ctx, "large-key", largeValue)
	if err != nil {
		t.Fatalf("failed to set large value: %v", err)
	}

	val, err := store.Get(ctx, "large-key")
	if err != nil {
		t.Fatalf("failed to get large value: %v", err)
	}

	if len(val) != len(largeValue) {
		t.Errorf("expected %d bytes, got %d", len(largeValue), len(val))
	}
}

func TestBboltStore_SpecialCharactersInKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	specialKeys := []string{
		"key/with/slashes",
		"key.with.dots",
		"key-with-dashes",
		"key_with_underscores",
		"key:with:colons",
	}

	for _, key := range specialKeys {
		err = store.Set(ctx, key, []byte("value"))
		if err != nil {
			t.Errorf("failed to set key '%s': %v", key, err)
			continue
		}

		val, err := store.Get(ctx, key)
		if err != nil {
			t.Errorf("failed to get key '%s': %v", key, err)
			continue
		}

		if string(val) != "value" {
			t.Errorf("wrong value for key '%s': expected 'value', got '%s'", key, string(val))
		}
	}
}

func TestBboltStore_FilePermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	tmpDir := t.TempDir()

	// Make directory read-only
	err := os.Chmod(tmpDir, 0444)
	if err != nil {
		t.Fatalf("failed to change permissions: %v", err)
	}
	defer os.Chmod(tmpDir, 0755)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = NewBboltStore(dbPath)
	if err == nil {
		t.Error("expected error creating store in read-only directory")
	}
}
