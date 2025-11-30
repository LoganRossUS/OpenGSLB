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
		Timeout: entry.config.Timeout,
	}

	result := m.checker.Check(ctx, target)

	statusChanged := entry.health.RecordResult(result)

	if statusChanged {
		newStatus := entry.health.Status()
		log.Printf("health status changed: %s -> %s", entry.health.Address(), newStatus)

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
