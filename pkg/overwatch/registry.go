// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package overwatch provides Overwatch mode functionality for OpenGSLB.
// Overwatch nodes receive health reports from agents, perform external validation,
// and serve authoritative DNS responses.
package overwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// BackendStatus represents the health status of a backend as determined by Overwatch.
type BackendStatus string

const (
	// StatusHealthy indicates the backend is healthy (agent claims healthy, validation passed).
	StatusHealthy BackendStatus = "healthy"
	// StatusUnhealthy indicates the backend is unhealthy (validation failed or agent claims unhealthy).
	StatusUnhealthy BackendStatus = "unhealthy"
	// StatusStale indicates no heartbeat received within threshold.
	StatusStale BackendStatus = "stale"
	// StatusOverridden indicates the status was manually overridden via API.
	StatusOverridden BackendStatus = "overridden"
)

// Backend represents a registered backend from an agent.
type Backend struct {
	// Service is the service name (maps to DNS domain).
	Service string `json:"service"`
	// Address is the backend IP address.
	Address string `json:"address"`
	// Port is the backend port.
	Port int `json:"port"`
	// Weight is the routing weight.
	Weight int `json:"weight"`

	// AgentID is the ID of the agent that registered this backend.
	AgentID string `json:"agent_id"`
	// Region is the geographic region.
	Region string `json:"region"`

	// AgentHealthy is the health status claimed by the agent.
	AgentHealthy bool `json:"agent_healthy"`
	// AgentLastSeen is when we last received a heartbeat from the agent.
	AgentLastSeen time.Time `json:"agent_last_seen"`

	// ValidationHealthy is the health status from external validation.
	// nil means not yet validated.
	ValidationHealthy *bool `json:"validation_healthy,omitempty"`
	// ValidationLastCheck is when external validation was last performed.
	ValidationLastCheck time.Time `json:"validation_last_check,omitempty"`
	// ValidationError is the last validation error, if any.
	ValidationError string `json:"validation_error,omitempty"`

	// OverrideStatus is a manual override status, if set.
	OverrideStatus *bool `json:"override_status,omitempty"`
	// OverrideReason is the reason for the override.
	OverrideReason string `json:"override_reason,omitempty"`
	// OverrideBy is who set the override.
	OverrideBy string `json:"override_by,omitempty"`
	// OverrideAt is when the override was set.
	OverrideAt time.Time `json:"override_at,omitempty"`

	// EffectiveStatus is the computed effective status based on the hierarchy.
	EffectiveStatus BackendStatus `json:"effective_status"`
}

// RegistryConfig configures the backend registry.
type RegistryConfig struct {
	// StaleThreshold is the duration after which a backend is considered stale
	// if no heartbeat is received.
	StaleThreshold time.Duration

	// RemoveAfter is the duration after which a stale backend is removed.
	RemoveAfter time.Duration

	// Logger for registry operations.
	Logger *slog.Logger
}

// DefaultRegistryConfig returns sensible defaults.
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		StaleThreshold: 30 * time.Second,
		RemoveAfter:    5 * time.Minute,
		Logger:         slog.Default(),
	}
}

// Registry manages backend registrations from agents.
type Registry struct {
	config   RegistryConfig
	store    store.Store
	backends map[string]*Backend // key: "service:address:port"
	mu       sync.RWMutex

	// Callbacks
	onStatusChange func(backend *Backend, oldStatus, newStatus BackendStatus)

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRegistry creates a new backend registry.
func NewRegistry(cfg RegistryConfig, st store.Store) *Registry {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		config:   cfg,
		store:    st,
		backends: make(map[string]*Backend),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins background tasks (stale detection, persistence).
func (r *Registry) Start() error {
	// Load persisted backends from store
	if r.store != nil {
		if err := r.loadFromStore(); err != nil {
			r.config.Logger.Warn("failed to load backends from store", "error", err)
		}
	}

	// Start stale detection loop
	r.wg.Add(1)
	go r.staleDetectionLoop()

	r.config.Logger.Info("backend registry started",
		"stale_threshold", r.config.StaleThreshold,
		"remove_after", r.config.RemoveAfter,
	)
	return nil
}

// Stop halts background tasks.
func (r *Registry) Stop() error {
	r.cancel()
	r.wg.Wait()
	r.config.Logger.Info("backend registry stopped")
	return nil
}

// OnStatusChange sets a callback for backend status changes.
func (r *Registry) OnStatusChange(fn func(backend *Backend, oldStatus, newStatus BackendStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onStatusChange = fn
}

// Register registers or updates a backend from an agent heartbeat.
func (r *Registry) Register(agentID, region, service, address string, port, weight int, healthy bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := backendKey(service, address, port)
	now := time.Now()

	backend, exists := r.backends[key]
	if !exists {
		backend = &Backend{
			Service:         service,
			Address:         address,
			Port:            port,
			Weight:          weight,
			AgentID:         agentID,
			Region:          region,
			AgentHealthy:    healthy,
			AgentLastSeen:   now,
			EffectiveStatus: StatusHealthy,
		}
		r.backends[key] = backend
		r.config.Logger.Info("backend registered",
			"service", service,
			"address", address,
			"port", port,
			"agent_id", agentID,
			"region", region,
		)
	} else {
		backend.AgentID = agentID
		backend.Region = region
		backend.Weight = weight
		backend.AgentHealthy = healthy
		backend.AgentLastSeen = now
	}

	oldStatus := backend.EffectiveStatus
	r.computeEffectiveStatus(backend)

	if oldStatus != backend.EffectiveStatus && r.onStatusChange != nil {
		r.onStatusChange(backend, oldStatus, backend.EffectiveStatus)
	}

	// Persist to store
	if r.store != nil {
		if err := r.persistBackend(backend); err != nil {
			r.config.Logger.Warn("failed to persist backend", "key", key, "error", err)
		}
	}

	return nil
}

// Deregister removes a backend.
func (r *Registry) Deregister(service, address string, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := backendKey(service, address, port)
	backend, exists := r.backends[key]
	if !exists {
		return fmt.Errorf("backend %s not found", key)
	}

	delete(r.backends, key)

	r.config.Logger.Info("backend deregistered",
		"service", service,
		"address", address,
		"port", port,
	)

	// Remove from store
	if r.store != nil {
		if err := r.store.Delete(r.ctx, storeKey(key)); err != nil {
			r.config.Logger.Warn("failed to delete backend from store", "key", key, "error", err)
		}
	}

	if r.onStatusChange != nil {
		r.onStatusChange(backend, backend.EffectiveStatus, "")
	}

	return nil
}

// UpdateValidation updates the external validation result for a backend.
func (r *Registry) UpdateValidation(service, address string, port int, healthy bool, validationErr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := backendKey(service, address, port)
	backend, exists := r.backends[key]
	if !exists {
		return fmt.Errorf("backend %s not found", key)
	}

	oldStatus := backend.EffectiveStatus
	backend.ValidationHealthy = &healthy
	backend.ValidationLastCheck = time.Now()
	backend.ValidationError = validationErr

	r.computeEffectiveStatus(backend)

	if oldStatus != backend.EffectiveStatus {
		r.config.Logger.Info("backend validation status changed",
			"service", service,
			"address", address,
			"port", port,
			"old_status", oldStatus,
			"new_status", backend.EffectiveStatus,
			"validation_healthy", healthy,
		)
		if r.onStatusChange != nil {
			r.onStatusChange(backend, oldStatus, backend.EffectiveStatus)
		}
	}

	// Persist to store
	if r.store != nil {
		if err := r.persistBackend(backend); err != nil {
			r.config.Logger.Warn("failed to persist backend", "key", key, "error", err)
		}
	}

	return nil
}

// SetOverride sets a manual override for a backend.
func (r *Registry) SetOverride(service, address string, port int, healthy bool, reason, by string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := backendKey(service, address, port)
	backend, exists := r.backends[key]
	if !exists {
		return fmt.Errorf("backend %s not found", key)
	}

	oldStatus := backend.EffectiveStatus
	backend.OverrideStatus = &healthy
	backend.OverrideReason = reason
	backend.OverrideBy = by
	backend.OverrideAt = time.Now()

	r.computeEffectiveStatus(backend)

	r.config.Logger.Info("backend override set",
		"service", service,
		"address", address,
		"port", port,
		"healthy", healthy,
		"reason", reason,
		"by", by,
	)

	if oldStatus != backend.EffectiveStatus && r.onStatusChange != nil {
		r.onStatusChange(backend, oldStatus, backend.EffectiveStatus)
	}

	// Persist to store
	if r.store != nil {
		if err := r.persistBackend(backend); err != nil {
			r.config.Logger.Warn("failed to persist backend", "key", key, "error", err)
		}
	}

	return nil
}

// ClearOverride removes a manual override for a backend.
func (r *Registry) ClearOverride(service, address string, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := backendKey(service, address, port)
	backend, exists := r.backends[key]
	if !exists {
		return fmt.Errorf("backend %s not found", key)
	}

	if backend.OverrideStatus == nil {
		return nil // No override to clear
	}

	oldStatus := backend.EffectiveStatus
	backend.OverrideStatus = nil
	backend.OverrideReason = ""
	backend.OverrideBy = ""
	backend.OverrideAt = time.Time{}

	r.computeEffectiveStatus(backend)

	r.config.Logger.Info("backend override cleared",
		"service", service,
		"address", address,
		"port", port,
	)

	if oldStatus != backend.EffectiveStatus && r.onStatusChange != nil {
		r.onStatusChange(backend, oldStatus, backend.EffectiveStatus)
	}

	// Persist to store
	if r.store != nil {
		if err := r.persistBackend(backend); err != nil {
			r.config.Logger.Warn("failed to persist backend", "key", key, "error", err)
		}
	}

	return nil
}

// GetBackend returns a backend by key.
func (r *Registry) GetBackend(service, address string, port int) (*Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := backendKey(service, address, port)
	backend, exists := r.backends[key]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent external modification
	copy := *backend
	return &copy, true
}

// GetBackends returns all backends for a service.
func (r *Registry) GetBackends(service string) []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Backend
	for _, backend := range r.backends {
		if backend.Service == service {
			copy := *backend
			result = append(result, &copy)
		}
	}
	return result
}

// GetHealthyBackends returns all healthy backends for a service.
func (r *Registry) GetHealthyBackends(service string) []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Backend
	for _, backend := range r.backends {
		if backend.Service == service && backend.EffectiveStatus == StatusHealthy {
			copy := *backend
			result = append(result, &copy)
		}
	}
	return result
}

// GetAllBackends returns all registered backends.
func (r *Registry) GetAllBackends() []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Backend, 0, len(r.backends))
	for _, backend := range r.backends {
		copy := *backend
		result = append(result, &copy)
	}
	return result
}

// IsHealthy returns true if the backend is healthy (for DNS handler).
func (r *Registry) IsHealthy(address string, port int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Search for backend by address:port (service-agnostic for DNS)
	for _, backend := range r.backends {
		if backend.Address == address && backend.Port == port {
			return backend.EffectiveStatus == StatusHealthy
		}
	}
	return false
}

// BackendCount returns the total number of registered backends.
func (r *Registry) BackendCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.backends)
}

// computeEffectiveStatus computes the effective status based on the hierarchy:
// 1. Manual override (if set) - takes precedence
// 2. External validation (if performed) - Overwatch ALWAYS wins
// 3. Staleness detection
// 4. Agent health claim (default)
func (r *Registry) computeEffectiveStatus(backend *Backend) {
	now := time.Now()

	// Check staleness first
	if now.Sub(backend.AgentLastSeen) > r.config.StaleThreshold {
		backend.EffectiveStatus = StatusStale
		return
	}

	// Manual override takes precedence
	if backend.OverrideStatus != nil {
		if *backend.OverrideStatus {
			backend.EffectiveStatus = StatusHealthy
		} else {
			backend.EffectiveStatus = StatusUnhealthy
		}
		return
	}

	// External validation ALWAYS wins over agent claims (ADR-015 hierarchy)
	if backend.ValidationHealthy != nil {
		if *backend.ValidationHealthy {
			backend.EffectiveStatus = StatusHealthy
		} else {
			backend.EffectiveStatus = StatusUnhealthy
		}
		return
	}

	// Fall back to agent claim
	if backend.AgentHealthy {
		backend.EffectiveStatus = StatusHealthy
	} else {
		backend.EffectiveStatus = StatusUnhealthy
	}
}

// staleDetectionLoop periodically checks for and removes stale backends.
func (r *Registry) staleDetectionLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.StaleThreshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.checkStaleBackends()
		}
	}
}

// checkStaleBackends checks for stale backends and removes expired ones.
func (r *Registry) checkStaleBackends() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for key, backend := range r.backends {
		timeSinceLastSeen := now.Sub(backend.AgentLastSeen)

		// Check if backend should be removed
		if timeSinceLastSeen > r.config.RemoveAfter {
			toRemove = append(toRemove, key)
			continue
		}

		// Check if backend should be marked stale
		if timeSinceLastSeen > r.config.StaleThreshold {
			if backend.EffectiveStatus != StatusStale {
				oldStatus := backend.EffectiveStatus
				backend.EffectiveStatus = StatusStale

				r.config.Logger.Warn("backend marked stale",
					"service", backend.Service,
					"address", backend.Address,
					"port", backend.Port,
					"last_seen", backend.AgentLastSeen,
				)

				if r.onStatusChange != nil {
					r.onStatusChange(backend, oldStatus, StatusStale)
				}
			}
		}
	}

	// Remove expired backends
	for _, key := range toRemove {
		backend := r.backends[key]
		delete(r.backends, key)

		r.config.Logger.Info("stale backend removed",
			"service", backend.Service,
			"address", backend.Address,
			"port", backend.Port,
		)

		if r.store != nil {
			if err := r.store.Delete(r.ctx, storeKey(key)); err != nil {
				r.config.Logger.Warn("failed to delete backend from store", "key", key, "error", err)
			}
		}

		if r.onStatusChange != nil {
			r.onStatusChange(backend, backend.EffectiveStatus, "")
		}
	}
}

// persistBackend saves a backend to the store.
func (r *Registry) persistBackend(backend *Backend) error {
	if r.store == nil {
		return nil
	}

	key := backendKey(backend.Service, backend.Address, backend.Port)
	data, err := json.Marshal(backend)
	if err != nil {
		return fmt.Errorf("failed to marshal backend: %w", err)
	}

	return r.store.Set(r.ctx, storeKey(key), data)
}

// loadFromStore loads backends from the store.
func (r *Registry) loadFromStore() error {
	if r.store == nil {
		return nil
	}

	pairs, err := r.store.List(r.ctx, "backends/")
	if err != nil {
		return fmt.Errorf("failed to list backends: %w", err)
	}

	for _, pair := range pairs {
		var backend Backend
		if err := json.Unmarshal(pair.Value, &backend); err != nil {
			r.config.Logger.Warn("failed to unmarshal backend", "key", pair.Key, "error", err)
			continue
		}

		key := backendKey(backend.Service, backend.Address, backend.Port)
		r.backends[key] = &backend

		// Recompute effective status
		r.computeEffectiveStatus(&backend)
	}

	r.config.Logger.Info("loaded backends from store", "count", len(r.backends))
	return nil
}

// backendKey generates a unique key for a backend.
func backendKey(service, address string, port int) string {
	return fmt.Sprintf("%s:%s:%d", service, address, port)
}

// storeKey generates the store key for a backend.
func storeKey(backendKey string) string {
	return "backends/" + backendKey
}
