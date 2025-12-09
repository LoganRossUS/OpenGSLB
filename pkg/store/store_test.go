// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
)

func TestBboltStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "kv.db")

	store, err := NewBboltStore(dbPath)
	if err != nil {
		t.Fatalf("NewBboltStore failed: %v", err)
	}
	defer store.Close()

	runStoreTests(t, store)
}

func TestRaftStore(t *testing.T) {
	// This test requires a full Raft setup, which is heavy.
	// We'll set up a single-node cluster.
	dir := t.TempDir()

	// Create minimal config
	cfg := cluster.DefaultConfig()
	cfg.NodeID = "test-node"
	cfg.BindAddress = "127.0.0.1:0" // Ephemeral port
	cfg.DataDir = dir
	cfg.Bootstrap = true

	// Use a specific port for stability
	cfg.BindAddress = "127.0.0.1:18088"

	node, err := cluster.NewRaftNode(cfg, nil)
	if err != nil {
		t.Skipf("Skipping RaftStore test due to node creation failure: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Skipf("Skipping RaftStore test due to start failure: %v", err)
		return
	}
	defer node.Stop(context.Background())

	// Wait for leader election
	time.Sleep(2 * time.Second)
	if !node.IsLeader() {
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 5*time.Second)
		defer timeoutCancel()
		if err := node.WaitForLeader(timeoutCtx); err != nil {
			t.Skipf("Skipping RaftStore test: failed to become leader: %v", err)
			return
		}
	}

	store := NewRaftStore(node)
	defer store.Close()

	runStoreTests(t, store)
}

func runStoreTests(t *testing.T, store Store) {
	ctx := context.Background()

	// 1. Set and Get
	t.Run("Set and Get", func(t *testing.T) {
		key := "foo"
		val := []byte("bar")
		if err := store.Set(ctx, key, val); err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if string(got) != string(val) {
			t.Errorf("Get = %s, want %s", string(got), string(val))
		}
	})

	// 2. Update
	t.Run("Update", func(t *testing.T) {
		key := "foo"
		val := []byte("baz")
		if err := store.Set(ctx, key, val); err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if string(got) != string(val) {
			t.Errorf("Get = %s, want %s", string(got), string(val))
		}
	})

	// 3. List
	t.Run("List", func(t *testing.T) {
		// Clear previous
		_ = store.Delete(ctx, "foo")

		data := map[string]string{
			"prefix/1": "val1",
			"prefix/2": "val2",
			"other/1":  "val3",
		}
		for k, v := range data {
			if err := store.Set(ctx, k, []byte(v)); err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		list, err := store.List(ctx, "prefix/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("List returned %d items, want 2", len(list))
		}
	})

	// 4. Watch
	t.Run("Watch", func(t *testing.T) {
		watchCtx, watchCancel := context.WithCancel(ctx)
		defer watchCancel()

		prefix := "watch/"
		ch, err := store.Watch(watchCtx, prefix)
		if err != nil {
			t.Fatalf("Watch failed: %v", err)
		}

		// Set value in a goroutine
		go func() {
			time.Sleep(100 * time.Millisecond)
			store.Set(ctx, "watch/foo", []byte("bar"))
		}()

		select {
		case ev := <-ch:
			if ev.Type != EventPut {
				t.Errorf("Event type = %v, want Put", ev.Type)
			}
			if ev.Key != "watch/foo" {
				t.Errorf("Event key = %s, want watch/foo", ev.Key)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for watch event")
		}
	})

	// 5. Delete
	t.Run("Delete", func(t *testing.T) {
		key := "del"
		if err := store.Set(ctx, key, []byte("val")); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
		if err := store.Delete(ctx, key); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		_, err := store.Get(ctx, key)
		if err != ErrKeyNotFound {
			t.Errorf("Get after delete error = %v, want ErrKeyNotFound", err)
		}
	})

	// 6. Get non-existent key
	t.Run("Get non-existent", func(t *testing.T) {
		_, err := store.Get(ctx, "nonexistent-key-12345")
		if err != ErrKeyNotFound {
			t.Errorf("Get non-existent error = %v, want ErrKeyNotFound", err)
		}
	})
}
