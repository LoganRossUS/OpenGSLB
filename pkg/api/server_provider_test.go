// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// newTestServerProvider returns a provider on an in-memory registry that
// holds one static and one agent backend of "web.example.com".
func newTestServerProvider(t *testing.T, validation bool) (*RegistryServerProvider, *overwatch.Registry) {
	t.Helper()
	registry := overwatch.NewRegistry(overwatch.DefaultRegistryConfig(), nil)
	if err := registry.RegisterStatic("web.example.com", "10.0.0.1", 80, 100, "us-east", "tcp"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("agent-1", "us-east", "web.example.com", "10.0.0.2", 80, 100, true); err != nil {
		t.Fatal(err)
	}
	p := NewRegistryServerProvider(registry, HealthCheckDefaults{
		Enabled:            validation,
		Interval:           30 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}, nil)
	p.SetPersistent(true)
	p.SetDomainLookup(func(name string) bool { return name == "web.example.com" })
	return p, registry
}

func wantStatus(t *testing.T, err error, status int, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want status %d", status)
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v (%T) carries no status, want %d", err, err, status)
	}
	if apiErr.Status != status {
		t.Errorf("status = %d, want %d (%s)", apiErr.Status, status, apiErr.Message)
	}
	if contains != "" && !strings.Contains(apiErr.Message, contains) {
		t.Errorf("message %q does not contain %q", apiErr.Message, contains)
	}
}

func decodeCheck(t *testing.T, body string) *ServerHealthCheckRequest {
	t.Helper()
	var req ServerHealthCheckRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return &req
}

func TestParseServerID(t *testing.T) {
	for id, want := range map[string][3]any{
		"web.example.com:10.0.0.1:80":      {"web.example.com", "10.0.0.1", 80},
		"web.example.com:2001:db8::1:8080": {"web.example.com", "2001:db8::1", 8080},
		"web.example.com:::1:443":          {"web.example.com", "::1", 443},
	} {
		service, address, port, err := parseServerID(id)
		if err != nil || service != want[0] || address != want[1] || port != want[2] {
			t.Errorf("parseServerID(%q) = %q, %q, %d, %v; want %v", id, service, address, port, err, want)
		}
		if got := serverID(service, address, port); got != id {
			t.Errorf("serverID does not rebuild %q: %q", id, got)
		}
	}
	for _, id := range []string{"", "nope", "a:b", "a:b:", ":b:80", "a::80", "a:b:port", "a:b:0", "a:b:70000"} {
		if _, _, _, err := parseServerID(id); err == nil {
			t.Errorf("parseServerID(%q) succeeded", id)
		}
	}
}

func TestServerProvider_CreateValidation(t *testing.T) {
	p, _ := newTestServerProvider(t, true)
	base := ServerSpec{Service: "web.example.com", Address: "10.0.0.9", Port: 8080, Enabled: true}

	for name, tc := range map[string]struct {
		mutate   func(s *ServerSpec)
		status   int
		contains string
	}{
		"no service":      {func(s *ServerSpec) { s.Service = "" }, 400, "service is required"},
		"service a URL":   {func(s *ServerSpec) { s.Service = "http://web" }, 400, "not a domain name"},
		"host name":       {func(s *ServerSpec) { s.Address = "backend.internal" }, 400, "IPv4 or IPv6"},
		"port 0":          {func(s *ServerSpec) { s.Port = 0 }, 400, "port"},
		"port too large":  {func(s *ServerSpec) { s.Port = 65536 }, 400, "port"},
		"negative weight": {func(s *ServerSpec) { s.Weight = -1 }, 400, "weight"},
		"huge weight":     {func(s *ServerSpec) { s.Weight = 1000001 }, 400, "weight"},
		"unknown domain":  {func(s *ServerSpec) { s.Service = "nope.example.com" }, 404, "create the domain"},
		"static exists":   {func(s *ServerSpec) { s.Address, s.Port = "10.0.0.1", 80 }, 409, "already exists"},
	} {
		spec := base
		tc.mutate(&spec)
		_, err := p.CreateServer(spec)
		if err == nil {
			t.Errorf("%s: created", name)
			continue
		}
		t.Run(name, func(t *testing.T) { wantStatus(t, err, tc.status, tc.contains) })
	}
	if got := len(p.ListServers()); got != 2 {
		t.Errorf("rejected requests left %d servers, want the 2 that were there", got)
	}

	// Defaults: weight 1; the address in its canonical form.
	spec := base
	spec.Address = "2001:0db8:0000:0000:0000:0000:0000:0001"
	server, err := p.CreateServer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != "web.example.com:2001:db8::1:8080" || server.Weight != 1 || server.Source != "api" || !server.Enabled {
		t.Errorf("created %+v", server)
	}
	if server.Service != "web.example.com" || server.Name != server.Service {
		t.Errorf("service = %q, name = %q; want the domain in both", server.Service, server.Name)
	}
	spec.Address = "2001:db8::1"
	_, err = p.CreateServer(spec)
	wantStatus(t, err, 409, "already exists")
}

func TestServerProvider_HealthCheckValidation(t *testing.T) {
	p, _ := newTestServerProvider(t, true)

	for body, contains := range map[string]string{
		`{"type":"icmp"}`:                       "http, https or tcp",
		`{"type":"tcp","path":"/x"}`:            "http and https checks only",
		`{"type":"tcp","host":"a.example.com"}`: "http and https checks only",
		`{"path":"health"}`:                     `must start with "/"`,
		`{"path":"/a b"}`:                       "white space",
		`{"host":"http://a.example.com"}`:       "not a host name",
		`{"interval":"999ms"}`:                  "between 1s and 3600s",
		`{"interval":"61m"}`:                    "between 1s and 3600s",
		`{"interval":"-30s"}`:                   "between 1s and 3600s",
		`{"timeout":"99ms"}`:                    "between 100ms and 60s",
		`{"timeout":"61s"}`:                     "between 100ms and 60s",
		`{"interval":"5s","timeout":"10s"}`:     "longer than health_check.interval (5s)",
		`{"interval":"2s"}`:                     "the node-wide timeout (5s) is longer than health_check.interval (2s)",
		`{"timeout":"45s"}`:                     "health_check.timeout (45s) is longer than the node-wide interval (30s)",
		`{"healthy_threshold":0}`:               "healthy_threshold must be between 1 and 100",
		`{"unhealthy_threshold":101}`:           "unhealthy_threshold must be between 1 and 100",
	} {
		_, err := p.mergeHealthCheck(nil, "", decodeCheck(t, body), true)
		if err == nil {
			t.Errorf("%s was accepted", body)
			continue
		}
		t.Run(body, func(t *testing.T) { wantStatus(t, err, 400, contains) })
	}

	// Refused while decoding: unknown fields and numbers as durations.
	for _, body := range []string{`{"expected_status":204}`, `{"headers":{"a":"b"}}`, `{"interval":30}`, `{"timeout":5000000000}`} {
		var req ServerHealthCheckRequest
		if err := json.Unmarshal([]byte(body), &req); err == nil {
			t.Errorf("%s decoded without an error", body)
		}
	}

	// Nothing set: the server follows the node-wide settings.
	check, err := p.mergeHealthCheck(nil, "", decodeCheck(t, `{}`), true)
	if err != nil || check != nil {
		t.Errorf("empty request: check = %+v, err = %v; want nil, nil", check, err)
	}
	check, err = p.mergeHealthCheck(nil, "", nil, true)
	if err != nil || check != nil {
		t.Errorf("no request: check = %+v, err = %v; want nil, nil", check, err)
	}
}

func TestServerProvider_HealthCheckMergeAndReplace(t *testing.T) {
	p, _ := newTestServerProvider(t, true)
	server, err := p.CreateServer(ServerSpec{
		Service: "web.example.com", Address: "10.0.0.9", Port: 8080, Enabled: true,
		HealthCheck: decodeCheck(t, `{"type":"https","path":"/ready","host":"web.internal","interval":"10s","timeout":"2s","healthy_threshold":4}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := ServerHealthCheck{
		Enabled: true, Type: "https", Path: "/ready", Host: "web.internal",
		Interval: Duration(10 * time.Second), Timeout: Duration(2 * time.Second),
		HealthyThreshold: 4, UnhealthyThreshold: 3, // 3 is the node-wide value
	}
	if server.HealthCheck != want {
		t.Errorf("created with %+v, want %+v", server.HealthCheck, want)
	}

	// PATCH semantics: only the named field changes.
	check, err := p.UpdateServerHealthCheck(server.ID, *decodeCheck(t, `{"timeout":"1500ms"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	want.Timeout = Duration(1500 * time.Millisecond)
	if *check != want {
		t.Errorf("after the partial update: %+v, want %+v", *check, want)
	}

	// The health_check of a server update is partial too.
	updated, err := p.UpdateServer(server.ID, ServerUpdate{HealthCheck: decodeCheck(t, `{"unhealthy_threshold":5}`)})
	if err != nil {
		t.Fatal(err)
	}
	want.UnhealthyThreshold = 5
	if updated.HealthCheck != want {
		t.Errorf("after the server update: %+v, want %+v", updated.HealthCheck, want)
	}

	// Switching to tcp drops the path and the host.
	check, err = p.UpdateServerHealthCheck(server.ID, *decodeCheck(t, `{"type":"tcp"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if check.Type != "tcp" || check.Path != "" || check.Host != "" || check.Interval != Duration(10*time.Second) {
		t.Errorf("after the switch to tcp: %+v", *check)
	}

	// PUT semantics: what is not named goes back to the node-wide setting.
	check, err = p.UpdateServerHealthCheck(server.ID, *decodeCheck(t, `{"type":"http"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	wantReplaced := ServerHealthCheck{
		Enabled: true, Type: "http", Path: "/",
		Interval: Duration(30 * time.Second), Timeout: Duration(5 * time.Second),
		HealthyThreshold: 2, UnhealthyThreshold: 3,
	}
	if *check != wantReplaced {
		t.Errorf("after the replace: %+v, want %+v", *check, wantReplaced)
	}

	// enabled=false turns the check off for this server only.
	check, err = p.UpdateServerHealthCheck(server.ID, *decodeCheck(t, `{"enabled":false}`), false)
	if err != nil || check.Enabled {
		t.Errorf("enabled=false: %+v, %v", check, err)
	}
	read, err := p.GetServerHealthCheck(server.ID)
	if err != nil || *read != *check {
		t.Errorf("GetServerHealthCheck = %+v, %v; want %+v", read, err, *check)
	}

	// A refused change leaves the check as it was.
	_, err = p.UpdateServerHealthCheck(server.ID, *decodeCheck(t, `{"enabled":true,"interval":"1ms"}`), false)
	wantStatus(t, err, 400, "interval")
	if read, _ := p.GetServerHealthCheck(server.ID); read.Enabled {
		t.Errorf("a refused change was applied: %+v", *read)
	}
}

// Static and agent servers show the node-wide check and cannot be changed.
func TestServerProvider_Ownership(t *testing.T) {
	p, registry := newTestServerProvider(t, true)
	const staticID = "web.example.com:10.0.0.1:80"
	const agentID = "web.example.com:10.0.0.2:80"

	static, err := p.GetServer(staticID)
	if err != nil {
		t.Fatal(err)
	}
	if static.Source != "static" || static.HealthCheck.Type != "tcp" || static.HealthCheck.Path != "" ||
		static.HealthCheck.Interval != Duration(30*time.Second) || static.HealthCheck.UnhealthyThreshold != 3 {
		t.Errorf("static server: %+v", static)
	}
	agent, err := p.GetServer(agentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Source != "agent" || agent.HealthCheck.Type != "http" || agent.HealthCheck.Path != "/" {
		t.Errorf("agent server: %+v", agent)
	}

	weight := 7
	for _, id := range []string{staticID, agentID} {
		_, err := p.UpdateServer(id, ServerUpdate{Weight: &weight})
		wantStatus(t, err, http.StatusConflict, "cannot be changed through the API")
		_, err = p.UpdateServerHealthCheck(id, *decodeCheck(t, `{"type":"tcp"}`), true)
		wantStatus(t, err, http.StatusConflict, "cannot be changed through the API")
	}
	if b, _ := registry.GetBackend("web.example.com", "10.0.0.1", 80); b.Weight != 100 {
		t.Errorf("a refused update changed the weight of the static server to %d", b.Weight)
	}

	wantStatus(t, p.DeleteServer(staticID), http.StatusConflict, "configuration file")
	// An agent's registration can be removed; the agent registers it again.
	if err := p.DeleteServer(agentID); err != nil {
		t.Errorf("delete of an agent server: %v", err)
	}
	wantStatus(t, p.DeleteServer(agentID), http.StatusNotFound, "not found")
	wantStatus(t, p.DeleteServer("nope"), http.StatusNotFound, "service:address:port")
	_, err = p.GetServer("web.example.com:10.0.0.77:80")
	wantStatus(t, err, http.StatusNotFound, "not found")
}

// Without external validation a check cannot run, so asking for one is refused.
func TestServerProvider_ValidationDisabled(t *testing.T) {
	p, _ := newTestServerProvider(t, false)
	spec := ServerSpec{Service: "web.example.com", Address: "10.0.0.9", Port: 8080, Enabled: true}

	spec.HealthCheck = decodeCheck(t, `{"type":"tcp","interval":"30s","timeout":"5s"}`)
	_, err := p.CreateServer(spec)
	wantStatus(t, err, http.StatusConflict, "overwatch.validation.enabled")

	spec.HealthCheck = decodeCheck(t, `{"enabled":false}`)
	server, err := p.CreateServer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if server.HealthCheck.Enabled {
		t.Errorf("health_check.enabled is true on a node that probes nothing: %+v", server.HealthCheck)
	}
	spec.Address, spec.HealthCheck = "10.0.0.10", nil
	server, err = p.CreateServer(spec)
	if err != nil {
		t.Fatal(err)
	}
	if server.HealthCheck.Enabled {
		t.Errorf("health_check.enabled is true on a node that probes nothing: %+v", server.HealthCheck)
	}
}

// Without a persistent store every write is refused with 503 and the reason.
func TestServerProvider_NoPersistence(t *testing.T) {
	p, registry := newTestServerProvider(t, true)
	if err := registry.RegisterAPI("web.example.com", "10.0.0.3", 80, 1, "us-east"); err != nil {
		t.Fatal(err)
	}
	p.SetPersistent(false)
	p.SetStoreUnavailable(`the data directory "/var/lib/opengslb" is not usable (permission denied)`)
	const apiID = "web.example.com:10.0.0.3:80"

	check := func(err error) {
		t.Helper()
		wantStatus(t, err, http.StatusServiceUnavailable, "/var/lib/opengslb")
		wantStatus(t, err, http.StatusServiceUnavailable, "overwatch.data_dir")
		wantStatus(t, err, http.StatusServiceUnavailable, "nothing was changed")
	}
	_, err := p.CreateServer(ServerSpec{Service: "web.example.com", Address: "10.0.0.9", Port: 8080, Enabled: true})
	check(err)
	weight := 9
	_, err = p.UpdateServer(apiID, ServerUpdate{Weight: &weight})
	check(err)
	_, err = p.UpdateServerHealthCheck(apiID, ServerHealthCheckRequest{}, true)
	check(err)
	check(p.DeleteServer(apiID))

	if got := len(p.ListServers()); got != 3 {
		t.Errorf("%d servers after the refused writes, want 3", got)
	}
	if b, _ := registry.GetBackend("web.example.com", "10.0.0.3", 80); b.Weight != 1 {
		t.Errorf("a refused update changed the weight to %d", b.Weight)
	}
	// Reads work.
	if _, err := p.GetServer(apiID); err != nil {
		t.Errorf("GetServer without a store: %v", err)
	}
}

func TestErrorStatus(t *testing.T) {
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{
		{errInvalid("bad %d", 1), 400, "bad 1"},
		{errMissing("gone"), 404, "gone"},
		{errConflict("taken"), 409, "taken"},
		{ErrNotFound, 404, "not found"},
		{errors.New("disk full"), 500, "failed to create server: disk full"},
	} {
		status, message := errorStatus(tc.err, "create server")
		if status != tc.status || message != tc.message {
			t.Errorf("errorStatus(%v) = %d %q, want %d %q", tc.err, status, message, tc.status, tc.message)
		}
	}
	status, message := errorStatus(persistence{}.errNoStore(), "create domain")
	if status != 503 || strings.Contains(message, "not implemented") || !strings.Contains(message, "no persistent store is configured") {
		t.Errorf("no store: %d %q", status, message)
	}
}

func TestResolveRoutingAlgorithm(t *testing.T) {
	for _, tc := range []struct {
		algorithm, policy, want string
	}{
		{"weighted", "", "weighted"},
		{"", "failover", "failover"},
		{"latency", "latency", "latency"},
		{" Round-Robin ", "", "round-robin"},
		{"learned_latency", "", "learned_latency"},
		{"", "", ""},
	} {
		got, err := resolveRoutingAlgorithm(tc.algorithm, tc.policy)
		if err != nil || got != tc.want {
			t.Errorf("resolveRoutingAlgorithm(%q, %q) = %q, %v; want %q", tc.algorithm, tc.policy, got, err, tc.want)
		}
	}
	_, err := resolveRoutingAlgorithm("weighted", "failover")
	wantStatus(t, err, 400, "routing_policy")
	_, err = resolveRoutingAlgorithm("fastest", "")
	wantStatus(t, err, 400, "round-robin, weighted, failover, geolocation, latency, learned_latency")
	_, err = resolveRoutingAlgorithm("", "rr")
	wantStatus(t, err, 400, `"rr"`)
}
