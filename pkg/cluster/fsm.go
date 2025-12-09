// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/hashicorp/raft"
)

// CommandType identifies the type of FSM command.
type CommandType string

const (
	CommandSet    CommandType = "set"
	CommandDelete CommandType = "delete"
)

// Command represents a state machine command.
type Command struct {
	Type  CommandType `json:"type"`
	Key   string      `json:"key"`
	Value []byte      `json:"value,omitempty"`
}

// FSM implements the raft.FSM interface for OpenGSLB.
// It provides a simple key-value store that can be used for
// distributed configuration and runtime state.
type FSM struct {
	mu       sync.RWMutex
	data     map[string][]byte
	watchers []FSMWatcher
	logger   *slog.Logger
}

// NewFSM creates a new FSM instance.
func NewFSM(logger *slog.Logger) *FSM {
	if logger == nil {
		logger = slog.Default()
	}
	return &FSM{
		data:   make(map[string][]byte),
		logger: logger,
	}
}

// Apply applies a Raft log entry to the FSM.
func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		f.logger.Error("failed to unmarshal command", "error", err)
		return fmt.Errorf("failed to unmarshal command: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Type {
	case CommandSet:
		f.data[cmd.Key] = cmd.Value
		f.logger.Debug("fsm set", "key", cmd.Key, "value_len", len(cmd.Value))
		f.notifyWatchersLocked(cmd.Key, cmd.Value, false)
	case CommandDelete:
		delete(f.data, cmd.Key)
		f.logger.Debug("fsm delete", "key", cmd.Key)
		f.notifyWatchersLocked(cmd.Key, nil, true)
	default:
		f.logger.Warn("unknown command type", "type", cmd.Type)
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}

	return nil
}

// notifyWatchersLocked notifies all registered watchers of a change.
// Watchers are invoked synchronously but should be implemented to be fast
// (e.g., by using buffered channels internally).
// MUST be called with f.mu held.
func (f *FSM) notifyWatchersLocked(key string, value []byte, isDelete bool) {
	for _, w := range f.watchers {
		// Invoke callback - watchers MUST be non-blocking
		// The RaftStore watcher uses a buffered channel with non-blocking send
		w(key, value, isDelete)
	}
}

// Snapshot returns an FSMSnapshot for creating a point-in-time snapshot.
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Create a deep copy of the data
	dataCopy := make(map[string][]byte, len(f.data))
	for k, v := range f.data {
		valueCopy := make([]byte, len(v))
		copy(valueCopy, v)
		dataCopy[k] = valueCopy
	}

	return &FSMSnapshot{data: dataCopy}, nil
}

// Restore restores the FSM from a snapshot.
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	var data map[string][]byte
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	f.mu.Lock()
	f.data = data
	f.mu.Unlock()

	f.logger.Info("fsm restored from snapshot", "keys", len(data))
	return nil
}

// Get retrieves a value from the FSM.
// Note: This is a local read and may be stale on followers.
func (f *FSM) Get(key string) ([]byte, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	value, exists := f.data[key]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent data races
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	return valueCopy, true
}

// Keys returns all keys in the FSM.
func (f *FSM) Keys() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	keys := make([]string, 0, len(f.data))
	for k := range f.data {
		keys = append(keys, k)
	}
	return keys
}

// List returns all key-value pairs where the key starts with the given prefix.
func (f *FSM) List(prefix string) map[string][]byte {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string][]byte)
	for k, v := range f.data {
		if strings.HasPrefix(k, prefix) {
			// Copy value to prevent data races
			valueCopy := make([]byte, len(v))
			copy(valueCopy, v)
			result[k] = valueCopy
		}
	}
	return result
}

// FSMSnapshot is a point-in-time snapshot of the FSM state.
type FSMSnapshot struct {
	data map[string][]byte
}

// Persist writes the snapshot to the given sink.
func (s *FSMSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode data
		data, err := json.Marshal(s.data)
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot: %w", err)
		}

		// Write to sink
		if _, err := sink.Write(data); err != nil {
			return fmt.Errorf("failed to write snapshot: %w", err)
		}

		return nil
	}()

	if err != nil {
		_ = sink.Cancel()
		return err
	}

	return sink.Close()
}

// Release releases any resources associated with the snapshot.
func (s *FSMSnapshot) Release() {}

// FSMWatcher defines a callback for FSM events.
// Implementations MUST be non-blocking as they are called during Apply().
// Use buffered channels with non-blocking sends if async processing is needed.
type FSMWatcher func(key string, value []byte, isDelete bool)

// AddWatcher adds a watcher to the FSM.
// The watcher callback will be invoked synchronously during Apply(),
// so it MUST be non-blocking.
func (f *FSM) AddWatcher(watcher FSMWatcher) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watchers = append(f.watchers, watcher)
}
