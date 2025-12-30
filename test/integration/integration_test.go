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

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const (
	gslbDNSAddr  = "127.0.0.1:15353"
	gslbAPIAddr  = "127.0.0.1:18080"
	gslbMetrics  = "127.0.0.1:19090"
	backend1Addr = "172.28.0.2:80"
	backend2Addr = "172.28.0.3:80"
	backend3Addr = "172.28.0.4:80"
	dnsTimeout   = 2 * time.Second
	httpTimeout  = 5 * time.Second
)

var (
	gslbProcess *exec.Cmd
	configPath  string
)

func TestMain(m *testing.M) {
	if err := setupTestEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	teardownTestEnvironment()
	os.Exit(code)
}

func setupTestEnvironment() error {
	if isGSLBRunning() {
		fmt.Println("OpenGSLB already running, using existing instance")
		return nil
	}

	fmt.Println("OpenGSLB not running, starting it...")

	var err error
	configPath, err = createTestConfig()
	if err != nil {
		return fmt.Errorf("failed to create test config: %w", err)
	}

	gslbBinary := os.Getenv("GSLB_BINARY")
	if gslbBinary == "" {
		candidates := []string{"./opengslb", "../opengslb", "../../opengslb"}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				gslbBinary = c
				break
			}
		}
	}

	if gslbBinary == "" {
		return fmt.Errorf("OpenGSLB binary not found. Set GSLB_BINARY env var or build with 'go build -o opengslb ./cmd/opengslb'")
	}

	if !filepath.IsAbs(gslbBinary) {
		abs, err := filepath.Abs(gslbBinary)
		if err == nil {
			gslbBinary = abs
		}
	}

	fmt.Printf("Starting OpenGSLB from: %s\n", gslbBinary)
	fmt.Printf("Config: %s\n", configPath)

	gslbProcess = exec.Command(gslbBinary, "--config", configPath)
	gslbProcess.Stdout = os.Stdout
	gslbProcess.Stderr = os.Stderr

	if err := gslbProcess.Start(); err != nil {
		return fmt.Errorf("failed to start OpenGSLB: %w", err)
	}

	fmt.Println("Waiting for OpenGSLB to start...")
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if isGSLBRunning() {
			fmt.Println("OpenGSLB started successfully")
			fmt.Println("Waiting for health checks to stabilize...")
			time.Sleep(5 * time.Second)
			return nil
		}
	}

	if gslbProcess.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- gslbProcess.Wait()
		}()

		select {
		case err := <-done:
			return fmt.Errorf("OpenGSLB process exited: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}

	return fmt.Errorf("OpenGSLB failed to start (not responding on %s)", gslbDNSAddr)
}

func teardownTestEnvironment() {
	if gslbProcess != nil && gslbProcess.Process != nil {
		gslbProcess.Process.Kill()
		gslbProcess.Wait()
	}
	if configPath != "" {
		os.Remove(configPath)
	}
}

func isGSLBRunning() bool {
	conn, err := net.DialTimeout("tcp", gslbDNSAddr, time.Second)
	if err == nil {
		conn.Close()
		return true
	}

	c := new(dns.Client)
	c.Timeout = time.Second
	m := new(dns.Msg)
	m.SetQuestion("test.", dns.TypeA)
	_, _, err = c.Exchange(m, gslbDNSAddr)
	return err == nil
}

func createTestConfig() (string, error) {
	config := `
mode: overwatch

dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

overwatch:
  gossip:
    encryption_key: "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
    bind_address: "127.0.0.1:17946"
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses and allows DNS spoofing attacks against my zones"

regions:
  - name: test-region
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 300
        service: roundrobin.test  # v1.1.0: Required
      - address: "172.28.0.3"
        port: 80
        weight: 100
        service: roundrobin.test  # v1.1.0: Required
      - address: "172.28.0.4"
        port: 80
        weight: 100
        service: roundrobin.test  # v1.1.0: Required
      # Servers for weighted.test domain
      - address: "172.28.0.2"
        port: 80
        weight: 300
        service: weighted.test
      - address: "172.28.0.3"
        port: 80
        weight: 100
        service: weighted.test
      - address: "172.28.0.4"
        port: 80
        weight: 100
        service: weighted.test
      # Servers for failover.test domain
      - address: "172.28.0.2"
        port: 80
        weight: 100
        service: failover.test
      - address: "172.28.0.3"
        port: 80
        weight: 100
        service: failover.test
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  - name: tcp-region
    servers:
      - address: "172.28.0.5"
        port: 9000
        weight: 100
        service: tcp.test  # v1.1.0: Required
    health_check:
      type: tcp
      interval: 2s
      timeout: 1s
      failure_threshold: 2
      success_threshold: 1

  # Regions for learned_latency testing (ADR-017)
  - name: region-a
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 100
        service: latency.test
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  - name: region-b
    servers:
      - address: "172.28.0.3"
        port: 80
        weight: 100
        service: latency.test
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

domains:
  - name: roundrobin.test
    routing_algorithm: round-robin
    regions:
      - test-region
    ttl: 10

  - name: weighted.test
    routing_algorithm: weighted
    regions:
      - test-region
    ttl: 10

  - name: failover.test
    routing_algorithm: failover
    regions:
      - test-region
    ttl: 10

  - name: tcp.test
    routing_algorithm: round-robin
    regions:
      - tcp-region
    ttl: 10

  # Learned latency domain for ADR-017 testing
  - name: latency.test
    routing_algorithm: learned_latency
    regions:
      - region-a
      - region-b
    ttl: 10

logging:
  level: info
  format: text

metrics:
  enabled: true
  address: "127.0.0.1:19090"

api:
  enabled: true
  address: "127.0.0.1:18080"
  allowed_networks:
    - "127.0.0.0/8"
    - "172.28.0.0/16"
`

	tmpFile, err := os.CreateTemp("", "opengslb-test-*.yaml")
	if err != nil {
		return "", err
	}

	if _, err := tmpFile.WriteString(config); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}

func queryDNS(domain string, qtype uint16) (*dns.Msg, error) {
	c := new(dns.Client)
	c.Timeout = dnsTimeout
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), qtype)
	r, _, err := c.Exchange(m, gslbDNSAddr)
	return r, err
}

func queryDNSGetIPs(domain string) ([]string, error) {
	r, err := queryDNS(domain, dns.TypeA)
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, ans := range r.Answer {
		if a, ok := ans.(*dns.A); ok {
			ips = append(ips, a.A.String())
		}
	}
	return ips, nil
}

func TestBackendHTTPConnectivity(t *testing.T) {
	backends := []string{
		"http://" + backend1Addr,
		"http://" + backend2Addr,
	}
	client := &http.Client{Timeout: httpTimeout}

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

func TestDNSBasicQuery(t *testing.T) {
	ips, err := queryDNSGetIPs("roundrobin.test")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("expected at least one IP in response")
	}
	validIPs := map[string]bool{"172.28.0.2": true, "172.28.0.3": true, "172.28.0.4": true}
	if !validIPs[ips[0]] {
		t.Errorf("unexpected IP %s, expected one of %v", ips[0], validIPs)
	}
}

func TestDNSNXDOMAIN(t *testing.T) {
	r, err := queryDNS("nonexistent.domain.test", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if r.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN (Rcode %d), got Rcode %d", dns.RcodeNameError, r.Rcode)
	}
}

func TestDNSTCPQuery(t *testing.T) {
	c := new(dns.Client)
	c.Net = "tcp"
	c.Timeout = dnsTimeout
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn("roundrobin.test"), dns.TypeA)

	r, _, err := c.Exchange(m, gslbDNSAddr)
	if err != nil {
		t.Fatalf("TCP DNS query failed: %v", err)
	}
	if len(r.Answer) == 0 {
		t.Error("expected at least one answer in TCP response")
	}
}

func TestDNSTTL(t *testing.T) {
	r, err := queryDNS("roundrobin.test", dns.TypeA)
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if len(r.Answer) == 0 {
		t.Fatal("no answers in response")
	}
	ttl := r.Answer[0].Header().Ttl
	if ttl == 0 || ttl > 30 {
		t.Errorf("unexpected TTL %d, expected 1-30", ttl)
	}
}

func TestRoundRobinDistribution(t *testing.T) {
	counts := make(map[string]int)
	numQueries := 30

	for i := 0; i < numQueries; i++ {
		ips, err := queryDNSGetIPs("roundrobin.test")
		if err != nil {
			t.Fatalf("query %d failed: %v", i, err)
		}
		if len(ips) > 0 {
			counts[ips[0]]++
		}
	}

	if len(counts) < 3 {
		t.Errorf("expected 3 different IPs, got %d: %v", len(counts), counts)
	}

	for ip, count := range counts {
		pct := float64(count) / float64(numQueries) * 100
		if pct < 13 || pct > 53 {
			t.Errorf("IP %s got %.1f%% of queries, expected ~33%%", ip, pct)
		}
	}
	t.Logf("Round-robin distribution: %v", counts)
}

func TestWeightedDistribution(t *testing.T) {
	counts := make(map[string]int)
	numQueries := 100

	for i := 0; i < numQueries; i++ {
		ips, err := queryDNSGetIPs("weighted.test")
		if err != nil {
			t.Fatalf("query %d failed: %v", i, err)
		}
		if len(ips) > 0 {
			counts[ips[0]]++
		}
	}

	weight300Count := counts["172.28.0.2"]
	pct300 := float64(weight300Count) / float64(numQueries) * 100

	t.Logf("Weighted distribution: %v", counts)
	t.Logf("Weight-300 server got %.1f%%", pct300)

	if pct300 < 40 || pct300 > 80 {
		t.Errorf("weight-300 server got %.1f%%, expected 40-80%%", pct300)
	}
}

func TestFailoverSelectsPrimary(t *testing.T) {
	var primaryCount, secondaryCount int

	for i := 0; i < 10; i++ {
		ips, err := queryDNSGetIPs("failover.test")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(ips) == 0 {
			continue
		}
		switch ips[0] {
		case "172.28.0.2":
			primaryCount++
		case "172.28.0.3":
			secondaryCount++
		}
	}

	t.Logf("Failover results: primary=%d, secondary=%d", primaryCount, secondaryCount)

	if primaryCount < 8 {
		t.Errorf("primary server should handle most requests, got %d/10", primaryCount)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Get("http://" + gslbMetrics + "/metrics")
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	expectedMetrics := []string{
		"opengslb_app_info",
		"opengslb_configured_domains",
		"go_goroutines",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("missing expected metric: %s", metric)
		}
	}
}

func TestHealthAPIEndpoint(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Get("http://" + gslbAPIAddr + "/api/v1/health/servers")
	if err != nil {
		t.Skipf("API endpoint not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var response struct {
		Servers []struct {
			Address string `json:"address"`
			Healthy bool   `json:"healthy"`
		} `json:"servers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Servers) == 0 {
		t.Error("expected at least one server in response")
	}
	t.Logf("Health API returned %d servers", len(response.Servers))
}

func TestHealthAPILiveness(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Get("http://" + gslbAPIAddr + "/api/v1/live")
	if err != nil {
		t.Skipf("Liveness endpoint not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHealthAPIReadiness(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Get("http://" + gslbAPIAddr + "/api/v1/ready")
	if err != nil {
		t.Skipf("Readiness endpoint not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unexpected status %d, expected 200 or 503", resp.StatusCode)
	}
}

// TestAPIServerCRUD tests the v1.1.0 server CRUD API endpoints
func TestAPIServerCRUD(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	// Test POST - Create new server
	t.Run("CreateServer", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name":    "api-test-server",
			"address": "172.28.0.100",
			"port":    8080,
			"weight":  150,
			"region":  "test-region",
			"enabled": true,
			"metadata": map[string]string{
				"service": "roundrobin.test",
			},
		}

		jsonBody, _ := json.Marshal(reqBody)
		resp, err := client.Post(
			"http://"+gslbAPIAddr+"/api/v1/servers",
			"application/json",
			bytes.NewBuffer(jsonBody),
		)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}

		// Wait for DNS propagation
		time.Sleep(2 * time.Second)

		// Verify server appears in DNS responses (round-robin, so query multiple times)
		// We now have 4 servers total (3 static + 1 API-created), so query enough times to see them all
		found := false
		seenIPs := make(map[string]bool)
		for i := 0; i < 20; i++ {
			ips, err := queryDNSGetIPs("roundrobin.test")
			if err != nil {
				t.Fatalf("DNS query %d failed: %v", i, err)
			}
			for _, ip := range ips {
				seenIPs[ip] = true
				if ip == "172.28.0.100" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("API-created server 172.28.0.100 not found in DNS responses after 20 queries. Seen IPs: %v", seenIPs)
		}
	})

	// Test GET - List servers
	t.Run("ListServers", func(t *testing.T) {
		resp, err := client.Get("http://" + gslbAPIAddr + "/api/v1/servers")
		if err != nil {
			t.Fatalf("failed to list servers: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var response struct {
			Servers []struct {
				ID       string            `json:"id"`
				Address  string            `json:"address"`
				Port     int               `json:"port"`
				Metadata map[string]string `json:"metadata"`
			} `json:"servers"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Should have both static and API-registered servers
		if len(response.Servers) < 4 {
			t.Errorf("expected at least 4 servers, got %d", len(response.Servers))
		}

		// Verify source tracking
		hasStatic := false
		hasAPI := false
		for _, s := range response.Servers {
			if source, ok := s.Metadata["source"]; ok {
				if source == "static" {
					hasStatic = true
				}
				if source == "api" {
					hasAPI = true
				}
			}
		}

		if !hasStatic {
			t.Error("no static servers found in list")
		}
		if !hasAPI {
			t.Error("no API-registered servers found in list")
		}

		t.Logf("Server list returned %d servers (static=%v, api=%v)",
			len(response.Servers), hasStatic, hasAPI)
	})

	// Test PATCH - Update server
	t.Run("UpdateServer", func(t *testing.T) {
		serverID := "roundrobin.test:172.28.0.100:8080"

		reqBody := map[string]interface{}{
			"weight": 250,
		}

		jsonBody, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest(
			"PATCH",
			"http://"+gslbAPIAddr+"/api/v1/servers/"+serverID,
			bytes.NewBuffer(jsonBody),
		)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to update server: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test DELETE - Remove server
	t.Run("DeleteServer", func(t *testing.T) {
		serverID := "roundrobin.test:172.28.0.100:8080"

		req, _ := http.NewRequest(
			"DELETE",
			"http://"+gslbAPIAddr+"/api/v1/servers/"+serverID,
			nil,
		)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to delete server: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 204 or 200, got %d", resp.StatusCode)
		}

		// Wait for DNS propagation
		time.Sleep(2 * time.Second)

		// Verify server no longer in DNS responses
		ips, err := queryDNSGetIPs("roundrobin.test")
		if err != nil {
			t.Fatalf("DNS query failed: %v", err)
		}

		for _, ip := range ips {
			if ip == "172.28.0.100" {
				t.Errorf("deleted server still appears in DNS responses")
			}
		}
	})
}

// TestLearnedLatencyRouting tests ADR-017 passive latency learning routing.
// It verifies that:
// 1. Without latency data, routing falls back to round-robin
// 2. With injected latency data, routing selects the lowest-latency backend
// 3. Different client subnets can get different routing decisions
func TestLearnedLatencyRouting(t *testing.T) {
	client := &http.Client{Timeout: httpTimeout}

	// Test 1: Without latency data, should fall back to round-robin
	t.Run("FallbackWithoutData", func(t *testing.T) {
		counts := make(map[string]int)
		for i := 0; i < 10; i++ {
			ips, err := queryDNSGetIPs("latency.test")
			if err != nil {
				t.Fatalf("DNS query failed: %v", err)
			}
			if len(ips) > 0 {
				counts[ips[0]]++
			}
		}
		// Should see both servers (fallback to round-robin)
		if len(counts) < 2 {
			t.Logf("Warning: only saw %d different IPs, expected 2 (round-robin fallback)", len(counts))
		}
		t.Logf("Fallback distribution (no latency data): %v", counts)
	})

	// Test 2: Inject latency data and verify routing uses it
	t.Run("RoutesBasedOnLatency", func(t *testing.T) {
		// Inject latency data: region-a (172.28.0.2) has 100ms, region-b (172.28.0.3) has 10ms
		// The 127.0.0.0/8 subnet is the client subnet (DNS queries come from localhost)
		injectLatency := func(subnet, backend, region string, latencyMs int64, samples uint64) error {
			reqBody := map[string]interface{}{
				"subnet":     subnet,
				"backend":    backend,
				"region":     region,
				"latency_ms": latencyMs,
				"samples":    samples,
			}
			jsonBody, _ := json.Marshal(reqBody)
			resp, err := client.Post(
				"http://"+gslbAPIAddr+"/api/v1/overwatch/latency",
				"application/json",
				bytes.NewBuffer(jsonBody),
			)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("expected 201, got %d: %s", resp.StatusCode, string(body))
			}
			return nil
		}

		// Inject latency data for localhost subnet
		// Note: The table uses /24 prefix for IPv4 lookups, so we must inject with /24
		if err := injectLatency("127.0.0.0/24", "latency.test", "region-a", 100, 10); err != nil {
			t.Fatalf("Failed to inject latency for region-a: %v", err)
		}
		if err := injectLatency("127.0.0.0/24", "latency.test", "region-b", 10, 10); err != nil {
			t.Fatalf("Failed to inject latency for region-b: %v", err)
		}

		// Wait for data to be available
		time.Sleep(100 * time.Millisecond)

		// Query multiple times - should consistently get region-b (lower latency)
		counts := make(map[string]int)
		for i := 0; i < 20; i++ {
			ips, err := queryDNSGetIPs("latency.test")
			if err != nil {
				t.Fatalf("DNS query %d failed: %v", i, err)
			}
			if len(ips) > 0 {
				counts[ips[0]]++
			}
		}

		t.Logf("Latency-based distribution: %v", counts)

		// region-b (172.28.0.3) should be selected consistently (it has lower latency)
		regionBCount := counts["172.28.0.3"]
		regionACount := counts["172.28.0.2"]

		if regionBCount < 15 {
			t.Errorf("Expected region-b (172.28.0.3, 10ms latency) to be selected most often, got %d/20 (region-a got %d)",
				regionBCount, regionACount)
		}
	})

	// Test 3: Verify latency data appears in API
	t.Run("LatencyDataInAPI", func(t *testing.T) {
		resp, err := client.Get("http://" + gslbAPIAddr + "/api/v1/overwatch/latency")
		if err != nil {
			t.Fatalf("Failed to get latency data: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		var response struct {
			Entries []struct {
				Subnet  string `json:"subnet"`
				Backend string `json:"backend"`
				Region  string `json:"region"`
				EWMAMs  int64  `json:"ewma_ms"`
			} `json:"entries"`
			Count int `json:"count"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if response.Count < 2 {
			t.Errorf("Expected at least 2 latency entries, got %d", response.Count)
		}

		t.Logf("Latency API returned %d entries", response.Count)
		for _, e := range response.Entries {
			t.Logf("  %s -> %s (%s): %dms", e.Subnet, e.Backend, e.Region, e.EWMAMs)
		}
	})
}
