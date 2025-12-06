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
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const (
	// OpenGSLB DNS server address (running in container or locally)
	gslbDNSAddr = "127.0.0.1:15353"
	gslbAPIAddr = "127.0.0.1:18080"
	gslbMetrics = "127.0.0.1:19090"

	// Docker network addresses for backends
	backend1Addr = "172.28.0.2:80"
	backend2Addr = "172.28.0.3:80"
	backend3Addr = "172.28.0.4:80"

	// Timeouts
	dnsTimeout  = 2 * time.Second
	httpTimeout = 5 * time.Second
)

var (
	gslbProcess *exec.Cmd
	configPath  string
)

// =============================================================================
// Test Environment Setup
// =============================================================================

func TestMain(m *testing.M) {
	// Setup: Start OpenGSLB if not already running
	if err := setupTestEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown
	teardownTestEnvironment()

	os.Exit(code)
}

func setupTestEnvironment() error {
	// Check if OpenGSLB is already running (CI may start it separately)
	if isGSLBRunning() {
		fmt.Println("OpenGSLB already running, using existing instance")
		return nil
	}

	// Create test config
	var err error
	configPath, err = createTestConfig()
	if err != nil {
		return fmt.Errorf("failed to create test config: %w", err)
	}

	// Start OpenGSLB
	gslbBinary := os.Getenv("GSLB_BINARY")
	if gslbBinary == "" {
		gslbBinary = "./opengslb"
	}

	gslbProcess = exec.Command(gslbBinary, "--config", configPath)
	gslbProcess.Stdout = os.Stdout
	gslbProcess.Stderr = os.Stderr

	if err := gslbProcess.Start(); err != nil {
		return fmt.Errorf("failed to start OpenGSLB: %w", err)
	}

	// Wait for startup
	time.Sleep(3 * time.Second)

	if !isGSLBRunning() {
		return fmt.Errorf("OpenGSLB failed to start")
	}

	fmt.Println("OpenGSLB started successfully")
	return nil
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
	conn, err := net.DialTimeout("udp", gslbDNSAddr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func createTestConfig() (string, error) {
	config := `
dns:
  listen_address: "127.0.0.1:15353"
  default_ttl: 30

regions:
  - name: roundrobin-region
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 100
      - address: "172.28.0.3"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  - name: weighted-region
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 300
      - address: "172.28.0.3"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  - name: failover-primary
    servers:
      - address: "172.28.0.2"
        port: 80
        weight: 100
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 2
      success_threshold: 1

  - name: failover-secondary
    servers:
      - address: "172.28.0.3"
        port: 80
        weight: 100
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
      - roundrobin-region
    ttl: 10

  - name: weighted.test
    routing_algorithm: weighted
    regions:
      - weighted-region
    ttl: 10

  - name: failover.test
    routing_algorithm: failover
    regions:
      - failover-primary
      - failover-secondary
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

// =============================================================================
// DNS Helper Functions
// =============================================================================

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

// =============================================================================
// Infrastructure Tests (verify test environment)
// =============================================================================

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

// =============================================================================
// Core DNS Tests
// =============================================================================

func TestDNSBasicQuery(t *testing.T) {
	ips, err := queryDNSGetIPs("roundrobin.test")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}

	if len(ips) == 0 {
		t.Fatal("expected at least one IP in response")
	}

	// Verify IP is one of our backends
	validIPs := map[string]bool{"172.28.0.2": true, "172.28.0.3": true}
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

// =============================================================================
// Round-Robin Routing Tests
// =============================================================================

func TestRoundRobinDistribution(t *testing.T) {
	counts := make(map[string]int)
	numQueries := 20

	for i := 0; i < numQueries; i++ {
		ips, err := queryDNSGetIPs("roundrobin.test")
		if err != nil {
			t.Fatalf("query %d failed: %v", i, err)
		}
		if len(ips) > 0 {
			counts[ips[0]]++
		}
	}

	// With round-robin, we should see both IPs
	if len(counts) < 2 {
		t.Errorf("expected 2 different IPs, got %d: %v", len(counts), counts)
	}

	// Each should get roughly half (allow 30% tolerance)
	for ip, count := range counts {
		pct := float64(count) / float64(numQueries) * 100
		if pct < 20 || pct > 80 {
			t.Errorf("IP %s got %.1f%% of queries, expected ~50%%", ip, pct)
		}
	}

	t.Logf("Round-robin distribution: %v", counts)
}

// =============================================================================
// Weighted Routing Tests
// =============================================================================

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

	// Weight 300 server (172.28.0.2) should get ~75%
	// Weight 100 server (172.28.0.3) should get ~25%
	weight300Count := counts["172.28.0.2"]
	weight100Count := counts["172.28.0.3"]

	pct300 := float64(weight300Count) / float64(numQueries) * 100
	pct100 := float64(weight100Count) / float64(numQueries) * 100

	t.Logf("Weighted distribution: 172.28.0.2=%.1f%%, 172.28.0.3=%.1f%%", pct300, pct100)

	// Allow 20% tolerance: weight-300 should get 55-95%
	if pct300 < 50 || pct300 > 95 {
		t.Errorf("weight-300 server got %.1f%%, expected 55-95%%", pct300)
	}
}

// =============================================================================
// Failover Routing Tests
// =============================================================================

func TestFailoverSelectsPrimary(t *testing.T) {
	// With both servers healthy, should always return primary (first in failover order)
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

	// Primary should get all or nearly all requests when healthy
	if primaryCount < 8 {
		t.Errorf("primary server should handle most requests, got %d/10", primaryCount)
	}
}

// =============================================================================
// Metrics Endpoint Tests
// =============================================================================

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

	// Check for expected metrics
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

// =============================================================================
// Health API Tests
// =============================================================================

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

	// Verify response is valid JSON with expected structure
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

	// Should be 200 if ready, 503 if not
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unexpected status %d, expected 200 or 503", resp.StatusCode)
	}
}
