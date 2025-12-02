package health

import (
	"context"
	"fmt"
	"net"
	"time"
)

// TCPChecker performs TCP health checks by attempting to establish a connection.
type TCPChecker struct {
	// dialer is the net.Dialer used for connections.
	dialer *net.Dialer
}

// TCPCheckerOption configures a TCPChecker.
type TCPCheckerOption func(*TCPChecker)

// WithDialer sets a custom net.Dialer for the TCP checker.
func WithDialer(d *net.Dialer) TCPCheckerOption {
	return func(c *TCPChecker) {
		c.dialer = d
	}
}

// NewTCPChecker creates a new TCP health checker.
func NewTCPChecker(opts ...TCPCheckerOption) *TCPChecker {
	c := &TCPChecker{
		dialer: &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: -1, // Disable keep-alive for health checks
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Type returns "tcp".
func (c *TCPChecker) Type() string {
	return "tcp"
}

// Check performs a TCP health check by attempting to connect to the target.
// A successful connection indicates the target is healthy.
func (c *TCPChecker) Check(ctx context.Context, target Target) Result {
	start := time.Now()

	result := Result{
		Timestamp: start,
	}

	// Format address correctly for IPv6 (needs brackets)
	var address string
	if isIPv6(target.Address) {
		address = fmt.Sprintf("[%s]:%d", target.Address, target.Port)
	} else {
		address = fmt.Sprintf("%s:%d", target.Address, target.Port)
	}

	// Use context-aware dial
	conn, err := c.dialer.DialContext(ctx, "tcp", address)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("tcp connect failed: %w", err)
		return result
	}

	// Connection successful - close it immediately
	conn.Close()
	result.Healthy = true

	return result
}

// isIPv6 checks if the address is an IPv6 address.
func isIPv6(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil && ip.To4() == nil
}
