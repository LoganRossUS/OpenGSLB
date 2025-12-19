//go:build windows

// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

import (
	"context"
	"encoding/binary"
	"net/netip"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows API constants
const (
	// TCP connection states
	tcpEstablished = 5

	// TCP extended stats type for path (RTT) data
	tcpConnectionEstatsPath = 3
)

var (
	iphlpapi                      = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetTcpTable2              = iphlpapi.NewProc("GetTcpTable2")
	procGetPerTcpConnectionEStats = iphlpapi.NewProc("GetPerTcpConnectionEStats")
)

// MIB_TCP_STATE enumeration
const (
	MIB_TCP_STATE_CLOSED     = 1
	MIB_TCP_STATE_LISTEN     = 2
	MIB_TCP_STATE_SYN_SENT   = 3
	MIB_TCP_STATE_SYN_RCVD   = 4
	MIB_TCP_STATE_ESTAB      = 5
	MIB_TCP_STATE_FIN_WAIT1  = 6
	MIB_TCP_STATE_FIN_WAIT2  = 7
	MIB_TCP_STATE_CLOSE_WAIT = 8
	MIB_TCP_STATE_CLOSING    = 9
	MIB_TCP_STATE_LAST_ACK   = 10
	MIB_TCP_STATE_TIME_WAIT  = 11
	MIB_TCP_STATE_DELETE_TCB = 12
)

// MIB_TCPROW2 structure
type mibTcpRow2 struct {
	dwState        uint32
	dwLocalAddr    uint32
	dwLocalPort    uint32
	dwRemoteAddr   uint32
	dwRemotePort   uint32
	dwOwningPid    uint32
	dwOffloadState uint32
}

// TCP_ESTATS_PATH_ROD_v0 structure - contains RTT data
type tcpEstatsPathRod struct {
	FastRetran               uint32
	Timeouts                 uint32
	SubnetMaskRcv            uint32
	CurRetxQueue             uint32
	MaxRetxQueue             uint32
	CurAppWQueue             uint32
	MaxAppWQueue             uint32
	CountRtt                 uint32
	SumRtt                   uint32
	SmoothedRtt              uint32 // This is what we want (in milliseconds)
	RttVar                   uint32
	MinRtt                   uint32
	MaxRtt                   uint32
	CurMss                   uint32
	MaxMss                   uint32
	MinMss                   uint32
	SpuriousRtoDetections    uint32
	CurCwnd                  uint32
	MaxCwnd                  uint32
	CurSsthresh              uint32
	MaxSsthresh              uint32
	MinSsthresh              uint32
	LimCwnd                  uint32
	LimMss                   uint32
	FastRetransmissionCount  uint32
	SlowStartRetransmitCount uint32
}

// windowsCollector implements the Collector interface for Windows.
type windowsCollector struct {
	config       CollectorConfig
	ports        map[uint16]bool
	observations chan Observation

	mu      sync.Mutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// newPlatformCollector creates a Windows-specific collector.
func newPlatformCollector(cfg CollectorConfig) (Collector, error) {
	// Check if we have administrative privileges
	if err := checkWindowsPrivileges(); err != nil {
		return nil, err
	}

	ports := make(map[uint16]bool)
	for _, p := range cfg.Ports {
		ports[p] = true
	}

	return &windowsCollector{
		config:       cfg,
		ports:        ports,
		observations: make(chan Observation, 1000),
	}, nil
}

// checkWindowsPrivileges verifies we have administrative access.
func checkWindowsPrivileges() error {
	// Try to get TCP table as a test
	var size uint32
	ret, _, _ := procGetTcpTable2.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		1,
	)

	// ERROR_INSUFFICIENT_BUFFER is expected, ERROR_ACCESS_DENIED means no privileges
	if ret == uintptr(windows.ERROR_ACCESS_DENIED) {
		return ErrInsufficientPrivileges
	}

	return nil
}

// Start begins collecting RTT data.
func (c *windowsCollector) Start(ctx context.Context) error {
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
func (c *windowsCollector) Observations() <-chan Observation {
	return c.observations
}

// Close stops the collector and releases resources.
func (c *windowsCollector) Close() error {
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

// pollLoop periodically queries Windows for TCP connection info.
func (c *windowsCollector) pollLoop() {
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

// pollOnce queries Windows for TCP connections and extracts RTT.
func (c *windowsCollector) pollOnce() {
	now := time.Now()

	// Get TCP table size
	var size uint32
	ret, _, _ := procGetTcpTable2.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		1,
	)
	if ret != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		return
	}

	// Allocate buffer and get TCP table
	buf := make([]byte, size)
	ret, _, _ = procGetTcpTable2.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1,
	)
	if ret != 0 {
		return
	}

	// Parse TCP table
	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	if numEntries == 0 {
		return
	}

	entrySize := uint32(unsafe.Sizeof(mibTcpRow2{}))
	for i := uint32(0); i < numEntries; i++ {
		offset := 4 + i*entrySize
		if offset+entrySize > uint32(len(buf)) {
			break
		}

		var row mibTcpRow2
		row.dwState = binary.LittleEndian.Uint32(buf[offset:])
		row.dwLocalAddr = binary.LittleEndian.Uint32(buf[offset+4:])
		row.dwLocalPort = binary.LittleEndian.Uint32(buf[offset+8:])
		row.dwRemoteAddr = binary.LittleEndian.Uint32(buf[offset+12:])
		row.dwRemotePort = binary.LittleEndian.Uint32(buf[offset+16:])

		// Only process established connections
		if row.dwState != MIB_TCP_STATE_ESTAB {
			continue
		}

		// Convert port from network byte order
		localPort := uint16(row.dwLocalPort >> 8)

		// Filter by our ports if configured
		if len(c.ports) > 0 && !c.ports[localPort] {
			continue
		}

		// Get extended stats for this connection
		rtt := c.getConnectionRTT(&row)
		if rtt == 0 {
			continue
		}

		// Convert remote address to netip.Addr
		remoteAddr := netip.AddrFrom4([4]byte{
			byte(row.dwRemoteAddr),
			byte(row.dwRemoteAddr >> 8),
			byte(row.dwRemoteAddr >> 16),
			byte(row.dwRemoteAddr >> 24),
		})

		// Skip loopback addresses
		if remoteAddr.IsLoopback() {
			continue
		}

		obs := Observation{
			RemoteAddr: remoteAddr,
			LocalPort:  localPort,
			RTT:        rtt,
			Timestamp:  now,
		}

		// Non-blocking send
		select {
		case c.observations <- obs:
		default:
			observationsDroppedTotal.Inc()
		}
	}
}

// getConnectionRTT retrieves the smoothed RTT for a connection.
func (c *windowsCollector) getConnectionRTT(row *mibTcpRow2) time.Duration {
	var rodSize uint32 = uint32(unsafe.Sizeof(tcpEstatsPathRod{}))
	var rod tcpEstatsPathRod

	// Call GetPerTcpConnectionEStats to get path (RTT) statistics
	ret, _, _ := procGetPerTcpConnectionEStats.Call(
		uintptr(unsafe.Pointer(row)),
		uintptr(tcpConnectionEstatsPath),
		0, 0, 0, // RW parameters (not needed for reading)
		0, 0, 0, // ROS parameters (not needed)
		uintptr(unsafe.Pointer(&rod)), // ROD output buffer
		uintptr(0),
		uintptr(rodSize),
	)

	if ret != 0 {
		return 0
	}

	// SmoothedRtt is in milliseconds on Windows
	if rod.SmoothedRtt == 0 {
		return 0
	}

	return time.Duration(rod.SmoothedRtt) * time.Millisecond
}
