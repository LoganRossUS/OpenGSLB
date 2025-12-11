// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// ValidatorConfig configures the external health validator.
type ValidatorConfig struct {
	// Enabled controls whether external validation is active.
	Enabled bool

	// CheckInterval is the frequency of validation checks.
	CheckInterval time.Duration

	// CheckTimeout is the timeout for validation checks.
	CheckTimeout time.Duration

	// Logger for validator operations.
	Logger *slog.Logger
}

// DefaultValidatorConfig returns sensible defaults.
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		Enabled:       true,
		CheckInterval: 30 * time.Second,
		CheckTimeout:  5 * time.Second,
		Logger:        slog.Default(),
	}
}

// Validator performs external health validation of agent-registered backends.
// ADR-015: Overwatch validation ALWAYS wins over agent claims.
type Validator struct {
	config   ValidatorConfig
	registry *Registry
	checker  health.Checker

	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewValidator creates a new external health validator.
func NewValidator(cfg ValidatorConfig, registry *Registry, checker health.Checker) *Validator {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Validator{
		config:   cfg,
		registry: registry,
		checker:  checker,
	}
}

// Start begins the validation loop.
func (v *Validator) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return fmt.Errorf("validator already running")
	}

	if !v.config.Enabled {
		v.config.Logger.Info("external validation disabled")
		return nil
	}

	v.ctx, v.cancel = context.WithCancel(context.Background())
	v.running = true

	v.wg.Add(1)
	go v.validationLoop()

	v.config.Logger.Info("external validator started",
		"check_interval", v.config.CheckInterval,
		"check_timeout", v.config.CheckTimeout,
	)
	return nil
}

// Stop halts the validation loop.
func (v *Validator) Stop() error {
	v.mu.Lock()
	if !v.running {
		v.mu.Unlock()
		return nil
	}
	v.running = false
	v.cancel()
	v.mu.Unlock()

	v.wg.Wait()
	v.config.Logger.Info("external validator stopped")
	return nil
}

// IsRunning returns whether the validator is running.
func (v *Validator) IsRunning() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.running
}

// ValidateNow triggers an immediate validation of all backends.
func (v *Validator) ValidateNow() {
	v.validateAllBackends()
}

// ValidateBackend validates a specific backend immediately.
func (v *Validator) ValidateBackend(service, address string, port int) error {
	backend, exists := v.registry.GetBackend(service, address, port)
	if !exists {
		return fmt.Errorf("backend not found: %s:%s:%d", service, address, port)
	}

	v.validateSingleBackend(backend)
	return nil
}

// validationLoop runs periodic validation of all backends.
func (v *Validator) validationLoop() {
	defer v.wg.Done()

	// Run initial validation
	v.validateAllBackends()

	ticker := time.NewTicker(v.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-v.ctx.Done():
			return
		case <-ticker.C:
			v.validateAllBackends()
		}
	}
}

// validateAllBackends validates all registered backends.
// Note: Stale backends are NOT skipped. External validation can "recover" a
// stale backend if the agent is unavailable but the backend service is still healthy.
// This aligns with ADR-015's health authority hierarchy where Overwatch validation
// takes precedence over staleness detection.
func (v *Validator) validateAllBackends() {
	backends := v.registry.GetAllBackends()
	if len(backends) == 0 {
		return
	}

	v.config.Logger.Debug("starting validation cycle", "backends", len(backends))

	// Use a semaphore to limit concurrent validations
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, backend := range backends {
		// Note: We validate ALL backends including stale ones.
		// This allows external validation to "recover" backends when:
		// - Agent process crashed but backend service is still running
		// - Network partition between agent and overwatch
		// - Agent temporarily unavailable
		wg.Add(1)
		sem <- struct{}{}

		go func(b *Backend) {
			defer wg.Done()
			defer func() { <-sem }()
			v.validateSingleBackend(b)
		}(backend)
	}

	wg.Wait()
	v.config.Logger.Debug("validation cycle complete")
}

// validateSingleBackend validates a single backend.
func (v *Validator) validateSingleBackend(backend *Backend) {
	// Use context.Background() if v.ctx is nil (for direct ValidateNow calls)
	baseCtx := v.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, v.config.CheckTimeout)
	defer cancel()

	// Build the check target
	target := health.Target{
		Address: backend.Address,
		Port:    backend.Port,
		Scheme:  "http", // Default to HTTP
		Timeout: v.config.CheckTimeout,
	}

	// Perform the health check
	result := v.checker.Check(ctx, target)

	var validationErr string
	if result.Error != nil {
		validationErr = result.Error.Error()
	}

	// Update the registry with validation result including latency
	if err := v.registry.UpdateValidationWithLatency(
		backend.Service,
		backend.Address,
		backend.Port,
		result.Healthy,
		validationErr,
		result.Latency,
	); err != nil {
		v.config.Logger.Warn("failed to update validation result",
			"service", backend.Service,
			"address", backend.Address,
			"port", backend.Port,
			"error", err,
		)
		return
	}

	// Record metrics
	RecordValidationResult(backend.Service, backend.Address, backend.Port, result.Healthy)
	RecordValidationLatency(backend.Service, backend.Address, backend.Port, result.Latency)

	// Log if validation disagrees with agent
	if backend.AgentHealthy != result.Healthy {
		v.config.Logger.Info("validation disagrees with agent",
			"service", backend.Service,
			"address", backend.Address,
			"port", backend.Port,
			"agent_healthy", backend.AgentHealthy,
			"validation_healthy", result.Healthy,
			"validation_error", validationErr,
		)
	}
}

// GetValidationStats returns validation statistics.
func (v *Validator) GetValidationStats() ValidationStats {
	backends := v.registry.GetAllBackends()

	stats := ValidationStats{
		TotalBackends: len(backends),
	}

	for _, backend := range backends {
		if backend.ValidationHealthy != nil {
			stats.ValidatedBackends++
			if *backend.ValidationHealthy {
				stats.HealthyBackends++
			} else {
				stats.UnhealthyBackends++
			}

			// Check for disagreement
			if backend.AgentHealthy != *backend.ValidationHealthy {
				stats.DisagreementCount++
			}
		}

		if backend.EffectiveStatus == StatusStale {
			stats.StaleBackends++
		}
	}

	return stats
}

// ValidationStats contains validation statistics.
type ValidationStats struct {
	TotalBackends     int `json:"total_backends"`
	ValidatedBackends int `json:"validated_backends"`
	HealthyBackends   int `json:"healthy_backends"`
	UnhealthyBackends int `json:"unhealthy_backends"`
	StaleBackends     int `json:"stale_backends"`
	DisagreementCount int `json:"disagreement_count"`
}
