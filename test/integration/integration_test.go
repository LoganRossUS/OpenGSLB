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

gossip:
  encryption_key: "dGhpcy1pcy1hLTMyLWJ5dGUtdGVzdC1rZXkh"
  bind_address: "127.0.0.1:17946"

regions:
  - name: test-region
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 300
      - address: "172.28.0.3"
        port: 80
        weight: 100
      - address: "172.28.0.4"
        port: 80
        weight: 100
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
    health_check:
      type: tcp
      interval: 2s
      timeout: 1s
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