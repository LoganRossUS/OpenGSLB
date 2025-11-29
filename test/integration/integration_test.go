//go:build integration

package integration

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestBackendHTTPConnectivity verifies mock backend servers are reachable
func TestBackendHTTPConnectivity(t *testing.T) {
	backends := []string{
		"http://172.28.0.2:80",  // backend1
		"http://172.28.0.3:80",  // backend2
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			resp, err := client.Get(backend)
			if err != nil {
				t.Fatalf("failed to connect to backend %s: %v", backend, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

// TestDNSMockResolution verifies the mock DNS resolver works
func TestDNSMockResolution(t *testing.T) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", "172.28.0.100:53")
		},
	}

	tests := []struct {
		host     string
		expected string
	}{
		{"backend1.test.local", "172.28.0.10"},
		{"backend2.test.local", "172.28.0.11"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ips, err := resolver.LookupIP(ctx, "ip4", tt.host)
			if err != nil {
				t.Fatalf("DNS lookup failed for %s: %v", tt.host, err)
			}

			if len(ips) == 0 {
				t.Fatalf("no IPs returned for %s", tt.host)
			}

			if ips[0].String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, ips[0].String())
			}
		})
	}
}
