// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

const (
	// OverrideKeyPrefix is the prefix for all override keys in the KV store.
	OverrideKeyPrefix = "overrides/"

	// OverrideAuthority is the authority level for external tool overrides.
	OverrideAuthority = "external_tool"
)

// OverrideManager manages health overrides for servers.
// It persists overrides to a KV store and provides an in-memory cache.
// Overrides are cleared on restart (as per design: intentional).
type OverrideManager struct {
	store  store.Store
	logger *slog.Logger

	mu        sync.RWMutex
	overrides map[string]*Override // key: "service/address"

	// Callback for audit logging
	auditLogger func(action, service, address, reason, source, clientIP string)
}

// NewOverrideManager creates a new OverrideManager.
// If store is nil, overrides will only be kept in memory.
func NewOverrideManager(kvStore store.Store, logger *slog.Logger) *OverrideManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &OverrideManager{
		store:     kvStore,
		logger:    logger,
		overrides: make(map[string]*Override),
	}
}

// SetAuditLogger sets the callback function for audit logging.
func (m *OverrideManager) SetAuditLogger(fn func(action, service, address, reason, source, clientIP string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.auditLogger = fn
}

// makeKey creates a unique key for the service/address combination.
func makeKey(service, address string) string {
	return service + "/" + address
}

// storeKey creates the full KV store key.
func storeKey(service, address string) string {
	return OverrideKeyPrefix + makeKey(service, address)
}

// SetOverride sets or updates an override for a service/address.
func (m *OverrideManager) SetOverride(ctx context.Context, service, address string, healthy bool, reason, source, clientIP string) (*Override, error) {
	if service == "" {
		return nil, fmt.Errorf("service is required")
	}
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	override := &Override{
		Service:   service,
		Address:   address,
		Healthy:   healthy,
		Reason:    reason,
		Source:    source,
		CreatedAt: time.Now().UTC(),
		Authority: OverrideAuthority,
	}

	key := makeKey(service, address)

	// Update in-memory cache
	m.mu.Lock()
	m.overrides[key] = override
	auditFn := m.auditLogger
	m.mu.Unlock()

	// Persist to KV store if available
	if m.store != nil {
		data, err := json.Marshal(override)
		if err != nil {
			m.logger.Error("failed to marshal override", "error", err)
			return override, nil // Return override even if persistence fails
		}

		if err := m.store.Set(ctx, storeKey(service, address), data); err != nil {
			m.logger.Error("failed to persist override",
				"service", service,
				"address", address,
				"error", err,
			)
			// Continue - override is still in memory
		}
	}

	m.logger.Info("override set",
		"service", service,
		"address", address,
		"healthy", healthy,
		"reason", reason,
		"source", source,
	)

	// Audit log
	if auditFn != nil {
		action := "override_set_unhealthy"
		if healthy {
			action = "override_set_healthy"
		}
		auditFn(action, service, address, reason, source, clientIP)
	}

	return override, nil
}

// ClearOverride removes an override for a service/address.
func (m *OverrideManager) ClearOverride(ctx context.Context, service, address, clientIP string) error {
	if service == "" {
		return fmt.Errorf("service is required")
	}
	if address == "" {
		return fmt.Errorf("address is required")
	}

	key := makeKey(service, address)

	m.mu.Lock()
	existing, exists := m.overrides[key]
	if exists {
		delete(m.overrides, key)
	}
	auditFn := m.auditLogger
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("override not found")
	}

	// Delete from KV store if available
	if m.store != nil {
		if err := m.store.Delete(ctx, storeKey(service, address)); err != nil {
			m.logger.Warn("failed to delete override from store",
				"service", service,
				"address", address,
				"error", err,
			)
			// Continue - override already removed from memory
		}
	}

	m.logger.Info("override cleared",
		"service", service,
		"address", address,
	)

	// Audit log
	if auditFn != nil {
		reason := ""
		source := ""
		if existing != nil {
			reason = existing.Reason
			source = existing.Source
		}
		auditFn("override_cleared", service, address, reason, source, clientIP)
	}

	return nil
}

// GetOverride retrieves an override for a service/address.
func (m *OverrideManager) GetOverride(service, address string) (*Override, bool) {
	key := makeKey(service, address)

	m.mu.RLock()
	defer m.mu.RUnlock()

	override, exists := m.overrides[key]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent mutation
	copy := *override
	return &copy, true
}

// ListOverrides returns all active overrides.
func (m *OverrideManager) ListOverrides() []Override {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Override, 0, len(m.overrides))
	for _, o := range m.overrides {
		result = append(result, *o)
	}

	return result
}

// IsOverridden checks if a server has an active override and returns the health state.
// Returns (healthy, hasOverride).
func (m *OverrideManager) IsOverridden(service, address string) (bool, bool) {
	override, exists := m.GetOverride(service, address)
	if !exists {
		return false, false
	}
	return override.Healthy, true
}

// IsServerHealthy checks if a server should be considered healthy based on overrides.
// This can be used by the health authority to incorporate external overrides.
// Returns (healthy, hasOverride).
func (m *OverrideManager) IsServerHealthy(service, address string) (bool, bool) {
	return m.IsOverridden(service, address)
}

// LoadFromStore loads overrides from the KV store into memory.
// This should NOT be called on restart since overrides are intentionally cleared.
// This method exists for potential future use cases where persistence across restarts
// might be desired.
func (m *OverrideManager) LoadFromStore(ctx context.Context) error {
	if m.store == nil {
		return nil
	}

	pairs, err := m.store.List(ctx, OverrideKeyPrefix)
	if err != nil {
		return fmt.Errorf("failed to list overrides: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, pair := range pairs {
		var override Override
		if err := json.Unmarshal(pair.Value, &override); err != nil {
			m.logger.Warn("failed to unmarshal override",
				"key", pair.Key,
				"error", err,
			)
			continue
		}

		key := makeKey(override.Service, override.Address)
		m.overrides[key] = &override
	}

	m.logger.Info("loaded overrides from store", "count", len(m.overrides))

	return nil
}

// Clear removes all overrides from memory and the KV store.
func (m *OverrideManager) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear from KV store
	if m.store != nil {
		pairs, err := m.store.List(ctx, OverrideKeyPrefix)
		if err != nil {
			m.logger.Warn("failed to list overrides for clearing", "error", err)
		} else {
			for _, pair := range pairs {
				if err := m.store.Delete(ctx, pair.Key); err != nil {
					m.logger.Warn("failed to delete override", "key", pair.Key, "error", err)
				}
			}
		}
	}

	// Clear in-memory cache
	m.overrides = make(map[string]*Override)

	m.logger.Info("all overrides cleared")

	return nil
}

// Count returns the number of active overrides.
func (m *OverrideManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.overrides)
}
