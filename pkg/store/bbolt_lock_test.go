// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A second open of a store that another process (here: another handle) holds
// must fail with an error that says so. Without a lock timeout it blocked
// forever, so an overwatch started on a data directory in use hung at startup
// with nothing in the log.
func TestBboltStore_OpenLockedFileFails(t *testing.T) {
	old := openLockTimeout
	openLockTimeout = 200 * time.Millisecond
	t.Cleanup(func() { openLockTimeout = old })

	path := filepath.Join(t.TempDir(), "locked.db")
	first, err := NewBboltStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer first.Close()

	type result struct {
		store *BboltStore
		err   error
	}
	done := make(chan result, 1)
	go func() {
		s, err := NewBboltStore(path)
		done <- result{s, err}
	}()

	select {
	case r := <-done:
		if r.err == nil {
			r.store.Close()
			t.Fatal("second open of a locked store succeeded")
		}
		if !strings.Contains(r.err.Error(), "locked by another process") || !strings.Contains(r.err.Error(), path) {
			t.Errorf("error does not say that %s is locked: %v", path, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second open of a locked store is still waiting after 5s; it must give up and report the lock")
	}

	// Once the first handle is closed the store opens again.
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	again, err := NewBboltStore(path)
	if err != nil {
		t.Fatalf("open after close: %v", err)
	}
	again.Close()
}
