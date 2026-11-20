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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	miekg "github.com/miekg/dns"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/version"
)

// These tests pin the fixes for the mismatches M2 to M8, M15 and M16 of
// docs/api-alignment.md. Each one drives a real overwatch (the wiring of the
// binary) through its REST API and, where the mismatch is about DNS, through a
// real DNS query.

// alignmentOverwatch is a running in-process overwatch.
type alignmentOverwatch struct {
	t           *testing.T
	app         *Application
	api         string // base URL of the REST API
	dns         string // host:port of the DNS listener
	backendPort int    // port of the one live HTTP backend on 127.0.0.1
	stop        func()
}

// startAlignmentOverwatch starts the application with the contract test's
// configuration. mutate may adjust the loaded configuration before start.
func startAlignmentOverwatch(t *testing.T, mutate func(cfg *config.Config)) *alignmentOverwatch {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/down" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)
	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	apiPort := contractFreePort(t)
	dnsPort := contractFreePort(t)
	cfg := loadTestConfig(t, fmt.Sprintf(contractConfig,
		contractFreePort(t), dnsPort, apiPort, backendPort, backendPort))
	// Short cycles so that validation verdicts arrive quickly in tests.
	cfg.Overwatch.Validation.CheckInterval = 200 * time.Millisecond
	cfg.Overwatch.Validation.CheckTimeout = time.Second
	if mutate != nil {
		mutate(cfg)
	}

	app := NewApplication(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := app.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = app.Start(ctx)
	}()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		_ = app.Shutdown(sctx)
		cancel()
		<-done
	}
	t.Cleanup(stop)

	o := &alignmentOverwatch{
		t:           t,
		app:         app,
		api:         fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		dns:         fmt.Sprintf("127.0.0.1:%d", dnsPort),
		backendPort: backendPort,
		stop:        stop,
	}
	waitForAPI(t, o.api)
	return o
}

// call sends a JSON request and returns the status and the raw body.
func (o *alignmentOverwatch) call(method, path, body string) (int, []byte) {
	o.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, o.api+path, rd)
	if err != nil {
		o.t.Fatalf("%s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		o.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// mustCall is call with an expected status; it returns the decoded JSON body.
func (o *alignmentOverwatch) mustCall(method, path, body string, want int) map[string]any {
	o.t.Helper()
	status, raw := o.call(method, path, body)
	if status != want {
		o.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, status, want, raw)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		o.t.Fatalf("%s %s: body is not a JSON object: %v: %s", method, path, err, raw)
	}
	return out
}

// resolve sends one A query and returns the rcode name and the sorted answers.
func (o *alignmentOverwatch) resolve(name string) (string, []string) {
	o.t.Helper()
	q := new(miekg.Msg)
	q.SetQuestion(miekg.Fqdn(name), miekg.TypeA)
	c := &miekg.Client{Net: "udp", Timeout: 2 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		resp, _, err := c.Exchange(q, o.dns)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var addrs []string
		for _, rr := range resp.Answer {
			if a, ok := rr.(*miekg.A); ok {
				addrs = append(addrs, a.A.String())
			}
		}
		sort.Strings(addrs)
		return miekg.RcodeToString[resp.Rcode], addrs
	}
	o.t.Fatalf("DNS query for %s failed: %v", name, lastErr)
	return "", nil
}

// answersOver collects the distinct addresses answered over n queries.
func (o *alignmentOverwatch) answersOver(name string, n int) map[string]int {
	o.t.Helper()
	seen := map[string]int{}
	for i := 0; i < n; i++ {
		_, addrs := o.resolve(name)
		for _, a := range addrs {
			seen[a]++
		}
	}
	return seen
}

// waitFor polls cond until it returns "" or ten seconds have passed. It
// returns the last answer of cond: "" when the condition was met.
func waitFor(cond func() string) string {
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if last = cond(); last == "" {
			return ""
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

// eventually fails the test when cond is not met within ten seconds.
func eventuallyMsg(t *testing.T, what string, cond func() string) {
	t.Helper()
	if last := waitFor(cond); last != "" {
		t.Fatalf("%s: %s", what, last)
	}
}

func field(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()
	var cur any = obj
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("field %s: %v is not an object", strings.Join(path, "."), cur)
		}
		cur, ok = m[p]
		if !ok {
			t.Fatalf("field %s is missing in %v", strings.Join(path, "."), obj)
		}
	}
	return cur
}

// M2: GET /api/v1/config reports the node identity and the build version.
func TestAlignment_M2_ConfigReportsIdentityAndVersion(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	for _, path := range []string{"/api/v1/config", "/api/v1/config/system"} {
		body := o.mustCall("GET", path, "", 200)
		if got := field(t, body, "config", "region"); got != "us-east" {
			t.Errorf("%s: config.region = %v, want us-east", path, got)
		}
		if got := field(t, body, "config", "node_id"); got != "contract-overwatch" {
			t.Errorf("%s: config.node_id = %v, want contract-overwatch", path, got)
		}
		if got := field(t, body, "config", "version"); got != version.Version {
			t.Errorf("%s: config.version = %v, want the build version %s", path, got, version.Version)
		}
		if got := field(t, body, "config", "build_info", "version"); got != version.Version {
			t.Errorf("%s: config.build_info.version = %v, want %s", path, got, version.Version)
		}
		if got := field(t, body, "config", "build_info", "go_version"); !strings.HasPrefix(fmt.Sprint(got), "go") {
			t.Errorf("%s: config.build_info.go_version = %v, want the Go runtime version", path, got)
		}
		if got := field(t, body, "config", "mode"); got != "overwatch" {
			t.Errorf("%s: config.mode = %v, want overwatch", path, got)
		}
		start, err := time.Parse(time.RFC3339, fmt.Sprint(field(t, body, "config", "start_time")))
		if err != nil || start.IsZero() || time.Since(start) > time.Hour {
			t.Errorf("%s: config.start_time = %v, want the process start time", path, field(t, body, "config", "start_time"))
		}
	}
}

// M5: routing_algorithm is the canonical field; routing_policy is a deprecated alias.
func TestAlignment_M5_RoutingAlgorithmField(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	const dom = "m5.gslb.example.com"

	// What Overlord sends today: routing_algorithm (and regions).
	body := o.mustCall("POST", "/api/v1/domains", `{"name":"`+dom+`","ttl":45,"routing_algorithm":"weighted","enabled":true,"regions":["us-east"]}`, 201)
	if got := field(t, body, "domain", "routing_algorithm"); got != "weighted" {
		t.Fatalf("create: domain.routing_algorithm = %v, want weighted", got)
	}
	if got := field(t, body, "domain", "routing_policy"); got != "weighted" {
		t.Errorf("create: deprecated domain.routing_policy = %v, want the same value as routing_algorithm", got)
	}
	entry := o.app.dnsRegistry.Lookup(dom)
	if entry == nil || entry.Router.Algorithm() != "weighted" {
		t.Fatalf("DNS registry entry for %s does not use the weighted router: %+v", dom, entry)
	}

	// Update with the canonical name.
	body = o.mustCall("PUT", "/api/v1/domains/"+dom, `{"routing_algorithm":"failover"}`, 200)
	if got := field(t, body, "domain", "routing_algorithm"); got != "failover" {
		t.Errorf("update: domain.routing_algorithm = %v, want failover", got)
	}
	if got := o.app.dnsRegistry.Lookup(dom).Router.Algorithm(); got != "failover" {
		t.Errorf("update: DNS router is %s, want failover", got)
	}

	// The deprecated alias keeps working on input.
	body = o.mustCall("PATCH", "/api/v1/domains/"+dom, `{"routing_policy":"round-robin"}`, 200)
	if got := field(t, body, "domain", "routing_algorithm"); got != "round-robin" {
		t.Errorf("alias update: domain.routing_algorithm = %v, want round-robin", got)
	}
	body = o.mustCall("GET", "/api/v1/domains/"+dom, "", 200)
	if got := field(t, body, "domain", "routing_algorithm"); got != "round-robin" {
		t.Errorf("read back: domain.routing_algorithm = %v, want round-robin", got)
	}

	// Both names with different values are ambiguous.
	status, raw := o.call("PUT", "/api/v1/domains/"+dom, `{"routing_algorithm":"weighted","routing_policy":"failover"}`)
	if status != 400 || !strings.Contains(string(raw), "routing_policy") {
		t.Errorf("conflicting names: status %d body %s, want 400 naming routing_policy", status, raw)
	}
	// Both names with the same value are fine.
	o.mustCall("PUT", "/api/v1/domains/"+dom, `{"routing_algorithm":"weighted","routing_policy":"weighted"}`, 200)

	// An unknown algorithm is rejected instead of being stored.
	status, raw = o.call("POST", "/api/v1/domains", `{"name":"bad.gslb.example.com","routing_algorithm":"fastest"}`)
	if status != 400 || !strings.Contains(string(raw), "fastest") {
		t.Errorf("unknown algorithm: status %d body %s, want 400 naming the value", status, raw)
	}
	if status, _ := o.call("GET", "/api/v1/domains/bad.gslb.example.com", ""); status != 404 {
		t.Errorf("a domain with an unknown algorithm was stored (GET answered %d)", status)
	}

	// The config-file domain reports the canonical field too.
	body = o.mustCall("GET", "/api/v1/domains/app.gslb.example.com", "", 200)
	if got := field(t, body, "domain", "routing_algorithm"); got != "round-robin" {
		t.Errorf("config domain: routing_algorithm = %v, want round-robin", got)
	}
}

// backendDomain creates an API domain and returns a helper bound to it.
type backendDomain struct {
	o    *alignmentOverwatch
	name string
}

func (o *alignmentOverwatch) newDomain(name string) backendDomain {
	o.t.Helper()
	o.mustCall("POST", "/api/v1/domains", `{"name":"`+name+`","ttl":5,"routing_algorithm":"round-robin","enabled":true}`, 201)
	return backendDomain{o: o, name: name}
}

func (d backendDomain) id(address string, port int) string {
	return fmt.Sprintf("%s:%s:%d", d.name, address, port)
}

// add creates a backend through the domain endpoint, the way Overlord does.
// healthCheck is the JSON of the health_check object, or "" for none.
func (d backendDomain) add(address string, port int, healthCheck string) (int, []byte) {
	d.o.t.Helper()
	body := fmt.Sprintf(`{"address":%q,"port":%d,"weight":100,"region":"us-east","enabled":true`, address, port)
	if healthCheck != "" {
		body += `,"health_check":` + healthCheck
	}
	return d.o.call("POST", "/api/v1/domains/"+d.name+"/backends", body+"}")
}

func (d backendDomain) mustAdd(address string, port int, healthCheck string) map[string]any {
	d.o.t.Helper()
	status, raw := d.add(address, port, healthCheck)
	if status != 201 {
		d.o.t.Fatalf("POST /domains/%s/backends %s:%d: status %d, want 201; body: %s", d.name, address, port, status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		d.o.t.Fatalf("POST /domains/%s/backends: %v: %s", d.name, err, raw)
	}
	return out
}

// answered waits until DNS answers every address in want for the domain.
func (d backendDomain) answered(want ...string) {
	d.o.t.Helper()
	eventuallyMsg(d.o.t, fmt.Sprintf("%s answers %v", d.name, want), func() string {
		seen := d.o.answersOver(d.name, 8)
		for _, addr := range want {
			if seen[addr] == 0 {
				return fmt.Sprintf("answers so far: %v", seen)
			}
		}
		return ""
	})
}

// The servers of the tests below have addresses of their own (127.0.0.3 and
// up). The health provider of the DNS handler looks a server up by address and
// port alone, so a server that shares both with a server of the configuration
// file would be answered or not according to either one's status.

// M4: a domain that exists and has no backends answers 200 with an empty
// list, and every listed backend carries its source and its health check.
func TestAlignment_M4_DomainBackendsListing(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	d := o.newDomain("m4.gslb.example.com")

	status, raw := o.call("GET", "/api/v1/domains/"+d.name+"/backends", "")
	if status != 200 {
		t.Errorf("domain without backends: status %d body %s, want 200 with an empty list", status, raw)
	} else {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("body: %v: %s", err, raw)
		}
		if got, ok := body["backends"].([]any); !ok || len(got) != 0 {
			t.Errorf("domain without backends: backends = %v, want []", body["backends"])
		}
		if got := body["total"]; got != float64(0) {
			t.Errorf("domain without backends: total = %v, want 0", got)
		}
	}
	if status, _ := o.call("GET", "/api/v1/domains/nope.gslb.example.com/backends", ""); status != 404 {
		t.Errorf("unknown domain: backends answered %d, want 404", status)
	}

	port := listenTCP(t, "127.0.0.3")
	d.mustAdd("127.0.0.3", port, `{"type":"tcp","interval":"7s","timeout":"3s"}`)
	body := o.mustCall("GET", "/api/v1/domains/"+d.name+"/backends", "", 200)
	listed := field(t, body, "backends").([]any)
	if len(listed) != 1 {
		t.Fatalf("backends listed = %d, want 1: %v", len(listed), listed)
	}
	bm := listed[0].(map[string]any)
	if bm["source"] != "api" {
		t.Errorf("API backend: source = %v, want api", bm["source"])
	}
	for k, want := range map[string]any{"type": "tcp", "interval": "7s", "timeout": "3s", "enabled": true} {
		hc, _ := bm["health_check"].(map[string]any)
		if hc[k] != want {
			t.Errorf("API backend: health_check.%s = %v, want %v", k, hc[k], want)
		}
	}

	// A backend of the configuration file: source static, the check type of
	// its region and the node-wide validation timing.
	body = o.mustCall("GET", "/api/v1/domains/app.gslb.example.com/backends", "", 200)
	static := field(t, body, "backends").([]any)
	if len(static) != 2 {
		t.Errorf("config-file domain lists %d backends, want its 2 servers", len(static))
	}
	for _, b := range static {
		bm := b.(map[string]any)
		if bm["source"] != "static" {
			t.Errorf("config-file backend %v: source = %v, want static", bm["id"], bm["source"])
		}
		hc, _ := bm["health_check"].(map[string]any)
		if hc["type"] != "http" || hc["interval"] != "200ms" || hc["timeout"] != "1s" {
			t.Errorf("config-file backend %v: health_check = %v, want type http, interval 200ms, timeout 1s", bm["id"], hc)
		}
	}
}

// M6: the health_check sent with a backend is applied. A backend whose check
// fails is not answered; when the check is changed to one that passes, it is.
func TestAlignment_M6_BackendHealthCheckIsApplied(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	d := o.newDomain("m6.gslb.example.com")
	upPort := listenHTTP(t, "127.0.0.3")
	downPort := listenHTTP(t, "127.0.0.4")

	body := d.mustAdd("127.0.0.3", upPort, `{"type":"http","path":"/","interval":"1s","timeout":"500ms"}`)
	for k, want := range map[string]any{"type": "http", "path": "/", "interval": "1s", "timeout": "500ms", "enabled": true} {
		hc, _ := field(t, body, "backend").(map[string]any)["health_check"].(map[string]any)
		if hc[k] != want {
			t.Errorf("create: backend.health_check.%s = %v, want %v", k, hc[k], want)
		}
	}
	// The server answers 503 on /down and 200 everywhere else.
	d.mustAdd("127.0.0.4", downPort, `{"type":"http","path":"/down","interval":"1s","timeout":"500ms"}`)

	d.answered("127.0.0.3")
	eventuallyMsg(t, "backend with a failing health check is not answered", func() string {
		if seen := o.answersOver(d.name, 8); seen["127.0.0.4"] != 0 {
			return fmt.Sprintf("127.0.0.4 is answered: %v", seen)
		}
		return ""
	})
	status, raw := o.call("GET", "/api/v1/servers/"+d.id("127.0.0.4", downPort), "")
	if status != 200 || !strings.Contains(string(raw), `"healthy":false`) {
		t.Errorf("server with a failing health check: status %d body %s, want healthy false", status, raw)
	}

	// The check can be read and changed as the server's health check.
	check := o.mustCall("GET", "/api/v1/servers/"+d.id("127.0.0.4", downPort)+"/health-check", "", 200)
	if got := field(t, check, "health_check", "path"); got != "/down" {
		t.Errorf("GET health-check: path = %v, want /down", got)
	}
	o.mustCall("PATCH", "/api/v1/servers/"+d.id("127.0.0.4", downPort)+"/health-check", `{"path":"/"}`, 200)
	d.answered("127.0.0.3", "127.0.0.4")
}

// M7: DELETE /domains/{name}/backends/{id} works, and the deleted address is
// no longer answered in DNS.
func TestAlignment_M7_DeletedBackendLeavesDNS(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	d := o.newDomain("m7.gslb.example.com")
	keepPort := listenTCP(t, "127.0.0.3")
	gonePort := listenTCP(t, "127.0.0.4")
	d.mustAdd("127.0.0.3", keepPort, `{"type":"tcp"}`)
	d.mustAdd("127.0.0.4", gonePort, `{"type":"tcp"}`)
	// Both are answered before the delete. Not fatal, so that the delete is
	// exercised either way.
	if last := waitFor(func() string {
		if seen := o.answersOver(d.name, 8); seen["127.0.0.3"] == 0 || seen["127.0.0.4"] == 0 {
			return fmt.Sprintf("answers so far: %v", seen)
		}
		return ""
	}); last != "" {
		t.Errorf("before the delete DNS does not answer both backends: %s", last)
	}

	goneID := d.id("127.0.0.4", gonePort)
	if status, raw := o.call("DELETE", "/api/v1/domains/"+d.name+"/backends/"+goneID, ""); status != 204 {
		t.Errorf("DELETE backend: status %d body %s, want 204", status, raw)
	}
	// The answer changes with the delete, not some time after it.
	seen := o.answersOver(d.name, 20)
	if seen["127.0.0.4"] != 0 {
		t.Errorf("after the delete DNS still answers the deleted address: %v", seen)
	}
	if seen["127.0.0.3"] == 0 {
		t.Errorf("after the delete DNS does not answer the remaining address: %v", seen)
	}
	body := o.mustCall("GET", "/api/v1/domains/"+d.name+"/backends", "", 200)
	if got := field(t, body, "backends").([]any); len(got) != 1 {
		t.Errorf("after the delete the domain lists %d backends, want 1", len(got))
	}
	if status, _ := o.call("DELETE", "/api/v1/domains/"+d.name+"/backends/"+goneID, ""); status != 404 {
		t.Errorf("second delete answered %d, want 404", status)
	}
	// An ID of another domain is not a backend of this one.
	other := o.newDomain("m7b.gslb.example.com")
	other.mustAdd("127.0.0.5", 8080, `{"enabled":false}`)
	if status, _ := o.call("DELETE", "/api/v1/domains/"+d.name+"/backends/"+other.id("127.0.0.5", 8080), ""); status != 404 {
		t.Errorf("delete through the wrong domain answered %d, want 404", status)
	}
	o.mustCall("GET", "/api/v1/servers/"+other.id("127.0.0.5", 8080), "", 200)

	// A backend of the configuration file cannot be removed through the API.
	staticID := fmt.Sprintf("app.gslb.example.com:127.0.0.1:%d", o.backendPort)
	if status, raw := o.call("DELETE", "/api/v1/domains/app.gslb.example.com/backends/"+staticID, ""); status != 409 {
		t.Errorf("DELETE of a config-file backend: status %d body %s, want 409", status, raw)
	}

	// Deleting the domain removes the servers created for it.
	o.mustCall("DELETE", "/api/v1/domains/"+d.name, "", 204)
	if status, _ := o.call("GET", "/api/v1/servers/"+d.id("127.0.0.3", keepPort), ""); status != 404 {
		t.Errorf("server of a deleted domain still exists (GET answered %d)", status)
	}
}

// M8: health-check interval and timeout are duration strings. A bare number
// is refused with a message that shows the string form, as is every value
// that makes no sense.
func TestAlignment_M8_HealthCheckDurations(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	d := o.newDomain("m8.gslb.example.com")

	// What Overlord sends today: seconds as numbers.
	status, raw := d.add("127.0.0.9", 8080, `{"type":"tcp","interval":30,"timeout":5}`)
	if status != 400 || !strings.Contains(string(raw), `30s`) || !strings.Contains(string(raw), "ambiguous") {
		t.Errorf("numeric interval: status %d body %s, want 400 that shows the string form", status, raw)
	}
	for _, bad := range []string{
		`{"type":"tcp","interval":"0s"}`,
		`{"type":"tcp","interval":"-5s"}`,
		`{"type":"tcp","interval":"30"}`,
		`{"type":"tcp","interval":"500ms"}`,
		`{"type":"tcp","interval":"2h"}`,
		`{"type":"tcp","timeout":"1ns"}`,
		`{"type":"tcp","timeout":"5m"}`,
		`{"type":"tcp","interval":"5s","timeout":"10s"}`,
		`{"type":"tcp","timeout":5.5}`,
		`{"type":"tcp","timeout":null,"interval":"abc"}`,
		`{"type":"icmp"}`,
		`{"type":"tcp","path":"/health"}`,
		`{"type":"http","path":"health"}`,
		`{"type":"http","unhealthy_threshold":-1}`,
		`{"type":"http","healthy_threshold":0}`,
		`{"type":"http","expected_status":204}`,
	} {
		status, raw := d.add("127.0.0.9", 8080, bad)
		if status != 400 {
			t.Errorf("health_check %s: status %d body %s, want 400", bad, status, raw)
		}
	}
	if status, _ := o.call("GET", "/api/v1/servers/"+d.id("127.0.0.9", 8080), ""); status != 404 {
		t.Errorf("a backend with a rejected health check was stored (GET answered %d)", status)
	}

	// Strings are accepted on every endpoint that takes a health check, and
	// come back as strings.
	body := d.mustAdd("127.0.0.9", 8080, `{"type":"tcp","interval":"30s","timeout":"5s"}`)
	if got := field(t, body, "backend", "health_check", "interval"); got != "30s" {
		t.Errorf("create: interval = %v, want \"30s\"", got)
	}
	id := d.id("127.0.0.9", 8080)
	body = o.mustCall("PUT", "/api/v1/servers/"+id, `{"health_check":{"type":"tcp","interval":"1m30s","timeout":"1500ms"}}`, 200)
	if got := field(t, body, "server", "health_check", "interval"); got != "90s" {
		t.Errorf("PUT /servers: interval = %v, want \"90s\"", got)
	}
	if got := field(t, body, "server", "health_check", "timeout"); got != "1500ms" {
		t.Errorf("PUT /servers: timeout = %v, want \"1500ms\"", got)
	}
	// What Overlord's updateServerOnNodes sends today.
	if status, raw := o.call("PUT", "/api/v1/servers/"+id, `{"weight":5,"health_check":{"type":"http","path":"/health","interval":30,"timeout":5}}`); status != 400 {
		t.Errorf("PUT /servers with numeric durations: status %d body %s, want 400", status, raw)
	}
	body = o.mustCall("GET", "/api/v1/servers/"+id, "", 200)
	if got := field(t, body, "server", "weight"); got != float64(100) {
		t.Errorf("a refused update changed the weight to %v", got)
	}
	body = o.mustCall("PUT", "/api/v1/servers/"+id+"/health-check", `{"type":"tcp","interval":"10s","timeout":"2s"}`, 200)
	if got := field(t, body, "health_check", "interval"); got != "10s" {
		t.Errorf("PUT health-check: interval = %v, want \"10s\"", got)
	}
	if status, raw := o.call("PATCH", "/api/v1/servers/"+id+"/health-check", `{"interval":30000000000}`); status != 400 {
		t.Errorf("PATCH health-check with nanoseconds: status %d body %s, want 400", status, raw)
	}
}

// M15 and M3: a backend created through the domain endpoint is a server. It
// is listed under /servers with the link to its domain in "service", it has
// the same representation in both listings, and an update through
// /servers/{id} reaches it, its domain listing and the DNS registry.
func TestAlignment_M15_BackendIsAServer(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	d := o.newDomain("m15.gslb.example.com")
	port := listenTCP(t, "127.0.0.3")
	id := d.id("127.0.0.3", port)

	created := d.mustAdd("127.0.0.3", port, `{"type":"tcp","interval":"1s","timeout":"500ms"}`)
	if got := field(t, created, "backend", "id"); got != id {
		t.Errorf("create: backend.id = %v, want %s", got, id)
	}
	if got := field(t, created, "backend").(map[string]any)["service"]; got != d.name {
		t.Errorf("create: backend.service = %v, want %s", got, d.name)
	}

	body := o.mustCall("GET", "/api/v1/servers", "", 200)
	var asServer map[string]any
	for _, s := range field(t, body, "servers").([]any) {
		if sm := s.(map[string]any); sm["id"] == id {
			asServer = sm
		}
	}
	if asServer == nil {
		t.Fatalf("GET /servers does not list the backend %s", id)
	}
	// M3: the link from a server to its domain.
	if asServer["service"] != d.name {
		t.Errorf("server %s: service = %v, want %s", id, asServer["service"], d.name)
	}
	if asServer["source"] != "api" {
		t.Errorf("server %s: source = %v, want api", id, asServer["source"])
	}
	// M3: one representation. The object under /servers has exactly the
	// fields of the object under /domains/{name}/backends.
	body = o.mustCall("GET", "/api/v1/domains/"+d.name+"/backends", "", 200)
	asBackend := field(t, body, "backends").([]any)[0].(map[string]any)
	for k := range asServer {
		if _, ok := asBackend[k]; !ok {
			t.Errorf("field %q is in the server and not in the backend representation", k)
		}
	}
	for k := range asBackend {
		if _, ok := asServer[k]; !ok {
			t.Errorf("field %q is in the backend and not in the server representation", k)
		}
	}
	o.mustCall("GET", "/api/v1/servers/"+id, "", 200)
	d.answered("127.0.0.3")

	// What Overlord's updateServerOnNodes does: PUT /servers/{id}.
	status, raw := o.call("PUT", "/api/v1/servers/"+id, `{"weight":70,"region":"eu-west","enabled":true}`)
	if status != 200 {
		t.Fatalf("PUT /servers/%s: status %d body %s, want 200", id, status, raw)
	}
	body = o.mustCall("GET", "/api/v1/domains/"+d.name+"/backends", "", 200)
	asBackend = field(t, body, "backends").([]any)[0].(map[string]any)
	if asBackend["weight"] != float64(70) || asBackend["region"] != "eu-west" {
		t.Errorf("after the update the domain lists weight %v region %v, want 70 eu-west", asBackend["weight"], asBackend["region"])
	}
	var dnsWeight int
	var dnsRegion string
	for _, s := range o.app.dnsRegistry.Lookup(d.name).Servers {
		if s.Address.String() == "127.0.0.3" {
			dnsWeight, dnsRegion = s.Weight, s.Region
		}
	}
	if dnsWeight != 70 || dnsRegion != "eu-west" {
		t.Errorf("after the update the DNS registry has weight %d region %q, want 70 eu-west", dnsWeight, dnsRegion)
	}

	// enabled=false takes the server out of the answer without deleting it.
	o.mustCall("PATCH", "/api/v1/servers/"+id, `{"enabled":false}`, 200)
	if rcode, addrs := o.resolve(d.name); len(addrs) != 0 {
		t.Errorf("disabled server is still answered: rcode %s answers %v", rcode, addrs)
	}
	body = o.mustCall("GET", "/api/v1/servers?enabled=false", "", 200)
	if got := field(t, body, "servers").([]any); len(got) != 1 {
		t.Errorf("GET /servers?enabled=false lists %d servers, want the disabled one", len(got))
	}
	o.mustCall("PATCH", "/api/v1/servers/"+id, `{"enabled":true}`, 200)
	d.answered("127.0.0.3")

	// The identity of a server cannot be changed, and a server of the
	// configuration file cannot be changed at all.
	if status, raw := o.call("PUT", "/api/v1/servers/"+id, `{"address":"127.0.0.99"}`); status != 400 {
		t.Errorf("PUT that changes the address: status %d body %s, want 400", status, raw)
	}
	staticID := fmt.Sprintf("app.gslb.example.com:127.0.0.1:%d", o.backendPort)
	if status, raw := o.call("PUT", "/api/v1/servers/"+staticID, `{"weight":1}`); status != 409 {
		t.Errorf("PUT of a config-file server: status %d body %s, want 409", status, raw)
	}
}

// M15, the other direction: a server created through /servers is a backend of
// its domain, and the canonical link is the service field.
func TestAlignment_M15_ServerIsABackend(t *testing.T) {
	o := startAlignmentOverwatch(t, nil)
	const dom = "two.gslb.example.com"
	o.mustCall("POST", "/api/v1/domains", `{"name":"`+dom+`","ttl":5}`, 201)

	body := o.mustCall("POST", "/api/v1/servers",
		fmt.Sprintf(`{"service":"%s","address":"127.0.0.1","port":%d,"weight":10,"region":"us-east","health_check":{"type":"tcp"}}`, dom, o.backendPort), 201)
	id := fmt.Sprintf("%s:127.0.0.1:%d", dom, o.backendPort)
	if got := field(t, body, "server", "id"); got != id {
		t.Errorf("create: server.id = %v, want %s", got, id)
	}
	if got := field(t, body, "server", "enabled"); got != true {
		t.Errorf("create without enabled: server.enabled = %v, want true", got)
	}
	body = o.mustCall("GET", "/api/v1/domains/"+dom+"/backends", "", 200)
	listed := field(t, body, "backends").([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["id"] != id {
		t.Fatalf("the domain does not list the server as its backend: %v", listed)
	}
	eventuallyMsg(t, "server created through /servers answered in DNS", func() string {
		if rcode, addrs := o.resolve(dom); len(addrs) != 1 || addrs[0] != "127.0.0.1" {
			return fmt.Sprintf("rcode %s answers %v", rcode, addrs)
		}
		return ""
	})

	// Duplicate: 409. Missing service: 400. Unknown domain: 404.
	if status, raw := o.call("POST", "/api/v1/servers", fmt.Sprintf(`{"service":"%s","address":"127.0.0.1","port":%d,"region":"us-east"}`, dom, o.backendPort)); status != 409 {
		t.Errorf("duplicate server: status %d body %s, want 409", status, raw)
	}
	if status, raw := o.call("POST", "/api/v1/servers", `{"address":"127.0.0.7","port":9002,"region":"us-east"}`); status != 400 {
		t.Errorf("server without a service: status %d body %s, want 400", status, raw)
	}
	if status, raw := o.call("POST", "/api/v1/servers", `{"service":"nope.gslb.example.com","address":"127.0.0.7","port":9002,"region":"us-east"}`); status != 404 {
		t.Errorf("server for an unknown domain: status %d body %s, want 404", status, raw)
	}
	// The deprecated ways to name the service still work.
	o.mustCall("POST", "/api/v1/servers", `{"name":"`+dom+`","address":"127.0.0.8","port":9003,"region":"us-east"}`, 201)
	o.mustCall("POST", "/api/v1/servers", `{"metadata":{"service":"`+dom+`"},"address":"127.0.0.9","port":9004,"region":"us-east"}`, 201)

	o.mustCall("DELETE", "/api/v1/servers/"+id, "", 204)
	eventuallyMsg(t, "deleted server no longer answered", func() string {
		if seen := o.answersOver(dom, 6); seen["127.0.0.1"] != 0 {
			return fmt.Sprintf("still answered: %v", seen)
		}
		return ""
	})
}

// M16: without a store, writes are refused with an accurate status and message.
func TestAlignment_M16_WritesWithoutPersistence(t *testing.T) {
	// A data directory below a regular file can never be created, whatever
	// user the test runs as.
	var blocked string
	o := startAlignmentOverwatch(t, func(cfg *config.Config) {
		file := filepath.Join(t.TempDir(), "not-a-directory")
		if err := writeFile(file, "x"); err != nil {
			t.Fatal(err)
		}
		blocked = filepath.Join(file, "data")
		cfg.Overwatch.DataDir = blocked
	})

	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/v1/domains", `{"name":"x.gslb.example.com"}`},
		{"PUT", "/api/v1/domains/app.gslb.example.com", `{"ttl":10}`},
		{"DELETE", "/api/v1/domains/x.gslb.example.com", ""},
		{"POST", "/api/v1/domains/app.gslb.example.com/backends", `{"address":"127.0.0.3","port":8080,"region":"us-east"}`},
		{"POST", "/api/v1/servers", `{"service":"app.gslb.example.com","address":"127.0.0.3","port":8080,"region":"us-east"}`},
		{"POST", "/api/v1/regions", `{"name":"ap-south","code":"ap-south"}`},
		{"PUT", "/api/v1/regions/us-east", `{"description":"d"}`},
		{"DELETE", "/api/v1/regions/ap-south", ""},
	} {
		status, raw := o.call(c.method, c.path, c.body)
		if status != http.StatusServiceUnavailable {
			t.Errorf("%s %s without a store: status %d body %s, want 503", c.method, c.path, status, raw)
			continue
		}
		msg := string(raw)
		if strings.Contains(msg, "not implemented") {
			t.Errorf("%s %s: the error still says \"not implemented\": %s", c.method, c.path, msg)
		}
		if !strings.Contains(msg, "data_dir") || !strings.Contains(msg, blocked) {
			t.Errorf("%s %s: the error does not name overwatch.data_dir and the directory: %s", c.method, c.path, msg)
		}
	}

	// Nothing was accepted, and reads and DNS for config-file state still work.
	if status, _ := o.call("GET", "/api/v1/domains/x.gslb.example.com", ""); status != 404 {
		t.Errorf("a domain was created without a store (GET answered %d)", status)
	}
	o.mustCall("GET", "/api/v1/domains", "", 200)
	eventuallyMsg(t, "config-file domain resolves without a store", func() string {
		if rcode, addrs := o.resolve("app.gslb.example.com"); len(addrs) == 0 {
			return fmt.Sprintf("rcode %s, no answers", rcode)
		}
		return ""
	})
}

// listenHTTP serves 200 on every path except /down (503) on a free port of
// the given loopback address. It returns the port.
func listenHTTP(t *testing.T, address string) int {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatalf("listen on %s: %v", address, err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/down" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	_ = srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	t.Cleanup(srv.Close)
	return l.Addr().(*net.TCPAddr).Port
}

// listenTCP accepts and closes connections on a free port of the given
// loopback address, so that a TCP health check of it passes. It returns the port.
func listenTCP(t *testing.T, address string) int {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatalf("listen on %s: %v", address, err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}

// GET /api/v1/config/dns reports the zones the node serves with their apex
// SOA and NS settings as resolved (configured values and defaults), the UDP
// size limit and return_last_healthy. Zones are set in the configuration
// file, so the endpoint is read-only for them.
func TestConfigDNS_ReportsZoneApexSettings(t *testing.T) {
	o := startAlignmentOverwatch(t, func(cfg *config.Config) {
		cfg.DNS.Zones = []config.Zone{
			{Name: "gslb.example.com"},
			{
				Name: "Apex.Example.NET",
				TTL:  7200,
				SOA: config.ZoneSOA{
					PrimaryNS:   "ns1.example.net",
					Contact:     "dns-admin@example.net",
					Serial:      2026112001,
					NegativeTTL: 30,
				},
				Nameservers: []string{"ns1.example.net", "ns2.example.net"},
			},
		}
		cfg.DNS.ReturnLastHealthy = true
	})

	body := o.mustCall("GET", "/api/v1/config/dns", "", 200)
	if got := field(t, body, "config", "max_udp_size"); got != float64(1232) {
		t.Errorf("config.max_udp_size = %v, want the default 1232", got)
	}
	if got := field(t, body, "config", "return_last_healthy"); got != true {
		t.Errorf("config.return_last_healthy = %v, want true", got)
	}
	zones, ok := field(t, body, "config", "zones").([]any)
	if !ok || len(zones) != 2 {
		t.Fatalf("config.zones = %v, want two zones", field(t, body, "config", "zones"))
	}

	// Sorted by name; names are canonical (lower case, trailing dot).
	explicit := zones[0].(map[string]any)
	want := map[string]any{
		"name": "apex.example.net.",
		"ttl":  float64(7200),
		"soa": map[string]any{
			"primary_ns":   "ns1.example.net.",
			"contact":      "dns-admin.example.net.",
			"serial":       float64(2026112001),
			"refresh":      float64(3600),
			"retry":        float64(600),
			"expire":       float64(604800),
			"negative_ttl": float64(30),
		},
		"nameservers": []any{"ns1.example.net.", "ns2.example.net."},
	}
	if !reflect.DeepEqual(explicit, want) {
		t.Errorf("zone with settings:\n got  %v\n want %v", explicit, want)
	}

	// A zone given by name only gets every field from the defaults, derived
	// from the node identity.
	defaulted := zones[1].(map[string]any)
	if defaulted["name"] != "gslb.example.com." {
		t.Fatalf("second zone = %v, want gslb.example.com.", defaulted["name"])
	}
	soa := defaulted["soa"].(map[string]any)
	if soa["primary_ns"] == "" || soa["contact"] != "hostmaster.gslb.example.com." || soa["serial"] != float64(1) {
		t.Errorf("defaulted SOA = %v", soa)
	}
	if ns := defaulted["nameservers"].([]any); len(ns) != 1 || ns[0] != soa["primary_ns"] {
		t.Errorf("defaulted nameservers = %v, want the primary nameserver %v", ns, soa["primary_ns"])
	}
}
