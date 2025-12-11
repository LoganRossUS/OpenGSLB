// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dnssec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// KeySyncConfig configures DNSSEC key synchronization between Overwatches.
type KeySyncConfig struct {
	// Peers are the API addresses of other Overwatch nodes.
	// Format: "https://overwatch-2.internal:9090"
	Peers []string

	// PollInterval is the time between key sync polls.
	// Default: 1 hour
	PollInterval time.Duration

	// Timeout is the timeout for sync requests.
	// Default: 30 seconds
	Timeout time.Duration

	// KeyStore for persisting synced keys.
	KeyStore *KeyStore

	// KeyManager for managing keys in memory.
	KeyManager *KeyManager

	// Logger for sync operations.
	Logger *slog.Logger

	// MetricsCallback is called with sync results for metrics.
	// Parameters: peer, success, keysImported
	MetricsCallback func(peer string, success bool, keysImported int)
}

// KeySyncer synchronizes DNSSEC keys between Overwatch nodes.
// This is the ONLY inter-Overwatch communication per ADR-015.
// Keys are synced with a "newest wins" strategy.
type KeySyncer struct {
	config     KeySyncConfig
	logger     *slog.Logger
	httpClient *http.Client

	mu              sync.RWMutex
	running         bool
	cancel          context.CancelFunc
	lastSyncTime    map[string]time.Time // peer -> last sync time
	lastSyncSuccess map[string]bool      // peer -> last sync success
	lastSyncError   map[string]string    // peer -> last error message
}

// KeySyncResponse is the response from the key sync API endpoint.
type KeySyncResponse struct {
	Keys []*KeyPair `json:"keys"`
}

// NewKeySyncer creates a new key syncer.
func NewKeySyncer(config KeySyncConfig) *KeySyncer {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if config.PollInterval == 0 {
		config.PollInterval = time.Hour
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &KeySyncer{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		lastSyncTime:    make(map[string]time.Time),
		lastSyncSuccess: make(map[string]bool),
		lastSyncError:   make(map[string]string),
	}
}

// Start begins the key sync polling loop.
func (ks *KeySyncer) Start(ctx context.Context) error {
	ks.mu.Lock()
	if ks.running {
		ks.mu.Unlock()
		return fmt.Errorf("key syncer already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	ks.cancel = cancel
	ks.running = true
	ks.mu.Unlock()

	ks.logger.Info("starting DNSSEC key syncer",
		"peers", ks.config.Peers,
		"poll_interval", ks.config.PollInterval,
	)

	// Do initial sync
	ks.syncAllPeers(ctx)

	// Start polling loop
	go ks.pollLoop(ctx)

	return nil
}

// Stop stops the key sync polling.
func (ks *KeySyncer) Stop() {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.running {
		return
	}

	if ks.cancel != nil {
		ks.cancel()
	}
	ks.running = false
	ks.logger.Info("stopped DNSSEC key syncer")
}

// pollLoop runs the periodic sync loop.
func (ks *KeySyncer) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(ks.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ks.syncAllPeers(ctx)
		}
	}
}

// syncAllPeers syncs keys from all configured peers.
func (ks *KeySyncer) syncAllPeers(ctx context.Context) {
	for _, peer := range ks.config.Peers {
		if err := ks.syncFromPeer(ctx, peer); err != nil {
			ks.logger.Warn("failed to sync from peer",
				"peer", peer,
				"error", err,
			)
			ks.recordSyncResult(peer, false, 0, err.Error())
		}
	}
}

// syncFromPeer fetches keys from a single peer and imports newer ones.
func (ks *KeySyncer) syncFromPeer(ctx context.Context, peer string) error {
	ks.logger.Debug("syncing keys from peer", "peer", peer)

	// Fetch keys from peer
	url := peer + "/api/v1/dnssec/keys"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "OpenGSLB-KeySync/1.0")

	resp, err := ks.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch keys: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("peer returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var syncResp KeySyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Import keys (newest wins)
	keysImported := 0
	for _, remoteKey := range syncResp.Keys {
		localKey := ks.config.KeyManager.GetKey(remoteKey.Zone)

		// Import if we don't have this key or the remote key is newer
		if remoteKey.IsNewerThan(localKey) {
			// Clear private key from synced keys (security)
			// We only need the public key for verification
			remoteCopy := *remoteKey
			remoteCopy.PrivateKey = ""

			if err := ks.config.KeyManager.SetKey(&remoteCopy); err != nil {
				ks.logger.Warn("failed to import key",
					"zone", remoteKey.Zone,
					"peer", peer,
					"error", err,
				)
				continue
			}

			// Persist the synced key
			if ks.config.KeyStore != nil {
				if err := ks.config.KeyStore.SaveKey(ctx, &remoteCopy); err != nil {
					ks.logger.Warn("failed to persist synced key",
						"zone", remoteKey.Zone,
						"error", err,
					)
				}
			}

			keysImported++
			ks.logger.Info("imported DNSSEC key from peer",
				"zone", remoteKey.Zone,
				"peer", peer,
				"key_tag", remoteKey.KeyTag,
				"remote_created", remoteKey.CreatedAt,
			)
		}
	}

	ks.recordSyncResult(peer, true, keysImported, "")

	ks.logger.Debug("completed sync from peer",
		"peer", peer,
		"keys_received", len(syncResp.Keys),
		"keys_imported", keysImported,
	)

	return nil
}

// recordSyncResult records the result of a sync attempt.
func (ks *KeySyncer) recordSyncResult(peer string, success bool, keysImported int, errMsg string) {
	ks.mu.Lock()
	ks.lastSyncTime[peer] = time.Now()
	ks.lastSyncSuccess[peer] = success
	ks.lastSyncError[peer] = errMsg
	ks.mu.Unlock()

	if ks.config.MetricsCallback != nil {
		ks.config.MetricsCallback(peer, success, keysImported)
	}
}

// SyncNow triggers an immediate sync from all peers.
func (ks *KeySyncer) SyncNow(ctx context.Context) {
	ks.syncAllPeers(ctx)
}

// GetSyncStatus returns the current sync status for all peers.
func (ks *KeySyncer) GetSyncStatus() []PeerSyncStatus {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	statuses := make([]PeerSyncStatus, 0, len(ks.config.Peers))
	for _, peer := range ks.config.Peers {
		status := PeerSyncStatus{
			Peer:        peer,
			LastSync:    ks.lastSyncTime[peer],
			LastSuccess: ks.lastSyncSuccess[peer],
			LastError:   ks.lastSyncError[peer],
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// PeerSyncStatus represents the sync status for a single peer.
type PeerSyncStatus struct {
	Peer        string    `json:"peer"`
	LastSync    time.Time `json:"last_sync"`
	LastSuccess bool      `json:"last_success"`
	LastError   string    `json:"last_error,omitempty"`
}

// IsRunning returns true if the syncer is running.
func (ks *KeySyncer) IsRunning() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.running
}

// SyncStatus represents the overall sync status for API responses.
type SyncStatus struct {
	Running      bool             `json:"running"`
	PollInterval string           `json:"poll_interval"`
	Peers        []PeerSyncStatus `json:"peers"`
}

// GetStatus returns the full sync status for API responses.
func (ks *KeySyncer) GetStatus() SyncStatus {
	return SyncStatus{
		Running:      ks.IsRunning(),
		PollInterval: ks.config.PollInterval.String(),
		Peers:        ks.GetSyncStatus(),
	}
}
