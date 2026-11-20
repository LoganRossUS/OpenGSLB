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

	// tick is how often the validation loop looks for backends that are due.
	// Zero means the smaller of CheckInterval and one second. Tests set it.
	tick time.Duration
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

	// schedMu guards the schedule of the validation loop.
	schedMu sync.Mutex
	// lastProbe is when the loop last started a probe of a backend.
	lastProbe map[string]time.Time
	// inFlight holds the backends whose probe has not returned yet.
	inFlight map[string]bool
	// sem limits the number of concurrent probes started by the loop.
	sem chan struct{}
}

// maxConcurrentValidations is the number of probes that run at the same time.
const maxConcurrentValidations = 10

// NewValidator creates a new external health validator.
func NewValidator(cfg ValidatorConfig, registry *Registry, checker health.Checker) *Validator {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Validator{
		config:    cfg,
		registry:  registry,
		checker:   checker,
		lastProbe: make(map[string]time.Time),
		inFlight:  make(map[string]bool),
		sem:       make(chan struct{}, maxConcurrentValidations),
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

// validationLoop probes every backend when it is due. A backend is due when
// its interval has passed since its last probe: its own interval when one was
// set through the API, the node-wide CheckInterval otherwise. A backend that
// has never been probed is due at once, so a server created at runtime gets
// its first verdict at the next tick.
func (v *Validator) validationLoop() {
	defer v.wg.Done()

	// Run initial validation
	v.validateDueBackends()

	ticker := time.NewTicker(v.tickInterval())
	defer ticker.Stop()

	for {
		select {
		case <-v.ctx.Done():
			return
		case <-ticker.C:
			v.validateDueBackends()
		}
	}
}

// tickInterval is how often the loop looks for due backends.
func (v *Validator) tickInterval() time.Duration {
	if v.config.tick > 0 {
		return v.config.tick
	}
	tick := time.Second
	if v.config.CheckInterval > 0 && v.config.CheckInterval < tick {
		tick = v.config.CheckInterval
	}
	return tick
}

// intervalFor returns the probe interval of a backend.
func (v *Validator) intervalFor(backend *Backend) time.Duration {
	if backend.HealthCheck != nil && backend.HealthCheck.Interval > 0 {
		return backend.HealthCheck.Interval
	}
	return v.config.CheckInterval
}

// timeoutFor returns the probe timeout of a backend.
func (v *Validator) timeoutFor(backend *Backend) time.Duration {
	if backend.HealthCheck != nil && backend.HealthCheck.Timeout > 0 {
		return backend.HealthCheck.Timeout
	}
	return v.config.CheckTimeout
}

// checkDisabled reports whether validation is turned off for a backend.
func checkDisabled(backend *Backend) bool {
	return backend.HealthCheck != nil && backend.HealthCheck.Disabled
}

// validateDueBackends starts a probe for every backend that is due. It does
// not wait for the probes: a probe that runs into its timeout must not delay
// the probes of other backends. A backend whose previous probe has not
// returned is skipped, so one backend is never probed twice at once.
func (v *Validator) validateDueBackends() {
	backends := v.registry.GetAllBackends()
	now := time.Now()

	v.schedMu.Lock()
	// Forget backends that are gone.
	known := make(map[string]bool, len(backends))
	for _, backend := range backends {
		known[backendKey(backend.Service, backend.Address, backend.Port)] = true
	}
	for key := range v.lastProbe {
		if !known[key] {
			delete(v.lastProbe, key)
		}
	}

	var due []*Backend
	for _, backend := range backends {
		// Note: stale backends are probed too. External validation can
		// "recover" a backend whose agent is gone while the service still runs
		// (ADR-015: Overwatch validation takes precedence over staleness).
		if checkDisabled(backend) {
			continue
		}
		key := backendKey(backend.Service, backend.Address, backend.Port)
		if v.inFlight[key] {
			continue
		}
		if last, probed := v.lastProbe[key]; probed && now.Sub(last) < v.intervalFor(backend) {
			continue
		}
		v.lastProbe[key] = now
		v.inFlight[key] = true
		due = append(due, backend)
	}
	v.schedMu.Unlock()

	if len(due) == 0 {
		return
	}
	v.config.Logger.Debug("starting validation probes", "due", len(due), "backends", len(backends))

	for _, backend := range due {
		v.wg.Add(1)
		go func(b *Backend) {
			defer v.wg.Done()
			key := backendKey(b.Service, b.Address, b.Port)
			defer func() {
				v.schedMu.Lock()
				delete(v.inFlight, key)
				v.schedMu.Unlock()
			}()

			// Limit the number of concurrent probes.
			select {
			case v.sem <- struct{}{}:
			case <-v.ctx.Done():
				return
			}
			defer func() { <-v.sem }()

			v.validateSingleBackend(b)
		}(backend)
	}
}

// validateAllBackends validates all registered backends once and waits for
// the results. It is used for an explicit "validate now" request.
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
	sem := make(chan struct{}, maxConcurrentValidations)
	var wg sync.WaitGroup

	for _, backend := range backends {
		// Note: We validate ALL backends including stale ones.
		// This allows external validation to "recover" backends when:
		// - Agent process crashed but backend service is still running
		// - Network partition between agent and overwatch
		// - Agent temporarily unavailable
		if checkDisabled(backend) {
			continue
		}
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
	if checkDisabled(backend) {
		return
	}

	// Use context.Background() if v.ctx is nil (for direct ValidateNow calls)
	v.mu.RLock()
	baseCtx := v.ctx
	v.mu.RUnlock()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	timeout := v.timeoutFor(backend)
	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	// Build the check target. A backend with its own health check (set
	// through the API) is probed with it; any other backend with its
	// configured check type and the node-wide timeout.
	// v1.1.1: Use HealthCheckType from backend to fix latency routing with TCP health checks
	scheme := backend.CheckType()
	target := health.Target{
		Address: backend.Address,
		Port:    backend.Port,
		Scheme:  scheme,
		Timeout: timeout,
	}
	if hc := backend.HealthCheck; hc != nil && scheme != "tcp" {
		target.Path = hc.Path
		target.Host = hc.Host
	}

	// Perform the health check
	result := v.checker.Check(ctx, target)

	// v1.1.1: Debug log to help diagnose latency routing issues
	v.config.Logger.Debug("validation check completed",
		"service", backend.Service,
		"address", backend.Address,
		"port", backend.Port,
		"check_type", scheme,
		"healthy", result.Healthy,
		"latency_ms", result.Latency.Milliseconds(),
	)

	var validationErr string
	if result.Error != nil {
		validationErr = result.Error.Error()
	}

	// The check may have been turned off or the backend removed while the
	// probe was running. A verdict that arrives after that is dropped.
	if current, ok := v.registry.GetBackend(backend.Service, backend.Address, backend.Port); !ok || checkDisabled(current) {
		return
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

	// Report a disagreement with the agent, if the backend has one.
	if after, ok := v.registry.GetBackend(backend.Service, backend.Address, backend.Port); ok {
		v.reportDisagreement(backend, after, validationErr)
	}
}

// reportDisagreement logs when the validation verdict contradicts an agent's
// claim. Only agent-registered backends make a claim: static and API backends
// never report, so in standalone mode there is nothing to disagree with and
// nothing is logged. For agent backends the disagreement is logged once, when
// the verdict changes, and at debug level on the following cycles while it
// persists.
func (v *Validator) reportDisagreement(before, after *Backend, validationErr string) {
	if after.Source != SourceAgent || after.ValidationHealthy == nil {
		return
	}
	verdict := *after.ValidationHealthy
	if after.AgentHealthy == verdict {
		return
	}

	attrs := []any{
		"service", after.Service,
		"address", after.Address,
		"port", after.Port,
		"agent_id", after.AgentID,
		"agent_healthy", after.AgentHealthy,
		"validation_healthy", verdict,
		"validation_error", validationErr,
	}

	verdictChanged := before.ValidationHealthy == nil || *before.ValidationHealthy != verdict
	if verdictChanged {
		v.config.Logger.Info("validation disagrees with agent", attrs...)
		RecordValidationDisagreement(after.Service, after.AgentHealthy, verdict)
		return
	}
	v.config.Logger.Debug("validation still disagrees with agent", attrs...)
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

			// Check for disagreement (only agents make a claim to disagree with)
			if backend.Source == SourceAgent && backend.AgentHealthy != *backend.ValidationHealthy {
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
