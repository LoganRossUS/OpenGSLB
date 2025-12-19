//go:build linux

// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// TCP connection state constants from Linux kernel
const (
	tcpEstablished = 1 // TCP_ESTABLISHED state value
)

// linuxCollector implements the Collector interface for Linux using netlink INET_DIAG.
// This uses the same kernel interface as `ss -ti`.
type linuxCollector struct {
	config       CollectorConfig
	ports        map[uint16]bool
	observations chan Observation

	mu      sync.Mutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// newPlatformCollector creates a Linux-specific collector.
func newPlatformCollector(cfg CollectorConfig) (Collector, error) {
	// Check if we can access TCP info (requires CAP_NET_ADMIN)
	if err := checkLinuxCapabilities(); err != nil {
		return nil, err
	}

	ports := make(map[uint16]bool)
	for _, p := range cfg.Ports {
		ports[p] = true
	}

	return &linuxCollector{
		config:       cfg,
		ports:        ports,
		observations: make(chan Observation, 1000),
	}, nil
}

// checkLinuxCapabilities verifies we have CAP_NET_ADMIN.
func checkLinuxCapabilities() error {
	// Try a test query to see if we have permissions
	_, err := netlink.SocketDiagTCPInfo(unix.AF_INET)
	if err != nil {
		return ErrInsufficientPrivileges
	}
	return nil
}

// Start begins collecting RTT data.
func (c *linuxCollector) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true

	c.wg.Add(1)
	go c.pollLoop()

	return nil
}

// Observations returns the channel of collected RTT observations.
func (c *linuxCollector) Observations() <-chan Observation {
	return c.observations
}

// Close stops the collector and releases resources.
func (c *linuxCollector) Close() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.cancel()
	c.mu.Unlock()

	c.wg.Wait()
	close(c.observations)
	return nil
}

// pollLoop periodically queries the kernel for TCP connection info.
func (c *linuxCollector) pollLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()

	// Do an initial poll immediately
	c.pollOnce()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.pollOnce()
		}
	}
}

// pollOnce queries the kernel for all TCP connections and extracts RTT.
func (c *linuxCollector) pollOnce() {
	now := time.Now()

	// Query IPv4 TCP connections
	c.pollFamily(unix.AF_INET, now)

	// Query IPv6 TCP connections
	c.pollFamily(unix.AF_INET6, now)
}

// pollFamily queries TCP connections for a specific address family.
func (c *linuxCollector) pollFamily(family uint8, now time.Time) {
	sockets, err := netlink.SocketDiagTCPInfo(family)
	if err != nil {
		// Log error but don't crash - might be transient
		return
	}

	for _, sock := range sockets {
		// Filter: only ESTABLISHED connections
		if sock.InetDiagMsg.State != tcpEstablished {
			continue
		}

		// Filter: only our backend ports (if configured)
		localPort := sock.InetDiagMsg.ID.SourcePort
		if len(c.ports) > 0 && !c.ports[localPort] {
			continue
		}

		// Extract RTT (in microseconds)
		if sock.TCPInfo == nil {
			continue
		}
		rttUs := sock.TCPInfo.Rtt
		if rttUs == 0 {
			continue
		}

		// Convert remote address to netip.Addr
		var remoteAddr netip.Addr
		if family == unix.AF_INET {
			// IPv4: first 4 bytes of destination
			remoteAddr = netip.AddrFrom4([4]byte(sock.InetDiagMsg.ID.Destination[:4]))
		} else {
			// IPv6: full 16 bytes
			remoteAddr = netip.AddrFrom16([16]byte(sock.InetDiagMsg.ID.Destination))
		}

		// Handle IPv4-mapped IPv6 addresses: normalize to IPv4
		if remoteAddr.Is4In6() {
			remoteAddr = remoteAddr.Unmap()
		}

		// Skip loopback addresses
		if remoteAddr.IsLoopback() {
			continue
		}

		// Convert RTT from microseconds to Duration
		rtt := time.Duration(rttUs) * time.Microsecond

		obs := Observation{
			RemoteAddr: remoteAddr,
			LocalPort:  localPort,
			RTT:        rtt,
			Timestamp:  now,
		}

		// Non-blocking send to avoid blocking the poll loop
		select {
		case c.observations <- obs:
		default:
			// Channel full, drop observation
			observationsDroppedTotal.Inc()
		}
	}
}
