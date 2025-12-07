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

package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRecordDNSQuery(t *testing.T) {
	// Record some queries
	RecordDNSQuery("example.com", "A", "success")
	RecordDNSQuery("example.com", "A", "success")
	RecordDNSQuery("example.com", "A", "nxdomain")
	RecordDNSQuery("other.com", "AAAA", "servfail")

	// Verify counter exists and doesn't panic
	// Full verification would require reading from the registry
}

func TestRecordDNSQueryDuration(t *testing.T) {
	RecordDNSQueryDuration("example.com", "success", 0.001)
	RecordDNSQueryDuration("example.com", "success", 0.005)
	RecordDNSQueryDuration("example.com", "nxdomain", 0.002)
}

func TestRecordHealthCheckResult(t *testing.T) {
	RecordHealthCheckResult("us-east-1", "10.0.1.10:80", "healthy")
	RecordHealthCheckResult("us-east-1", "10.0.1.10:80", "unhealthy")
	RecordHealthCheckResult("us-west-2", "10.0.2.10:80", "healthy")
}

func TestRecordHealthCheckDuration(t *testing.T) {
	RecordHealthCheckDuration("us-east-1", "10.0.1.10:80", 0.05)
	RecordHealthCheckDuration("us-east-1", "10.0.1.10:80", 0.1)
}

func TestSetHealthyServers(t *testing.T) {
	SetHealthyServers("us-east-1", 3)
	SetHealthyServers("us-east-1", 2)
	SetHealthyServers("us-west-2", 5)
}

func TestRecordRoutingDecision(t *testing.T) {
	RecordRoutingDecision("example.com", "round-robin", "10.0.1.10:80")
	RecordRoutingDecision("example.com", "round-robin", "10.0.1.11:80")
	RecordRoutingDecision("other.com", "weighted", "10.0.2.10:80")
}

func TestSetAppInfo(t *testing.T) {
	SetAppInfo("0.1.0-dev")
}

func TestSetConfigMetrics(t *testing.T) {
	SetConfigMetrics(5, 10, float64(time.Now().Unix()))
}

func TestMetricsServer(t *testing.T) {
	// Use a random available port
	cfg := ServerConfig{
		Address: "127.0.0.1:0",
	}

	server := NewServer(cfg)

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)

	go func() {
		errChan <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Note: Since we use :0, we can't easily get the actual port
	// This test primarily verifies the server starts without error

	// Cancel to trigger shutdown
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not shut down in time")
	}
}

func TestMetricsServerEndpoints(t *testing.T) {
	// Start server on specific port for testing
	cfg := ServerConfig{
		Address: "127.0.0.1:19090",
	}

	server := NewServer(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	t.Run("metrics endpoint", func(t *testing.T) {
		resp, err := http.Get("http://127.0.0.1:19090/metrics")
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Check for some expected metrics
		if !strings.Contains(bodyStr, "opengslb_") {
			t.Error("expected opengslb_ metrics in response")
		}

		// Check for standard Go metrics
		if !strings.Contains(bodyStr, "go_goroutines") {
			t.Error("expected go_goroutines metric in response")
		}
	})

	t.Run("health endpoint", func(t *testing.T) {
		resp, err := http.Get("http://127.0.0.1:19090/health")
		if err != nil {
			t.Fatalf("failed to get health: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		if string(body) != "OK" {
			t.Errorf("expected 'OK', got %q", string(body))
		}
	})

	// Cleanup
	cancel()
	<-errChan
}
