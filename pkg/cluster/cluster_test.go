// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"os"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with NodeID",
			config: Config{
				NodeID:      "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
			},
			wantErr: false,
		},
		{
			name: "valid config with NodeName",
			config: Config{
				NodeName:    "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
			},
			wantErr: false,
		},
		{
			name: "valid config with both IDs",
			config: Config{
				NodeID:      "node1-id",
				NodeName:    "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
			},
			wantErr: false,
		},
		{
			name: "missing node identity",
			config: Config{
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
			},
			wantErr: true,
			errMsg:  "NodeID or NodeName",
		},
		{
			name: "missing bind address",
			config: Config{
				NodeID:  "node1",
				DataDir: "/tmp/raft",
			},
			wantErr: true,
			errMsg:  "BindAddress",
		},
		{
			name: "missing data dir",
			config: Config{
				NodeID:      "node1",
				BindAddress: "127.0.0.1:7946",
			},
			wantErr: true,
			errMsg:  "DataDir",
		},
		{
			name: "both bootstrap and join",
			config: Config{
				NodeID:      "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
				Bootstrap:   true,
				Join:        []string{"127.0.0.1:9090"},
			},
			wantErr: true,
			errMsg:  "bootstrap and join",
		},
		{
			name: "valid bootstrap",
			config: Config{
				NodeID:      "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
				Bootstrap:   true,
			},
			wantErr: false,
		},
		{
			name: "valid join",
			config: Config{
				NodeID:      "node1",
				BindAddress: "127.0.0.1:7946",
				DataDir:     "/tmp/raft",
				Join:        []string{"127.0.0.1:9090", "127.0.0.2:9090"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error message %q should contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestConfigGetNodeID(t *testing.T) {
	tests := []struct {
		name     string
		nodeID   string
		nodeName string
		want     string
	}{
		{"prefer NodeID", "id1", "name1", "id1"},
		{"fallback to NodeName", "", "name1", "name1"},
		{"empty both", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{NodeID: tt.nodeID, NodeName: tt.nodeName}
			if got := c.GetNodeID(); got != tt.want {
				t.Errorf("GetNodeID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigGetAdvertiseAddress(t *testing.T) {
	tests := []struct {
		name          string
		bindAddr      string
		advertiseAddr string
		want          string
	}{
		{"prefer advertise", "bind:7946", "advertise:7946", "advertise:7946"},
		{"fallback to bind", "bind:7946", "", "bind:7946"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{BindAddress: tt.bindAddr, AdvertiseAddress: tt.advertiseAddr}
			if got := c.GetAdvertiseAddress(); got != tt.want {
				t.Errorf("GetAdvertiseAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HeartbeatTimeout != 1000*time.Millisecond {
		t.Errorf("HeartbeatTimeout = %v, want 1s", cfg.HeartbeatTimeout)
	}
	if cfg.ElectionTimeout != 1000*time.Millisecond {
		t.Errorf("ElectionTimeout = %v, want 1s", cfg.ElectionTimeout)
	}
	if cfg.SnapshotInterval != 120*time.Second {
		t.Errorf("SnapshotInterval = %v, want 120s", cfg.SnapshotInterval)
	}
	if cfg.SnapshotThreshold != 8192 {
		t.Errorf("SnapshotThreshold = %v, want 8192", cfg.SnapshotThreshold)
	}
}

// containsString checks if substr is in s (simple helper).
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestMain handles test setup and teardown.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
