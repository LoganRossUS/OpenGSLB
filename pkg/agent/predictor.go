// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/cluster"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/metrics"
)

// Signal types for predictive health.
const (
	// SignalBleed indicates the node should have traffic gradually reduced.
	SignalBleed = "bleed"

	// SignalClear indicates the node has recovered and can resume normal traffic.
	SignalClear = "clear"
)

// Reason codes for signals.
const (
	ReasonCPUHigh       = "cpu_high"
	ReasonMemoryHigh    = "memory_high"
	ReasonErrorRateHigh = "error_rate_high"
	ReasonRecovered     = "recovered"
)

// Predictor evaluates metrics against thresholds and generates predictive signals.
// It implements the "predictive from the inside" philosophy by monitoring local
// system resources and signaling when thresholds are exceeded.
type Predictor struct {
	config  config.PredictiveHealthConfig
	nodeID  string
	monitor *Monitor
	logger  *slog.Logger

	// Current state
	mu          sync.RWMutex
	bleeding    bool
	bleedReason string
	bleedValue  float64

	// Callback for signal changes
	onSignal func(*cluster.PredictiveSignal)

	// Evaluation interval
	interval time.Duration
}

// NewPredictor creates a new Predictor for threshold evaluation.
func NewPredictor(cfg config.PredictiveHealthConfig, nodeID string, monitor *Monitor, logger *slog.Logger) *Predictor {
	if logger == nil {
		logger = slog.Default()
	}

	return &Predictor{
		config:   cfg,
		nodeID:   nodeID,
		monitor:  monitor,
		logger:   logger,
		interval: 5 * time.Second, // Default evaluation interval
	}
}

// OnSignal sets the callback function to invoke when a signal is generated.
// This is typically wired to gossip.BroadcastPredictive.
func (p *Predictor) OnSignal(fn func(*cluster.PredictiveSignal)) {
	p.onSignal = fn
}

// SetInterval sets the evaluation interval.
func (p *Predictor) SetInterval(d time.Duration) {
	if d > 0 {
		p.interval = d
	}
}

// Start begins the evaluation loop. It runs until the context is canceled.
func (p *Predictor) Start(ctx context.Context) error {
	if !p.config.Enabled {
		p.logger.Info("predictive health monitoring disabled")
		return nil
	}

	p.logger.Info("starting predictive health monitoring",
		"cpu_threshold", p.config.CPU.Threshold,
		"memory_threshold", p.config.Memory.Threshold,
		"error_rate_threshold", p.config.ErrorRate.Threshold,
	)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("stopping predictive health monitoring")
			return ctx.Err()
		case <-ticker.C:
			signal, err := p.Evaluate()
			if err != nil {
				p.logger.Error("failed to evaluate metrics", "error", err)
				continue
			}

			if signal != nil && p.onSignal != nil {
				p.onSignal(signal)
			}

			// Update Prometheus metrics
			p.updateMetrics()
		}
	}
}

// Evaluate checks current metrics against thresholds and returns a signal if warranted.
// Returns nil if no signal change is needed.
func (p *Predictor) Evaluate() (*cluster.PredictiveSignal, error) {
	metrics, err := p.monitor.Collect()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check thresholds
	cpuExceeded := metrics.CPUPercent > p.config.CPU.Threshold
	memExceeded := metrics.MemoryPercent > p.config.Memory.Threshold
	errorExceeded := metrics.ErrorRate > p.config.ErrorRate.Threshold

	anyExceeded := cpuExceeded || memExceeded || errorExceeded

	// Determine if state has changed
	if anyExceeded && !p.bleeding {
		// Transitioned to bleeding state
		p.bleeding = true
		reason, value, threshold := p.determineReason(metrics, cpuExceeded, memExceeded, errorExceeded)
		p.bleedReason = reason
		p.bleedValue = value

		p.logger.Warn("predictive health threshold exceeded, signaling bleed",
			"reason", reason,
			"value", value,
			"threshold", threshold,
		)

		return &cluster.PredictiveSignal{
			NodeID:    p.nodeID,
			Signal:    SignalBleed,
			Reason:    reason,
			Value:     value,
			Threshold: threshold,
		}, nil
	} else if !anyExceeded && p.bleeding {
		// Recovered from bleeding state
		p.bleeding = false
		previousReason := p.bleedReason
		p.bleedReason = ""
		p.bleedValue = 0

		p.logger.Info("predictive health recovered, clearing signal",
			"previous_reason", previousReason,
		)

		return &cluster.PredictiveSignal{
			NodeID:    p.nodeID,
			Signal:    SignalClear,
			Reason:    ReasonRecovered,
			Value:     0,
			Threshold: 0,
		}, nil
	}

	// No state change
	return nil, nil
}

// determineReason returns the reason, current value, and threshold for a bleed signal.
// Priority: CPU > Memory > ErrorRate (first exceeded wins)
func (p *Predictor) determineReason(metrics *Metrics, cpu, mem, errorRate bool) (string, float64, float64) {
	if cpu {
		return ReasonCPUHigh, metrics.CPUPercent, p.config.CPU.Threshold
	}
	if mem {
		return ReasonMemoryHigh, metrics.MemoryPercent, p.config.Memory.Threshold
	}
	if errorRate {
		return ReasonErrorRateHigh, metrics.ErrorRate, p.config.ErrorRate.Threshold
	}
	return "", 0, 0
}

// IsBleeding returns true if the predictor is currently signaling a bleed.
func (p *Predictor) IsBleeding() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.bleeding
}

// BleedReason returns the current bleed reason, or empty string if not bleeding.
func (p *Predictor) BleedReason() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.bleedReason
}

// GetMetrics returns the current metrics and state for monitoring purposes.
func (p *Predictor) GetMetrics() (*Metrics, bool, string) {
	metrics, err := p.monitor.Collect()
	if err != nil {
		return nil, false, ""
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return metrics, p.bleeding, p.bleedReason
}

// updateMetrics updates Prometheus metrics with current values.
func (p *Predictor) updateMetrics() {
	metricsData, bleeding, _ := p.GetMetrics()
	if metricsData != nil {
		// Using the pkg/metrics helper
		metrics.SetPredictiveMetrics(metricsData.CPUPercent, metricsData.MemoryPercent, metricsData.ErrorRate, bleeding)
	}
}
