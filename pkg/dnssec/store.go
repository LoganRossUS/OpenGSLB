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
	"log/slog"
	"strings"

	"github.com/loganrossus/OpenGSLB/pkg/store"
	"github.com/miekg/dns"
)

// KeyStore provides persistent storage for DNSSEC keys.
type KeyStore struct {
	store      store.Store
	keyManager *KeyManager
	logger     *slog.Logger
}

// NewKeyStore creates a new DNSSEC key store.
func NewKeyStore(s store.Store, km *KeyManager, logger *slog.Logger) *KeyStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &KeyStore{
		store:      s,
		keyManager: km,
		logger:     logger,
	}
}

// SaveKey saves a key pair to persistent storage.
func (ks *KeyStore) SaveKey(ctx context.Context, key *KeyPair) error {
	if key == nil {
		return fmt.Errorf("key cannot be nil")
	}

	data, err := json.Marshal(key)
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	// Key format: dnssec/{zone}
	storeKey := store.PrefixDNSSEC + sanitizeZone(key.Zone)

	if err := ks.store.Set(ctx, storeKey, data); err != nil {
		return fmt.Errorf("failed to store key: %w", err)
	}

	ks.logger.Info("saved DNSSEC key",
		"zone", key.Zone,
		"key_tag", key.KeyTag,
		"algorithm", key.Algorithm,
	)

	return nil
}

// LoadKey loads a key pair from persistent storage.
func (ks *KeyStore) LoadKey(ctx context.Context, zone string) (*KeyPair, error) {
	zone = dns.Fqdn(zone)
	storeKey := store.PrefixDNSSEC + sanitizeZone(zone)

	data, err := ks.store.Get(ctx, storeKey)
	if err != nil {
		if err == store.ErrKeyNotFound {
			return nil, nil // Key doesn't exist
		}
		return nil, fmt.Errorf("failed to load key: %w", err)
	}

	var key KeyPair
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key: %w", err)
	}

	return &key, nil
}

// DeleteKey removes a key from persistent storage.
func (ks *KeyStore) DeleteKey(ctx context.Context, zone string) error {
	zone = dns.Fqdn(zone)
	storeKey := store.PrefixDNSSEC + sanitizeZone(zone)

	if err := ks.store.Delete(ctx, storeKey); err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	ks.logger.Info("deleted DNSSEC key", "zone", zone)
	return nil
}

// LoadAllKeys loads all keys from persistent storage into the key manager.
func (ks *KeyStore) LoadAllKeys(ctx context.Context) error {
	pairs, err := ks.store.List(ctx, store.PrefixDNSSEC)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	loaded := 0
	for _, pair := range pairs {
		var key KeyPair
		if err := json.Unmarshal(pair.Value, &key); err != nil {
			ks.logger.Warn("failed to unmarshal key",
				"key", pair.Key,
				"error", err,
			)
			continue
		}

		if err := ks.keyManager.SetKey(&key); err != nil {
			ks.logger.Warn("failed to set key in manager",
				"zone", key.Zone,
				"error", err,
			)
			continue
		}

		loaded++
		ks.logger.Debug("loaded DNSSEC key",
			"zone", key.Zone,
			"key_tag", key.KeyTag,
			"created_at", key.CreatedAt,
		)
	}

	ks.logger.Info("loaded DNSSEC keys from storage", "count", loaded)
	return nil
}

// SaveAllKeys saves all keys from the key manager to persistent storage.
func (ks *KeyStore) SaveAllKeys(ctx context.Context) error {
	keys := ks.keyManager.GetAllKeys()

	saved := 0
	for _, key := range keys {
		if err := ks.SaveKey(ctx, key); err != nil {
			ks.logger.Warn("failed to save key",
				"zone", key.Zone,
				"error", err,
			)
			continue
		}
		saved++
	}

	ks.logger.Info("saved DNSSEC keys to storage", "count", saved)
	return nil
}

// EnsureKeysForZones generates keys for any zones that don't have one.
func (ks *KeyStore) EnsureKeysForZones(ctx context.Context, zones []string, algorithm Algorithm) error {
	for _, zone := range zones {
		zone = dns.Fqdn(zone)

		// Check if key exists in manager
		if ks.keyManager.GetKey(zone) != nil {
			continue
		}

		// Try to load from store
		key, err := ks.LoadKey(ctx, zone)
		if err != nil {
			return fmt.Errorf("failed to load key for %s: %w", zone, err)
		}

		if key != nil {
			// Load into manager
			if err := ks.keyManager.SetKey(key); err != nil {
				return fmt.Errorf("failed to set key for %s: %w", zone, err)
			}
			ks.logger.Info("loaded existing DNSSEC key", "zone", zone)
			continue
		}

		// Generate new key
		newKey, err := ks.keyManager.GenerateKey(zone, algorithm)
		if err != nil {
			return fmt.Errorf("failed to generate key for %s: %w", zone, err)
		}

		// Save to store
		if err := ks.SaveKey(ctx, newKey); err != nil {
			return fmt.Errorf("failed to save key for %s: %w", zone, err)
		}

		ks.logger.Info("generated new DNSSEC key",
			"zone", zone,
			"key_tag", newKey.KeyTag,
			"algorithm", newKey.Algorithm,
		)
	}

	return nil
}

// sanitizeZone converts a zone name to a safe storage key.
func sanitizeZone(zone string) string {
	// Replace dots with underscores for cleaner keys
	// But preserve the trailing dot handling
	zone = dns.Fqdn(zone)
	// Remove trailing dot for storage
	return strings.TrimSuffix(zone, ".")
}

// KeyInfo represents key information for API responses.
type KeyInfo struct {
	Zone       string    `json:"zone"`
	KeyTag     uint16    `json:"key_tag"`
	Algorithm  Algorithm `json:"algorithm"`
	Flags      uint16    `json:"flags"`
	PublicKey  string    `json:"public_key"`
	CreatedAt  string    `json:"created_at"`
	NodeID     string    `json:"node_id"`
	CanSign    bool      `json:"can_sign"`
	AgeSeconds int64     `json:"age_seconds"`
}

// GetKeyInfo returns key information for API responses.
func (ks *KeyStore) GetKeyInfo(zone string) *KeyInfo {
	key := ks.keyManager.GetKey(zone)
	if key == nil {
		return nil
	}

	return &KeyInfo{
		Zone:       key.Zone,
		KeyTag:     key.KeyTag,
		Algorithm:  key.Algorithm,
		Flags:      key.Flags,
		PublicKey:  key.PublicKey,
		CreatedAt:  key.CreatedAt.Format("2006-01-02T15:04:05Z"),
		NodeID:     key.NodeID,
		CanSign:    key.CanSign(),
		AgeSeconds: int64(key.Age().Seconds()),
	}
}

// GetAllKeyInfo returns information about all managed keys.
func (ks *KeyStore) GetAllKeyInfo() []*KeyInfo {
	keys := ks.keyManager.GetAllKeys()
	infos := make([]*KeyInfo, 0, len(keys))

	for _, key := range keys {
		infos = append(infos, &KeyInfo{
			Zone:       key.Zone,
			KeyTag:     key.KeyTag,
			Algorithm:  key.Algorithm,
			Flags:      key.Flags,
			PublicKey:  key.PublicKey,
			CreatedAt:  key.CreatedAt.Format("2006-01-02T15:04:05Z"),
			NodeID:     key.NodeID,
			CanSign:    key.CanSign(),
			AgeSeconds: int64(key.Age().Seconds()),
		})
	}

	return infos
}
