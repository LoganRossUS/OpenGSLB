// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"log/slog"
	"sync"
	"time"
	// ADR-015: Removed pkg/cluster import - cluster package deleted
)

// PredictiveState represents the current predictive health state.
type PredictiveState struct {
	Bleeding    bool      `json:"bleeding"`
	BleedReason string    `json:"bleed_reason,omitempty"`
	BleedingAt  time.Time `json:"bleeding_at,omitempty"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemPercent  float64   `json:"mem_percent"`
	ErrorRate   float64   `json:"error_rate"`
	LastCheck   time.Time `json:"last_check"`
}

// PredictiveConfig holds configuration for predictive health monitoring.
type PredictiveConfig struct {
	Enabled            bool          `yaml:"enabled"`
	CPUThreshold       float64       `yaml:"cpu_threshold"`
	MemoryThreshold    float64       `yaml:"memory_threshold"`
	ErrorRateThreshold float64       `yaml:"error_rate_threshold"`
	CheckInterval      time.Duration `yaml:"check_interval"`
}

// Predictor monitors local system metrics and predicts failures.
type Predictor struct {
	config   PredictiveConfig
	logger   *slog.Logger
	monitor  SystemMonitor
	state    PredictiveState
	mu       sync.RWMutex
	stopCh   chan struct{}
	doneCh   chan struct{}
	callback func(PredictiveState)
}

// SystemMonitor provides system metrics.
type SystemMonitor interface {
	CPUPercent() (float64, error)
	MemoryPercent() (float64, error)
	ErrorRate() (float64, error)
}

// NewPredictor creates a new predictive health monitor.
func NewPredictor(cfg PredictiveConfig, monitor SystemMonitor, logger *slog.Logger) *Predictor {
	return &Predictor{
		config:  cfg,
		logger:  logger,
		monitor: monitor,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start begins predictive monitoring.
func (p *Predictor) Start() {
	if !p.config.Enabled {
		p.logger.Info("predictive health monitoring disabled")
		close(p.doneCh)
		return
	}

	go p.monitorLoop()
}

// Stop stops predictive monitoring.
func (p *Predictor) Stop() {
	close(p.stopCh)
	<-p.doneCh
}

// SetCallback sets the function to call when state changes.
func (p *Predictor) SetCallback(cb func(PredictiveState)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callback = cb
}

// GetState returns the current predictive state.
func (p *Predictor) GetState() PredictiveState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

// IsBleeding returns true if the system is in a bleeding state.
func (p *Predictor) IsBleeding() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state.Bleeding
}

func (p *Predictor) monitorLoop() {
	defer close(p.doneCh)

	ticker := time.NewTicker(p.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.checkMetrics()
		}
	}
}

func (p *Predictor) checkMetrics() {
	cpu, cpuErr := p.monitor.CPUPercent()
	mem, memErr := p.monitor.MemoryPercent()
	errRate, errRateErr := p.monitor.ErrorRate()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Update metrics even if some failed
	if cpuErr == nil {
		p.state.CPUPercent = cpu
	}
	if memErr == nil {
		p.state.MemPercent = mem
	}
	if errRateErr == nil {
		p.state.ErrorRate = errRate
	}
	p.state.LastCheck = time.Now()

	// Check thresholds
	wasBleeding := p.state.Bleeding
	bleedReason := ""

	if cpuErr == nil && cpu >= p.config.CPUThreshold {
		bleedReason = "cpu_threshold_exceeded"
	} else if memErr == nil && mem >= p.config.MemoryThreshold {
		bleedReason = "memory_threshold_exceeded"
	} else if errRateErr == nil && errRate >= p.config.ErrorRateThreshold {
		bleedReason = "error_rate_threshold_exceeded"
	}

	if bleedReason != "" {
		if !p.state.Bleeding {
			p.state.Bleeding = true
			p.state.BleedReason = bleedReason
			p.state.BleedingAt = time.Now()
			p.logger.Warn("predictive bleed triggered",
				"reason", bleedReason,
				"cpu", cpu,
				"memory", mem,
				"error_rate", errRate)
		}
	} else {
		if p.state.Bleeding {
			p.logger.Info("predictive bleed cleared",
				"was_reason", p.state.BleedReason,
				"cpu", cpu,
				"memory", mem,
				"error_rate", errRate)
		}
		p.state.Bleeding = false
		p.state.BleedReason = ""
		p.state.BleedingAt = time.Time{}
	}

	// Notify callback if state changed
	if wasBleeding != p.state.Bleeding && p.callback != nil {
		stateCopy := p.state
		go p.callback(stateCopy)
	}
}
