// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	miekg "github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/routing"
)

// The reload tests start a real overwatch in-process (the wiring the binary
// uses), drive it through the REST API and the DNS port, and reload it either
// with a real SIGHUP or by calling handleReload, the function the SIGHUP
// handler calls.

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// reloadServer is one static server in the generated configuration file.
type reloadServer struct {
	address string
	port    int
	service string
	weight  int
}

// reloadDomain is one domain in the generated configuration file.
type reloadDomain struct {
	name      string
	algorithm string
}

// reloadEnv is a running overwatch plus the file it was configured from.
type reloadEnv struct {
	t       *testing.T
	app     *Application
	logger  *slog.Logger
	logs    *syncBuffer
	base    string // REST API base URL
	dnsAddr string
	cfgPath string
	dataDir string

	gossipPort, dnsPort, apiPort int

	stop func()
}

// liveBackend starts an HTTP server on ip:0 that answers 200 to everything and
// returns its port. Tests that need several distinguishable A records bind
// other 127.0.0.0/8 addresses, which only works where the whole block is
// routed to loopback (Linux). Elsewhere the test is skipped.
func liveBackend(t *testing.T, ip string) int {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		if ip != "127.0.0.1" {
			t.Skipf("cannot bind %s on this platform: %v", ip, err)
		}
		t.Fatalf("listen on %s: %v", ip, err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return l.Addr().(*net.TCPAddr).Port
}

// renderConfig renders an overwatch configuration file. Every static server
// lives in region us-east and every domain routes to that region.
func (e *reloadEnv) renderConfig(domains []reloadDomain, servers []reloadServer) string {
	var b strings.Builder
	fmt.Fprintf(&b, `mode: overwatch
overwatch:
  identity:
    node_id: reload-overwatch
    region: us-east
  data_dir: %q
  gossip:
    bind_address: "127.0.0.1:%d"
    encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  validation:
    enabled: true
    check_interval: 1s
    check_timeout: 1s
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses"
dns:
  listen_address: "127.0.0.1:%d"
  default_ttl: 30
  zones:
    - gslb.example.com
api:
  enabled: true
  address: "127.0.0.1:%d"
logging:
  level: info
  format: text
metrics:
  enabled: false
regions:
  - name: us-east
    countries: ["US"]
    continents: ["NA"]
    health_check:
      type: http
      interval: 1s
      timeout: 1s
      path: /
      failure_threshold: 3
      success_threshold: 1
    servers:
`, e.dataDir, e.gossipPort, e.dnsPort, e.apiPort)
	for _, s := range servers {
		weight := s.weight
		if weight == 0 {
			weight = 100
		}
		fmt.Fprintf(&b, "      - address: %s\n        port: %d\n        weight: %d\n        service: %s\n",
			s.address, s.port, weight, s.service)
	}
	b.WriteString("domains:\n")
	for _, d := range domains {
		algorithm := d.algorithm
		if algorithm == "" {
			algorithm = "round-robin"
		}
		fmt.Fprintf(&b, "  - name: %s\n    routing_algorithm: %s\n    regions: [us-east]\n    ttl: 30\n",
			d.name, algorithm)
	}
	return b.String()
}

// writeConfig replaces the configuration file on disk. It does not reload.
func (e *reloadEnv) writeConfig(domains []reloadDomain, servers []reloadServer) {
	e.t.Helper()
	if err := os.WriteFile(e.cfgPath, []byte(e.renderConfig(domains, servers)), 0o600); err != nil {
		e.t.Fatalf("write config: %v", err)
	}
}

// newReloadEnv allocates ports and directories. dataDir may be shared between
// two environments to simulate a restart on the same store.
func newReloadEnv(t *testing.T, dataDir string) *reloadEnv {
	t.Helper()
	if dataDir == "" {
		dataDir = t.TempDir()
	}
	logs := &syncBuffer{}
	return &reloadEnv{
		t:          t,
		logs:       logs,
		logger:     slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		cfgPath:    filepath.Join(t.TempDir(), "config.yaml"),
		dataDir:    dataDir,
		gossipPort: contractFreePort(t),
		dnsPort:    contractFreePort(t),
		apiPort:    contractFreePort(t),
	}
}

// start loads the configuration file and runs the application until stop (or
// test cleanup). It mirrors what main does between config.Load and the signal
// loop.
func (e *reloadEnv) start() {
	e.t.Helper()
	cfg, err := config.Load(e.cfgPath)
	if err != nil {
		e.t.Fatalf("load config: %v", err)
	}
	applyModeOverride(cfg, e.logger)

	// handleReload reads the package-level configPath, as the binary does.
	previousPath := configPath
	configPath = e.cfgPath

	e.app = NewApplication(cfg, e.logger)
	if err := e.app.Initialize(); err != nil {
		configPath = previousPath
		e.t.Fatalf("Initialize: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = e.app.Start(ctx)
	}()
	var once sync.Once
	e.stop = func() {
		once.Do(func() {
			sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer scancel()
			_ = e.app.Shutdown(sctx)
			cancel()
			<-done
			configPath = previousPath
		})
	}
	e.t.Cleanup(e.stop)

	e.base = fmt.Sprintf("http://127.0.0.1:%d", e.apiPort)
	e.dnsAddr = fmt.Sprintf("127.0.0.1:%d", e.dnsPort)
	waitForAPI(e.t, e.base)
	e.waitForDNS()
}

// startReloadOverwatch writes a configuration and starts an overwatch on it.
func startReloadOverwatch(t *testing.T, domains []reloadDomain, servers []reloadServer) *reloadEnv {
	t.Helper()
	e := newReloadEnv(t, "")
	e.writeConfig(domains, servers)
	e.start()
	return e
}

// query sends one A query over UDP and returns the rcode and the A records.
func (e *reloadEnv) query(name string) (int, []string, error) {
	m := new(miekg.Msg)
	m.SetQuestion(miekg.Fqdn(name), miekg.TypeA)
	c := &miekg.Client{Net: "udp", Timeout: 2 * time.Second}
	resp, _, err := c.Exchange(m, e.dnsAddr)
	if err != nil {
		return 0, nil, err
	}
	var ips []string
	for _, rr := range resp.Answer {
		if a, ok := rr.(*miekg.A); ok {
			ips = append(ips, a.A.String())
		}
	}
	return resp.Rcode, ips, nil
}

func (e *reloadEnv) waitForDNS() {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, err := e.query("probe.gslb.example.com"); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	e.t.Fatalf("DNS server at %s did not come up", e.dnsAddr)
}

// answers queries name repeatedly and returns the sorted set of addresses
// seen, so that every server in rotation shows up. It fails on any rcode
// other than NOERROR.
func (e *reloadEnv) answers(name string) []string {
	e.t.Helper()
	seen := map[string]bool{}
	for i := 0; i < 24; i++ {
		rcode, ips, err := e.query(name)
		if err != nil {
			e.t.Fatalf("query %s: %v", name, err)
		}
		if rcode != miekg.RcodeSuccess {
			e.t.Fatalf("query %s: rcode %s, want NOERROR", name, miekg.RcodeToString[rcode])
		}
		for _, ip := range ips {
			seen[ip] = true
		}
	}
	out := make([]string, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func (e *reloadEnv) requireResolves(name string, want ...string) {
	e.t.Helper()
	got := e.answers(name)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		e.t.Fatalf("%s resolves to %v, want %v", name, got, want)
	}
}

func (e *reloadEnv) requireRcode(name string, want int) {
	e.t.Helper()
	rcode, ips, err := e.query(name)
	if err != nil {
		e.t.Fatalf("query %s: %v", name, err)
	}
	if rcode != want {
		e.t.Fatalf("query %s: rcode %s (answers %v), want %s",
			name, miekg.RcodeToString[rcode], ips, miekg.RcodeToString[want])
	}
}

// call performs one REST call and returns the status code and body.
func (e *reloadEnv) call(method, path, body string) (int, string) {
	e.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.base+path, rd)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *reloadEnv) mustCall(method, path, body string, want int) string {
	e.t.Helper()
	status, got := e.call(method, path, body)
	if status != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, got)
	}
	return got
}

// createDomain creates a domain through the REST API.
func (e *reloadEnv) createDomain(name string) {
	e.t.Helper()
	e.mustCall("POST", "/api/v1/domains",
		fmt.Sprintf(`{"name":%q,"ttl":20,"routing_policy":"round-robin","enabled":true}`, name), http.StatusCreated)
}

// createServer creates a server for a domain through the REST API.
func (e *reloadEnv) createServer(service, address string, port int) {
	e.t.Helper()
	e.mustCall("POST", "/api/v1/servers",
		fmt.Sprintf(`{"address":%q,"port":%d,"weight":100,"region":"us-east","enabled":true,"metadata":{"service":%q}}`,
			address, port, service), http.StatusCreated)
}

// reload calls handleReload, the function the SIGHUP handler in main calls.
func (e *reloadEnv) reload() {
	e.t.Helper()
	if err := handleReload(e.app, e.logger); err != nil {
		e.t.Fatalf("handleReload: %v", err)
	}
}

// dnsServers returns the "address:port" of every server the DNS registry
// holds for a domain, sorted, or nil when the domain is not registered.
func (e *reloadEnv) dnsServers(domain string) []string {
	entry := e.app.dnsRegistry.Lookup(domain)
	if entry == nil {
		return nil
	}
	out := make([]string, 0, len(entry.Servers))
	for _, s := range entry.Servers {
		out = append(out, fmt.Sprintf("%s:%d", s.Address, s.Port))
	}
	sort.Strings(out)
	return out
}

// dnsState returns the whole DNS registry as domain -> sorted servers.
func (e *reloadEnv) dnsState() map[string][]string {
	state := map[string][]string{}
	for _, name := range e.app.dnsRegistry.Domains() {
		state[name] = e.dnsServers(name)
	}
	return state
}

const (
	reloadStaticDomain = "app.gslb.example.com"
	reloadAPIDomain    = "api-created.gslb.example.com"
)

// TestReload_APICreatedDomainSurvives is the reproduction of the reported
// defect: a domain and server created through the REST API resolve, and must
// still resolve after a reload.
func TestReload_APICreatedDomainSurvives(t *testing.T) {
	staticPort := liveBackend(t, "127.0.0.1")
	apiPort := liveBackend(t, "127.0.0.2")

	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: reloadStaticDomain}})

	e.createDomain(reloadAPIDomain)
	e.createServer(reloadAPIDomain, "127.0.0.2", apiPort)
	e.requireResolves(reloadAPIDomain, "127.0.0.2")
	e.requireResolves(reloadStaticDomain, "127.0.0.1")

	e.reload()

	e.requireResolves(reloadAPIDomain, "127.0.0.2")
	e.requireResolves(reloadStaticDomain, "127.0.0.1")
	e.mustCall("GET", "/api/v1/domains/"+reloadAPIDomain, "", http.StatusOK)
}

// TestReload_SIGHUP sends a real SIGHUP to the test process and lets the
// signal loop that main runs handle it.
func TestReload_SIGHUP(t *testing.T) {
	staticPort := liveBackend(t, "127.0.0.1")
	apiPort := liveBackend(t, "127.0.0.2")

	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: reloadStaticDomain}})

	e.createDomain(reloadAPIDomain)
	e.createServer(reloadAPIDomain, "127.0.0.2", apiPort)
	e.requireResolves(reloadAPIDomain, "127.0.0.2")

	// Install the handler before raising the signal: an unhandled SIGHUP
	// would terminate the test binary.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	loopDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		defer close(loopDone)
		runSignalLoop(e.app, e.logger, sigChan, errChan)
	}()

	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("kill -HUP self: %v", err)
	}

	// The reload is asynchronous to the signal. Its completion is visible as
	// a log line (and as opengslb_config_reloads_total{result="success"}).
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(e.logs.String(), "configuration reloaded successfully") {
		if strings.Contains(e.logs.String(), "configuration reload failed") {
			t.Fatalf("reload failed:\n%s", e.logs.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("no reload after SIGHUP:\n%s", e.logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	e.requireResolves(reloadAPIDomain, "127.0.0.2")
	e.requireResolves(reloadStaticDomain, "127.0.0.1")
	e.mustCall("GET", "/api/v1/domains/"+reloadAPIDomain, "", http.StatusOK)

	// Leave the loop the way main does on a fatal application error.
	errChan <- nil
	<-loopDone
}

// TestReload_MergesConfigFileAndAPIState covers the state a reload has to
// merge: the configuration file is the source of truth for what it defines,
// the store for what the API created.
func TestReload_MergesConfigFileAndAPIState(t *testing.T) {
	const (
		keptDomain    = "kept.gslb.example.com"
		removedDomain = "removed.gslb.example.com"
		deletedDomain = "api-deleted.gslb.example.com"
	)
	keptPort := liveBackend(t, "127.0.0.1")
	removedPort := liveBackend(t, "127.0.0.3")
	apiPort := liveBackend(t, "127.0.0.2")
	addedPort := liveBackend(t, "127.0.0.4")
	deletedPort := liveBackend(t, "127.0.0.5")

	e := startReloadOverwatch(t,
		[]reloadDomain{{name: keptDomain}, {name: removedDomain}},
		[]reloadServer{
			{address: "127.0.0.1", port: keptPort, service: keptDomain},
			{address: "127.0.0.3", port: removedPort, service: removedDomain},
		})

	// API-created state: a domain with a server, a domain backend, a region,
	// and a second domain that is deleted again before the reload.
	e.createDomain(reloadAPIDomain)
	e.createServer(reloadAPIDomain, "127.0.0.2", apiPort)
	e.mustCall("POST", "/api/v1/domains/"+reloadAPIDomain+"/backends",
		`{"address":"127.0.0.9","port":8080,"weight":10,"region":"us-east","enabled":true}`, http.StatusCreated)
	e.mustCall("POST", "/api/v1/regions",
		`{"name":"ap-south","code":"ap-south","continent":"AS","countries":["IN"],"enabled":true}`, http.StatusCreated)
	e.createDomain(deletedDomain)
	e.createServer(deletedDomain, "127.0.0.5", deletedPort)
	e.requireResolves(deletedDomain, "127.0.0.5")
	e.mustCall("DELETE", "/api/v1/servers/"+fmt.Sprintf("%s:127.0.0.5:%d", deletedDomain, deletedPort), "", http.StatusNoContent)
	e.mustCall("DELETE", "/api/v1/domains/"+deletedDomain, "", http.StatusNoContent)

	e.requireResolves(keptDomain, "127.0.0.1")
	e.requireResolves(removedDomain, "127.0.0.3")
	e.requireResolves(reloadAPIDomain, "127.0.0.2")
	e.requireRcode(deletedDomain, miekg.RcodeNameError)

	// New file: removedDomain and its server are gone, keptDomain gains a
	// second server.
	e.writeConfig(
		[]reloadDomain{{name: keptDomain}},
		[]reloadServer{
			{address: "127.0.0.1", port: keptPort, service: keptDomain},
			{address: "127.0.0.4", port: addedPort, service: keptDomain},
		})
	e.reload()

	t.Run("config domain removed from the file disappears", func(t *testing.T) {
		e.t = t
		e.requireRcode(removedDomain, miekg.RcodeNameError)
		if servers := e.dnsServers(removedDomain); servers != nil {
			t.Errorf("DNS registry still holds %s: %v", removedDomain, servers)
		}
		// Its static server must leave the backend registry too, as it would
		// after a restart, or it would keep being validated and listed.
		if _, ok := e.app.backendRegistry.GetBackend(removedDomain, "127.0.0.3", removedPort); ok {
			t.Errorf("backend registry still holds the static server of %s", removedDomain)
		}
	})

	t.Run("config server added to the file is served", func(t *testing.T) {
		e.t = t
		e.requireResolves(keptDomain, "127.0.0.1", "127.0.0.4")
		// A server added by a reload is registered for validation exactly
		// like one present at startup.
		b, ok := e.app.backendRegistry.GetBackend(keptDomain, "127.0.0.4", addedPort)
		if !ok {
			t.Fatalf("backend registry does not hold the server added by the reload")
		}
		if string(b.Source) != "static" {
			t.Errorf("added server has source %q, want static", b.Source)
		}
	})

	t.Run("API-created domain and server stay", func(t *testing.T) {
		e.t = t
		e.requireResolves(reloadAPIDomain, "127.0.0.2")
		e.mustCall("GET", "/api/v1/domains/"+reloadAPIDomain, "", http.StatusOK)
		e.mustCall("GET", "/api/v1/servers/"+fmt.Sprintf("%s:127.0.0.2:%d", reloadAPIDomain, apiPort), "", http.StatusOK)
	})

	t.Run("API-created domain backend stays", func(t *testing.T) {
		e.t = t
		got := e.dnsServers(reloadAPIDomain)
		want := []string{fmt.Sprintf("127.0.0.2:%d", apiPort), "127.0.0.9:8080"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("DNS registry servers for %s = %v, want %v", reloadAPIDomain, got, want)
		}
		body := e.mustCall("GET", "/api/v1/domains/"+reloadAPIDomain+"/backends", "", http.StatusOK)
		if !strings.Contains(body, `"127.0.0.9"`) {
			t.Errorf("domain backend missing from the API after reload: %s", body)
		}
	})

	t.Run("API-created region stays", func(t *testing.T) {
		e.t = t
		e.mustCall("GET", "/api/v1/regions/ap-south", "", http.StatusOK)
	})

	t.Run("API-deleted domain does not come back", func(t *testing.T) {
		e.t = t
		e.requireRcode(deletedDomain, miekg.RcodeNameError)
		e.mustCall("GET", "/api/v1/domains/"+deletedDomain, "", http.StatusNotFound)
		if servers := e.dnsServers(deletedDomain); servers != nil {
			t.Errorf("DNS registry holds deleted domain %s: %v", deletedDomain, servers)
		}
	})

	t.Run("a second reload changes nothing", func(t *testing.T) {
		e.t = t
		before := e.dnsState()
		e.reload()
		after := e.dnsState()
		if fmt.Sprint(before) != fmt.Sprint(after) {
			t.Errorf("DNS registry changed on an idempotent reload:\nbefore %v\nafter  %v", before, after)
		}
	})
	e.t = t
}

// TestReload_MatchesRestart checks the property the fix is built on: the DNS
// state after a reload equals the DNS state after a restart on the same
// configuration file and the same store.
func TestReload_MatchesRestart(t *testing.T) {
	const (
		keptDomain    = "kept.gslb.example.com"
		removedDomain = "removed.gslb.example.com"
	)
	keptPort := liveBackend(t, "127.0.0.1")
	removedPort := liveBackend(t, "127.0.0.3")
	apiPort := liveBackend(t, "127.0.0.2")
	addedPort := liveBackend(t, "127.0.0.4")

	before := []reloadServer{
		{address: "127.0.0.1", port: keptPort, service: keptDomain},
		{address: "127.0.0.3", port: removedPort, service: removedDomain},
	}
	after := []reloadServer{
		{address: "127.0.0.1", port: keptPort, service: keptDomain, weight: 50},
		{address: "127.0.0.4", port: addedPort, service: keptDomain},
	}

	e := startReloadOverwatch(t, []reloadDomain{{name: keptDomain}, {name: removedDomain}}, before)
	e.createDomain(reloadAPIDomain)
	e.createServer(reloadAPIDomain, "127.0.0.2", apiPort)
	e.mustCall("POST", "/api/v1/domains/"+reloadAPIDomain+"/backends",
		`{"address":"127.0.0.9","port":8080,"weight":10,"region":"us-east","enabled":true}`, http.StatusCreated)

	e.writeConfig([]reloadDomain{{name: keptDomain}}, after)
	e.reload()
	reloaded := e.dnsState()
	e.stop()

	// Restart: a new process on the same store and the same file.
	r := newReloadEnv(t, e.dataDir)
	r.writeConfig([]reloadDomain{{name: keptDomain}}, after)
	r.start()
	restarted := r.dnsState()

	if fmt.Sprint(reloaded) != fmt.Sprint(restarted) {
		t.Errorf("DNS state differs between reload and restart:\nreload  %v\nrestart %v", reloaded, restarted)
	}
	want := map[string][]string{
		keptDomain + ".":      {fmt.Sprintf("127.0.0.1:%d", keptPort), fmt.Sprintf("127.0.0.4:%d", addedPort)},
		reloadAPIDomain + ".": {fmt.Sprintf("127.0.0.2:%d", apiPort), "127.0.0.9:8080"},
	}
	if fmt.Sprint(reloaded) != fmt.Sprint(want) {
		t.Errorf("DNS state after reload = %v, want %v", reloaded, want)
	}
}

// TestReload_KeepsLearnedLatencyProvider: routers built by a reload must be
// wired like routers built at startup. The learned_latency router falls back
// to round-robin without its provider.
func TestReload_KeepsLearnedLatencyProvider(t *testing.T) {
	port := liveBackend(t, "127.0.0.1")
	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain, algorithm: "learned_latency"}},
		[]reloadServer{{address: "127.0.0.1", port: port, service: reloadStaticDomain}})

	provider := func() routing.LearnedLatencyProvider {
		t.Helper()
		entry := e.app.dnsRegistry.Lookup(reloadStaticDomain)
		if entry == nil {
			t.Fatalf("%s is not registered", reloadStaticDomain)
		}
		router, ok := entry.Router.(*routing.LearnedLatencyRouter)
		if !ok {
			t.Fatalf("router is %T, want *routing.LearnedLatencyRouter", entry.Router)
		}
		return router.GetProvider()
	}

	if provider() == nil {
		t.Fatal("no learned latency provider at startup")
	}
	e.reload()
	if provider() == nil {
		t.Fatal("learned latency provider lost by the reload")
	}
}

// TestReload_StoreFailureKeepsLiveState: a reload that cannot read the store
// must fail and keep serving what it served, not drop the API-created state.
func TestReload_StoreFailureKeepsLiveState(t *testing.T) {
	staticPort := liveBackend(t, "127.0.0.1")
	apiPort := liveBackend(t, "127.0.0.2")

	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: reloadStaticDomain}})
	e.createDomain(reloadAPIDomain)
	e.createServer(reloadAPIDomain, "127.0.0.2", apiPort)
	before := e.dnsState()

	if err := e.app.overwatchStore.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	err := handleReload(e.app, e.logger)
	if err == nil {
		t.Fatal("handleReload succeeded although the store could not be read")
	}
	if !strings.Contains(err.Error(), "stored domains") {
		t.Errorf("error does not name the cause: %v", err)
	}
	if after := e.dnsState(); fmt.Sprint(before) != fmt.Sprint(after) {
		t.Errorf("DNS state changed by a failed reload:\nbefore %v\nafter  %v", before, after)
	}
	e.requireResolves(reloadAPIDomain, "127.0.0.2")
	e.requireResolves(reloadStaticDomain, "127.0.0.1")
}

// TestReload_ConcurrentWithAPIChanges reloads in a loop while the REST API
// creates and deletes domains and servers. Whatever the interleaving, the DNS
// registry must end up holding exactly what the API was told. Run with -race.
func TestReload_ConcurrentWithAPIChanges(t *testing.T) {
	staticPort := liveBackend(t, "127.0.0.1")
	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{{address: "127.0.0.1", port: staticPort, service: reloadStaticDomain}})

	const domains = 30
	name := func(i int) string { return fmt.Sprintf("d%02d.gslb.example.com", i) }

	stopReloads := make(chan struct{})
	reloadsDone := make(chan int)
	go func() {
		n := 0
		defer func() { reloadsDone <- n }()
		for {
			select {
			case <-stopReloads:
				return
			default:
			}
			if err := handleReload(e.app, e.logger); err != nil {
				t.Errorf("handleReload: %v", err)
				return
			}
			n++
		}
	}()

	// Every third domain is deleted again; the others keep one server.
	for i := 0; i < domains; i++ {
		e.createDomain(name(i))
		e.createServer(name(i), "127.0.0.1", 20000+i)
		if i%3 == 0 {
			e.mustCall("DELETE", fmt.Sprintf("/api/v1/servers/%s:127.0.0.1:%d", name(i), 20000+i), "", http.StatusNoContent)
			e.mustCall("DELETE", "/api/v1/domains/"+name(i), "", http.StatusNoContent)
		}
	}
	close(stopReloads)
	reloads := <-reloadsDone
	if reloads == 0 {
		t.Fatal("no reload ran while the API was changing state")
	}
	t.Logf("%d reloads ran concurrently with %d domain creations", reloads, domains)

	want := map[string][]string{
		reloadStaticDomain + ".": {fmt.Sprintf("127.0.0.1:%d", staticPort)},
	}
	for i := 0; i < domains; i++ {
		if i%3 != 0 {
			want[name(i)+"."] = []string{fmt.Sprintf("127.0.0.1:%d", 20000+i)}
		}
	}
	if got := e.dnsState(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("DNS state after concurrent reloads:\n got %v\nwant %v", got, want)
	}
}

// TestRestart_StaticServerRemovedFromConfig: the configuration file is the
// source of truth for static servers. One that was removed from the file must
// not be loaded back from the store by the next start.
func TestRestart_StaticServerRemovedFromConfig(t *testing.T) {
	keptPort := liveBackend(t, "127.0.0.1")
	removedPort := liveBackend(t, "127.0.0.3")

	e := startReloadOverwatch(t,
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{
			{address: "127.0.0.1", port: keptPort, service: reloadStaticDomain},
			{address: "127.0.0.3", port: removedPort, service: reloadStaticDomain},
		})
	e.requireResolves(reloadStaticDomain, "127.0.0.1", "127.0.0.3")
	e.stop()

	r := newReloadEnv(t, e.dataDir)
	r.writeConfig(
		[]reloadDomain{{name: reloadStaticDomain}},
		[]reloadServer{{address: "127.0.0.1", port: keptPort, service: reloadStaticDomain}})
	r.start()

	r.requireResolves(reloadStaticDomain, "127.0.0.1")
	if _, ok := r.app.backendRegistry.GetBackend(reloadStaticDomain, "127.0.0.3", removedPort); ok {
		t.Errorf("backend registry holds a static server that is no longer in the configuration file")
	}
}
