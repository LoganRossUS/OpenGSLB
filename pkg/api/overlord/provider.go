// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// DefaultDataProvider implements DataProvider for the Overlord API.
// It provides access to OpenGSLB's configuration and runtime state.
type DefaultDataProvider struct {
	config      *config.Config
	configMu    sync.RWMutex
	registry    *overwatch.Registry
	validator   *overwatch.Validator
}

// NewDefaultDataProvider creates a new DefaultDataProvider.
func NewDefaultDataProvider(cfg *config.Config, registry *overwatch.Registry, validator *overwatch.Validator) *DefaultDataProvider {
	return &DefaultDataProvider{
		config:    cfg,
		registry:  registry,
		validator: validator,
	}
}

// GetConfig returns the current configuration.
func (p *DefaultDataProvider) GetConfig() *config.Config {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config
}

// UpdateConfig updates the configuration.
func (p *DefaultDataProvider) UpdateConfig(cfg *config.Config) error {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	p.config = cfg
	return nil
}

// GetBackendRegistry returns the backend registry.
func (p *DefaultDataProvider) GetBackendRegistry() *overwatch.Registry {
	return p.registry
}

// GetValidator returns the external validator.
func (p *DefaultDataProvider) GetValidator() *overwatch.Validator {
	return p.validator
}

// SetConfig updates the configuration.
func (p *DefaultDataProvider) SetConfig(cfg *config.Config) {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	p.config = cfg
}

// SetRegistry updates the backend registry.
func (p *DefaultDataProvider) SetRegistry(registry *overwatch.Registry) {
	p.registry = registry
}

// SetValidator updates the external validator.
func (p *DefaultDataProvider) SetValidator(validator *overwatch.Validator) {
	p.validator = validator
}
