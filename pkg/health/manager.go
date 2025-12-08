// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package health

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ServerConfig defines the health check configuration for a server.
type ServerConfig struct {
	Address  string
	Port     int
	Path     string        // For HTTP checks
	Scheme   string        // http or https
	Host     string        // Host header for HTTPS (for TLS SNI)
	Interval time.Duration // Check interval
	Timeout  time.Duration // Per-check timeout
}

// ManagerConfig configures the health check manager.
type ManagerConfig struct {
	// FailThreshold is the number of consecutive failures before marking unhealthy.
	FailThreshold int

	// PassThreshold is the number of consecutive successes before marking healthy.
	PassThreshold int

	// DefaultInterval is the default check interval if not specified per-server.
	DefaultInterval time.Duration

	// DefaultTimeout is the default timeout if not specified per-server.
	DefaultTimeout time.Duration
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		FailThreshold:   3,
		PassThreshold:   2,
		DefaultInterval: 30 * time.Second,
		DefaultTimeout:  5 * time.Second,
	}
}

// Manager orchestrates health checks for multiple servers.
// It implements the HealthStatusProvider interface expected by the DNS handler.
type Manager struct {
	config   ManagerConfig
	checker  Checker
	servers  map[string]*serverEntry
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	wg       sync.WaitGroup
	onChange func(address string, status Status) // Optional callback
}

type serverEntry struct {
	config ServerConfig
	health *ServerHealth
	stopCh chan struct{}
}

// NewManager creates a new health check manager.
func NewManager(checker Checker, config ManagerConfig) *Manager {
	return &Manager{
		config:  config,
		checker: checker,
		servers: make(map[string]*serverEntry),
		stopCh:  make(chan struct{}),
	}
}

// OnStatusChange sets a callback for health status changes.
func (m *Manager) OnStatusChange(fn func(address string, status Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// AddServer registers a server for health checking.
func (m *Manager) AddServer(cfg ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := serverKey(cfg.Address, cfg.Port)
	if _, exists := m.servers[key]; exists {
		return fmt.Errorf("server %s already registered", key)
	}

	// Apply defaults
	if cfg.Interval == 0 {
		cfg.Interval = m.config.DefaultInterval
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = m.config.DefaultTimeout
	}

	entry := &serverEntry{
		config: cfg,
		health: NewServerHealth(key, m.config.FailThreshold, m.config.PassThreshold),
		stopCh: make(chan struct{}),
	}
	m.servers[key] = entry

	// If manager is already running, start checking this server
	if m.running {
		m.wg.Add(1)
		go m.checkLoop(entry)
	}

	return nil
}

// RemoveServer unregisters a server from health checking.
func (m *Manager) RemoveServer(address string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := serverKey(address, port)
	entry, exists := m.servers[key]
	if !exists {
		return fmt.Errorf("server %s not found", key)
	}

	// Stop the check loop for this server
	close(entry.stopCh)
	delete(m.servers, key)

	return nil
}

// Start begins health checking all registered servers.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return errors.New("manager already running")
	}

	m.running = true
	m.stopCh = make(chan struct{})

	for _, entry := range m.servers {
		m.wg.Add(1)
		go m.checkLoop(entry)
	}

	log.Printf("health manager started, monitoring %d servers", len(m.servers))
	return nil
}

// Stop halts all health checking.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	// Wait for all check loops to finish
	m.wg.Wait()
	log.Println("health manager stopped")
	return nil
}

// checkLoop runs periodic health checks for a single server.
func (m *Manager) checkLoop(entry *serverEntry) {
	defer m.wg.Done()

	ticker := time.NewTicker(entry.config.Interval)
	defer ticker.Stop()

	// Run an immediate check
	m.performCheck(entry)

	for {
		select {
		case <-m.stopCh:
			return
		case <-entry.stopCh:
			return
		case <-ticker.C:
			m.performCheck(entry)
		}
	}
}

// performCheck executes a single health check and updates status.
func (m *Manager) performCheck(entry *serverEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), entry.config.Timeout)
	defer cancel()

	target := Target{
		Address: entry.config.Address,
		Port:    entry.config.Port,
		Path:    entry.config.Path,
		Scheme:  entry.config.Scheme,
		Host:    entry.config.Host,
		Timeout: entry.config.Timeout,
	}

	result := m.checker.Check(ctx, target)

	// Log detailed error information for failed health checks
	if !result.Healthy && result.Error != nil {
		log.Printf("health check failed: %s reason=%v latency=%v",
			entry.health.Address(), result.Error, result.Latency)
	}

	statusChanged := entry.health.RecordResult(result)

	if statusChanged {
		newStatus := entry.health.Status()
		if newStatus == StatusUnhealthy && result.Error != nil {
			log.Printf("health status changed: %s -> %s reason=%v",
				entry.health.Address(), newStatus, result.Error)
		} else {
			log.Printf("health status changed: %s -> %s", entry.health.Address(), newStatus)
		}

		m.mu.RLock()
		onChange := m.onChange
		m.mu.RUnlock()

		if onChange != nil {
			onChange(entry.health.Address(), newStatus)
		}
	}
}

// IsHealthy returns true if the specified server is healthy.
// This implements part of the HealthStatusProvider interface.
func (m *Manager) IsHealthy(address string, port int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := serverKey(address, port)
	entry, exists := m.servers[key]
	if !exists {
		return false
	}
	return entry.health.IsHealthy()
}

// GetHealthyServers returns addresses of all healthy servers.
func (m *Manager) GetHealthyServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []string
	for _, entry := range m.servers {
		if entry.health.IsHealthy() {
			healthy = append(healthy, entry.health.Address())
		}
	}
	return healthy
}

// GetStatus returns the health status of a specific server.
func (m *Manager) GetStatus(address string, port int) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := serverKey(address, port)
	entry, exists := m.servers[key]
	if !exists {
		return Snapshot{}, false
	}
	return entry.health.Snapshot(), true
}

// GetAllStatus returns snapshots of all server health states.
func (m *Manager) GetAllStatus() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]Snapshot, 0, len(m.servers))
	for _, entry := range m.servers {
		snapshots = append(snapshots, entry.health.Snapshot())
	}
	return snapshots
}

// ServerCount returns the number of registered servers.
func (m *Manager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.servers)
}

func serverKey(address string, port int) string {
	return fmt.Sprintf("%s:%d", address, port)
}

// Reconfigure updates the health manager with a new set of servers.
// It compares the new configuration with the current state and:
//   - Stops health checks for removed servers
//   - Starts health checks for added servers
//   - Updates configuration for existing servers (restarts their check loops)
//
// Returns the number of servers added, removed, and updated.
func (m *Manager) Reconfigure(newServers []ServerConfig) (added, removed, updated int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build map of new servers for quick lookup
	newServerMap := make(map[string]ServerConfig, len(newServers))
	for _, cfg := range newServers {
		// Apply defaults
		if cfg.Interval == 0 {
			cfg.Interval = m.config.DefaultInterval
		}
		if cfg.Timeout == 0 {
			cfg.Timeout = m.config.DefaultTimeout
		}
		key := serverKey(cfg.Address, cfg.Port)
		newServerMap[key] = cfg
	}

	// Find servers to remove (in current but not in new)
	var toRemove []string
	for key := range m.servers {
		if _, exists := newServerMap[key]; !exists {
			toRemove = append(toRemove, key)
		}
	}

	// Remove old servers
	for _, key := range toRemove {
		entry := m.servers[key]
		close(entry.stopCh)
		delete(m.servers, key)
		removed++
	}

	// Add or update servers
	for key, cfg := range newServerMap {
		if entry, exists := m.servers[key]; exists {
			// Server exists - check if config changed
			if configChanged(entry.config, cfg) {
				// Stop old check loop
				close(entry.stopCh)

				// Create new entry with updated config
				newEntry := &serverEntry{
					config: cfg,
					health: entry.health, // Preserve health state
					stopCh: make(chan struct{}),
				}
				m.servers[key] = newEntry

				// Start new check loop if manager is running
				if m.running {
					m.wg.Add(1)
					go m.checkLoop(newEntry)
				}
				updated++
			}
			// If config hasn't changed, leave it alone
		} else {
			// New server - add it
			entry := &serverEntry{
				config: cfg,
				health: NewServerHealth(key, m.config.FailThreshold, m.config.PassThreshold),
				stopCh: make(chan struct{}),
			}
			m.servers[key] = entry

			// Start check loop if manager is running
			if m.running {
				m.wg.Add(1)
				go m.checkLoop(entry)
			}
			added++
		}
	}

	return added, removed, updated
}

// configChanged returns true if the server configuration has changed
// in a way that requires restarting the health check loop.
func configChanged(old, new ServerConfig) bool {
	return old.Path != new.Path ||
		old.Scheme != new.Scheme ||
		old.Host != new.Host ||
		old.Interval != new.Interval ||
		old.Timeout != new.Timeout
}
