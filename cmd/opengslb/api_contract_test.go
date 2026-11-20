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
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/test/apicontract"
)

// The contract test starts a real overwatch in-process (the same wiring the
// binary uses), calls every operation in api/openapi.yaml and validates the
// real responses against the spec. A handler change that alters a response
// shape, or a spec change the handlers do not honor, fails this test.

const specPath = "../../api/openapi.yaml"

// contractConfig is a minimal overwatch configuration. Placeholders, in order:
// gossip port, DNS port, API port, backend port (twice).
const contractConfig = `
mode: overwatch
overwatch:
  identity:
    node_id: contract-overwatch
    region: us-east
  gossip:
    bind_address: "127.0.0.1:%d"
    encryption_key: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
  validation:
    enabled: true
    check_interval: 30s
    check_timeout: 2s
  dnssec:
    enabled: false
    security_acknowledgment: "I understand that disabling DNSSEC removes cryptographic authentication of DNS responses"
  geolocation:
    database_path: "../../test/apicontract/testdata/geo-fixture.mmdb"
    default_region: us-east
    custom_mappings:
      - cidr: "10.0.0.0/8"
        region: us-east
        comment: "internal"
dns:
  listen_address: "127.0.0.1:%d"
  default_ttl: 30
  zones:
    - gslb.example.com
api:
  enabled: true
  address: "127.0.0.1:%d"
regions:
  - name: us-east
    countries: ["US"]
    continents: ["NA"]
    servers:
      - address: 127.0.0.1
        port: %d
        weight: 100
        service: app.gslb.example.com
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 3
      success_threshold: 1
  - name: eu-west
    countries: ["DE"]
    continents: ["EU"]
    servers:
      - address: 127.0.0.2
        port: %d
        weight: 50
        service: app.gslb.example.com
    health_check:
      type: http
      interval: 2s
      timeout: 1s
      path: /
      failure_threshold: 3
      success_threshold: 1
domains:
  - name: app.gslb.example.com
    routing_algorithm: round-robin
    regions: [us-east, eu-west]
    ttl: 30
logging:
  level: error
  format: text
metrics:
  enabled: false
`

type contractCall struct {
	op     string // operationId in api/openapi.yaml
	method string
	path   string
	body   string
	want   int
}

func contractFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForAPI(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/v1/live")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("API at %s did not come up", base)
}

// startContractOverwatch starts the real application and returns its API base
// URL and the port of the one live backend (127.0.0.1:port).
func startContractOverwatch(t *testing.T) (string, int) {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)
	backendPort := backend.Listener.Addr().(*net.TCPAddr).Port

	apiPort := contractFreePort(t)
	cfg := loadTestConfig(t, fmt.Sprintf(contractConfig,
		contractFreePort(t), contractFreePort(t), apiPort, backendPort, backendPort))

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
	t.Cleanup(func() {
		sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer scancel()
		_ = app.Shutdown(sctx)
		cancel()
		<-done
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	waitForAPI(t, base)

	// Wait until validation has marked the live backend healthy so that
	// health-dependent fields are populated.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/api/v1/overwatch/backends?status=healthy")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(b), `"validation_healthy":true`) {
				return base, backendPort
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("backend was never validated healthy")
	return "", 0
}

type contractHealthProvider struct{ start time.Time }

func (p contractHealthProvider) GetVersion() string      { return "contract" }
func (p contractHealthProvider) GetStartTime() time.Time { return p.start }
func (p contractHealthProvider) IsHealthy() bool         { return true }

// startUnwiredHandlers serves the handlers that pkg/api/server.go can register
// but cmd/opengslb never attaches (x-status: unwired), so their documented
// success responses are still validated.
func startUnwiredHandlers(t *testing.T) string {
	t.Helper()
	port := contractFreePort(t)
	srv, err := api.NewServer(api.ServerConfig{
		Address: fmt.Sprintf("127.0.0.1:%d", port),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, api.NewHandlers(nil, nil, nil, nil))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv.SetSimpleHealthHandlers(api.NewSimpleHealthHandlers(contractHealthProvider{start: time.Now()}))
	srv.SetDNSSECHandlers(api.NewDNSSECHandlers(api.DNSSECHandlersConfig{Enabled: false}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForAPI(t, base)
	return base
}

func TestAPIContract(t *testing.T) {
	spec, err := apicontract.Load(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	base, bp := startContractOverwatch(t)
	unwired := startUnwiredHandlers(t)

	const app = "app.gslb.example.com"
	const dom = "contract.gslb.example.com"
	srvID := dom + ":127.0.0.6:9001"
	staticID := fmt.Sprintf("%s:127.0.0.1:%d", app, bp)
	override := fmt.Sprintf("/api/v1/overwatch/backends/%s/127.0.0.1/%d/override", app, bp)
	extOverride := fmt.Sprintf("/api/v1/overrides/%s/127.0.0.1:%d", app, bp)

	// Order matters: later calls depend on resources created by earlier ones.
	calls := []contractCall{
		// Discovery and probes
		{"getApiRoot", "GET", "/api", "", 200},
		{"getApiRoot", "POST", "/api", "", 405},
		{"getV1Root", "GET", "/api/v1", "", 200},
		{"getV1Root", "GET", "/api/v1/does-not-exist", "", 404},
		{"getHealthRoot", "GET", "/api/v1/health", "", 200},
		{"getGeoRoot", "GET", "/api/v1/geo", "", 200},
		{"getOverwatchRoot", "GET", "/api/v1/overwatch", "", 200},
		{"getClusterRoot", "GET", "/api/v1/cluster", "", 200},
		{"getVersion", "GET", "/api/v1/version", "", 200},
		{"getSimpleHealth", "GET", "/api/health", "", 404},
		{"getLive", "GET", "/api/v1/live", "", 200},
		{"getLive", "POST", "/api/v1/live", "", 405},
		{"getReady", "GET", "/api/v1/ready", "", 200},

		// Health
		{"getHealthServers", "GET", "/api/v1/health/servers", "", 200},
		{"getHealthRegions", "GET", "/api/v1/health/regions", "", 200},

		// Overwatch
		{"listOverwatchBackends", "GET", "/api/v1/overwatch/backends", "", 200},
		{"listOverwatchBackends", "GET", "/api/v1/overwatch/backends?service=" + app + "&region=us-east&status=healthy", "", 200},
		{"getOverwatchStats", "GET", "/api/v1/overwatch/stats", "", 200},
		{"triggerValidation", "POST", "/api/v1/overwatch/validate", "", 200},
		{"triggerValidation", "POST", "/api/v1/overwatch/validate?service=nope&address=192.0.2.9&port=1", "", 404},
		{"triggerValidation", "POST", "/api/v1/overwatch/validate?service=nope&address=192.0.2.9&port=x", "", 400},
		{"getClusterStatus", "GET", "/api/v1/cluster/status", "", 200},
		{"getClusterStatus", "GET", "/api/v1/cluster/status?expected_agents=3", "", 200},
		{"getLatencyTable", "GET", "/api/v1/overwatch/latency", "", 200},
		{"injectLatency", "POST", "/api/v1/overwatch/latency", `{"subnet":"10.9.0.0/24","backend":"app.gslb.example.com","region":"us-east","latency_ms":20,"samples":5}`, 201},
		{"injectLatency", "POST", "/api/v1/overwatch/latency", `{"subnet":"bad","backend":"a","region":"b"}`, 400},
		{"getLatencyTable", "GET", "/api/v1/overwatch/latency", "", 200},
		{"setBackendOverride", "POST", override, `{"healthy":false,"reason":"maintenance"}`, 200},
		{"listOverwatchBackends", "GET", "/api/v1/overwatch/backends", "", 200},
		{"clearBackendOverride", "DELETE", override, "", 200},
		{"setBackendOverride", "POST", "/api/v1/overwatch/backends/nope/192.0.2.9/1/override", `{"healthy":false}`, 404},
		{"setBackendOverride", "POST", "/api/v1/overwatch/backends/nope/192.0.2.9/x/override", `{"healthy":false}`, 400},
		{"clearBackendOverride", "DELETE", "/api/v1/overwatch/backends/nope/192.0.2.9/1/override", "", 404},
		{"listAgentCerts", "GET", "/api/v1/overwatch/agents", "", 503},
		{"listExpiringAgentCerts", "GET", "/api/v1/overwatch/agents/expiring", "", 503},
		{"getAgentCert", "GET", "/api/v1/overwatch/agents/agent-1", "", 503},
		{"deleteAgentCert", "DELETE", "/api/v1/overwatch/agents/agent-1", "", 503},
		{"revokeAgentCert", "POST", "/api/v1/overwatch/agents/agent-1/revoke", `{"reason":"test"}`, 503},

		// External overrides
		{"listOverrides", "GET", "/api/v1/overrides", "", 200},
		{"setOverride", "PUT", extOverride, `{"healthy":false,"reason":"drain","source":"contract-test"}`, 200},
		{"setOverride", "PUT", extOverride, `{"healthy":false}`, 400},
		{"listOverrides", "GET", "/api/v1/overrides", "", 200},
		{"getOverride", "GET", extOverride, "", 200},
		{"getOverride", "GET", "/api/v1/overrides/a/b", "", 404},
		{"getOverride", "GET", "/api/v1/overrides/a", "", 400},
		{"clearOverride", "DELETE", extOverride, "", 204},
		{"clearOverride", "DELETE", extOverride, "", 404},

		// Geolocation
		{"listGeoMappings", "GET", "/api/v1/geo/mappings", "", 200},
		{"addGeoMapping", "PUT", "/api/v1/geo/mappings", `{"cidr":"172.16.0.0/12","region":"eu-west","comment":"c"}`, 201},
		{"addGeoMapping", "PUT", "/api/v1/geo/mappings", `{"cidr":"bad","region":"eu-west"}`, 400},
		{"addGeoMappingPost", "POST", "/api/v1/geo/mappings", `{"cidr":"192.168.0.0/16","region":"eu-west"}`, 201},
		{"listGeoMappings", "GET", "/api/v1/geo/mappings", "", 200},
		{"deleteGeoMapping", "DELETE", "/api/v1/geo/mappings/172.16.0.0%2F12", "", 204},
		{"deleteGeoMapping", "DELETE", "/api/v1/geo/mappings/172.16.0.0%2F12", "", 404},
		{"testGeoIp", "GET", "/api/v1/geo/test?ip=192.0.2.1", "", 200},
		{"testGeoIp", "GET", "/api/v1/geo/test?ip=10.1.1.1", "", 200},
		{"testGeoIp", "GET", "/api/v1/geo/test?ip=203.0.113.9", "", 200},
		{"testGeoIp", "GET", "/api/v1/geo/test", "", 400},

		// Domains
		{"listDomains", "GET", "/api/v1/domains", "", 200},
		{"getDomain", "GET", "/api/v1/domains/" + app, "", 200},
		{"getDomain", "GET", "/api/v1/domains/nope.example.com", "", 404},
		{"createDomain", "POST", "/api/v1/domains", `{"name":"contract.gslb.example.com","ttl":20,"routing_algorithm":"weighted","enabled":true,"description":"d","tags":["a"],"settings":{"geo_routing_enabled":false,"failover_enabled":true,"failover_threshold":2,"load_balancing_method":"x"}}`, 201},
		{"createDomain", "POST", "/api/v1/domains", `{"ttl":20}`, 400},
		{"createDomain", "POST", "/api/v1/domains", `{"name":"bad.gslb.example.com","routing_algorithm":"fastest"}`, 400},
		{"createDomain", "POST", "/api/v1/domains", `{"name":"bad.gslb.example.com","routing_algorithm":"weighted","routing_policy":"failover"}`, 400},
		{"createDomain", "POST", "/api/v1/domains", `{"name":"contract.gslb.example.com"}`, 409},
		{"createDomain", "POST", "/api/v1/domains", `{"name":"` + app + `"}`, 409},
		{"getDomain", "GET", "/api/v1/domains/" + dom, "", 200},
		{"listDomains", "GET", "/api/v1/domains", "", 200},
		{"updateDomain", "PUT", "/api/v1/domains/" + dom, `{"ttl":45,"routing_algorithm":"failover"}`, 200},
		// routing_policy is the deprecated alias of routing_algorithm.
		{"patchDomain", "PATCH", "/api/v1/domains/" + dom, `{"description":"patched","routing_policy":"round-robin"}`, 200},
		{"patchDomain", "PATCH", "/api/v1/domains/" + dom, `{"routing_algorithm":"fastest"}`, 400},
		{"updateDomain", "PUT", "/api/v1/domains/nope.example.com", `{"ttl":45}`, 404},
		{"updateDomain", "PUT", "/api/v1/domains/" + app, `{"ttl":45}`, 409},
		// A domain without backends is an empty list, not 404 (M4).
		{"listDomainBackends", "GET", "/api/v1/domains/" + dom + "/backends", "", 200},
		{"listDomainBackends", "GET", "/api/v1/domains/nope.example.com/backends", "", 404},
		{"addDomainBackend", "POST", "/api/v1/domains/" + dom + "/backends", `{"address":"127.0.0.3","port":8080,"weight":10,"priority":1,"region":"us-east","enabled":true,"health_check":{"type":"http","path":"/healthz","host":"app.internal","interval":"30s","timeout":"5s","healthy_threshold":2,"unhealthy_threshold":3}}`, 201},
		{"addDomainBackend", "POST", "/api/v1/domains/" + dom + "/backends", `{"address":"2001:db8::3","port":8080,"enabled":false,"health_check":{"enabled":false}}`, 201},
		{"addDomainBackend", "POST", "/api/v1/domains/" + dom + "/backends", `{"port":8080}`, 400},
		// Durations are strings; a number is rejected (M8).
		{"addDomainBackend", "POST", "/api/v1/domains/" + dom + "/backends", `{"address":"127.0.0.4","port":8080,"health_check":{"type":"tcp","interval":30,"timeout":5}}`, 400},
		{"addDomainBackend", "POST", "/api/v1/domains/" + dom + "/backends", `{"address":"127.0.0.3","port":8080}`, 409},
		{"addDomainBackend", "POST", "/api/v1/domains/nope.example.com/backends", `{"address":"127.0.0.3","port":8080}`, 404},
		{"listDomainBackends", "GET", "/api/v1/domains/" + dom + "/backends", "", 200},
		{"listDomainBackends", "GET", "/api/v1/domains/" + app + "/backends", "", 200},
		// A backend is a server (M15).
		{"getServer", "GET", "/api/v1/servers/" + dom + ":127.0.0.3:8080", "", 200},
		{"getServer", "GET", "/api/v1/servers/" + dom + ":2001:db8::3:8080", "", 200},
		{"updateServer", "PUT", "/api/v1/servers/" + dom + ":127.0.0.3:8080", `{"weight":20,"enabled":true,"health_check":{"interval":"10s"}}`, 200},
		// The delete is reachable (M7).
		{"removeDomainBackend", "DELETE", "/api/v1/domains/" + dom + "/backends/" + dom + ":127.0.0.3:8080", "", 204},
		{"removeDomainBackend", "DELETE", "/api/v1/domains/" + dom + "/backends/" + dom + ":127.0.0.3:8080", "", 404},
		{"removeDomainBackend", "DELETE", "/api/v1/domains/" + dom + "/backends/" + staticID, "", 404},
		{"removeDomainBackend", "DELETE", "/api/v1/domains/" + app + "/backends/" + staticID, "", 409},
		{"removeDomainBackend", "GET", "/api/v1/domains/" + dom + "/backends/" + dom + ":2001:db8::3:8080", "", 405},

		// Servers
		{"listServers", "GET", "/api/v1/servers", "", 200},
		{"listServers", "GET", "/api/v1/servers?region=us-east&enabled=true&healthy=true", "", 200},
		{"listServers", "GET", "/api/v1/servers?enabled=false", "", 200},
		{"createServer", "POST", "/api/v1/servers", `{"service":"contract.gslb.example.com","address":"127.0.0.6","port":9001,"weight":100,"region":"us-east","enabled":true,"health_check":{"type":"tcp","interval":"15s","timeout":"3s"}}`, 201},
		{"createServer", "POST", "/api/v1/servers", `{"port":9000}`, 400},
		{"createServer", "POST", "/api/v1/servers", `{"address":"127.0.0.7","port":9002}`, 400},
		// The deprecated ways to name the domain: metadata.service and name.
		{"createServer", "POST", "/api/v1/servers", `{"address":"127.0.0.7","port":9002,"metadata":{"service":"contract.gslb.example.com"}}`, 201},
		{"createServer", "POST", "/api/v1/servers", `{"address":"127.0.0.8","port":9003,"name":"contract.gslb.example.com"}`, 201},
		{"createServer", "POST", "/api/v1/servers", `{"service":"contract.gslb.example.com","address":"127.0.0.6","port":9001}`, 409},
		{"createServer", "POST", "/api/v1/servers", `{"service":"nope.example.com","address":"127.0.0.6","port":9001}`, 404},
		{"listServers", "GET", "/api/v1/servers", "", 200},
		{"getServer", "GET", "/api/v1/servers/" + srvID, "", 200},
		{"getServer", "GET", "/api/v1/servers/" + staticID, "", 200},
		{"getServer", "GET", "/api/v1/servers/nope", "", 404},
		{"updateServer", "PUT", "/api/v1/servers/" + srvID, `{"weight":7,"region":"eu-west"}`, 200},
		{"patchServer", "PATCH", "/api/v1/servers/" + srvID, `{"weight":8,"enabled":false}`, 200},
		{"updateServer", "PUT", "/api/v1/servers/" + srvID, `{"address":"127.0.0.99"}`, 400},
		{"updateServer", "PUT", "/api/v1/servers/" + srvID, `{"health_check":{"interval":30}}`, 400},
		{"updateServer", "PUT", "/api/v1/servers/nope", `{"weight":7}`, 404},
		{"updateServer", "PUT", "/api/v1/servers/" + staticID, `{"weight":7}`, 409},
		{"getServerHealthCheck", "GET", "/api/v1/servers/" + srvID + "/health-check", "", 200},
		{"getServerHealthCheck", "GET", "/api/v1/servers/" + staticID + "/health-check", "", 200},
		{"getServerHealthCheck", "GET", "/api/v1/servers/nope/health-check", "", 404},
		{"updateServerHealthCheck", "PUT", "/api/v1/servers/" + srvID + "/health-check", `{"enabled":true,"type":"https","path":"/ready","host":"app.internal","interval":"30s","timeout":"5s","healthy_threshold":2,"unhealthy_threshold":3}`, 200},
		{"patchServerHealthCheck", "PATCH", "/api/v1/servers/" + srvID + "/health-check", `{"timeout":"1500ms"}`, 200},
		// Integer nanoseconds, the wire format up to 1.1.10, are rejected (M8).
		{"updateServerHealthCheck", "PUT", "/api/v1/servers/" + srvID + "/health-check", `{"enabled":true,"type":"tcp","interval":30000000000,"timeout":5000000000}`, 400},
		{"patchServerHealthCheck", "PATCH", "/api/v1/servers/" + srvID + "/health-check", `{"expected_status":204}`, 400},
		{"updateServerHealthCheck", "PUT", "/api/v1/servers/nope/health-check", `{"type":"tcp"}`, 404},
		{"updateServerHealthCheck", "PUT", "/api/v1/servers/" + staticID + "/health-check", `{"type":"tcp"}`, 409},
		{"deleteServer", "DELETE", "/api/v1/servers/" + srvID, "", 204},
		{"deleteServer", "DELETE", "/api/v1/servers/" + srvID, "", 404},
		{"deleteServer", "DELETE", "/api/v1/servers/nope", "", 404},
		{"deleteServer", "DELETE", "/api/v1/servers/" + staticID, "", 409},

		// Regions
		{"listRegions", "GET", "/api/v1/regions", "", 200},
		{"listRegions", "GET", "/api/v1/regions?enabled=true&continent=EU", "", 200},
		{"getRegion", "GET", "/api/v1/regions/us-east", "", 200},
		{"getRegion", "GET", "/api/v1/regions/nope", "", 404},
		{"createRegion", "POST", "/api/v1/regions", `{"name":"ap-south","code":"ap-south","description":"d","latitude":1.5,"longitude":2.5,"continent":"AS","countries":["IN"],"enabled":true,"priority":3}`, 201},
		{"createRegion", "POST", "/api/v1/regions", `{"name":"x"}`, 400},
		{"getRegion", "GET", "/api/v1/regions/ap-south", "", 200},
		{"updateRegion", "PUT", "/api/v1/regions/ap-south", `{"description":"changed","priority":4}`, 200},
		{"patchRegion", "PATCH", "/api/v1/regions/ap-south", `{"priority":5}`, 200},
		{"updateRegion", "PUT", "/api/v1/regions/us-east", `{"description":"changed"}`, 500},
		{"updateRegion", "PUT", "/api/v1/regions/nope", `{"description":"changed"}`, 404},
		{"deleteRegion", "DELETE", "/api/v1/regions/ap-south", "", 204},
		{"deleteRegion", "DELETE", "/api/v1/regions/us-east", "", 500},
		{"deleteRegion", "DELETE", "/api/v1/regions/nope", "", 404},

		// Domain deletion (after the server and backend calls that need the domain)
		{"deleteDomain", "DELETE", "/api/v1/domains/" + dom, "", 204},
		{"deleteDomain", "DELETE", "/api/v1/domains/nope.example.com", "", 404},
		{"deleteDomain", "DELETE", "/api/v1/domains/" + app, "", 409},
		// The servers created for the domain went with it.
		{"getServer", "GET", "/api/v1/servers/" + dom + ":127.0.0.7:9002", "", 404},

		// Nodes (stub provider)
		{"listOverwatchNodes", "GET", "/api/v1/nodes/overwatch", "", 200},
		{"getOverwatchNode", "GET", "/api/v1/nodes/overwatch/local", "", 200},
		{"getOverwatchNode", "GET", "/api/v1/nodes/overwatch/contract-overwatch", "", 404},
		{"listAgentNodes", "GET", "/api/v1/nodes/agent", "", 200},
		{"getAgentNode", "GET", "/api/v1/nodes/agent/nope", "", 404},
		{"registerAgentNode", "POST", "/api/v1/nodes/agent", `{"name":"a1","address":"10.0.0.9","region":"us-east"}`, 500},
		{"registerAgentNode", "POST", "/api/v1/nodes/agent", `{"name":"a1"}`, 400},
		{"deregisterAgentNode", "DELETE", "/api/v1/nodes/agent/a1", "", 500},
		{"getAgentCertificate", "GET", "/api/v1/nodes/agent/a1/certificate", "", 404},
		{"reissueAgentCertificate", "POST", "/api/v1/nodes/agent/a1/certificate", "", 500},
		{"revokeAgentCertificate", "DELETE", "/api/v1/nodes/agent/a1/certificate", "", 500},

		// Gossip (stub provider)
		{"listGossipNodes", "GET", "/api/v1/gossip/nodes", "", 200},
		{"getGossipNode", "GET", "/api/v1/gossip/nodes/local", "", 200},
		{"getGossipNode", "GET", "/api/v1/gossip/nodes/nope", "", 404},
		{"getGossipConfig", "GET", "/api/v1/gossip/config", "", 200},
		{"updateGossipConfig", "PUT", "/api/v1/gossip/config", `{"enabled":true}`, 500},
		{"patchGossipConfig", "PATCH", "/api/v1/gossip/config", `{"enabled":true}`, 500},
		{"generateGossipKey", "POST", "/api/v1/gossip/generate-key", "", 200},

		// Audit log (stub provider)
		{"listAuditLogs", "GET", "/api/v1/audit-logs", "", 200},
		{"listAuditLogs", "GET", "/api/v1/audit-logs?limit=5&offset=0&sort_by=timestamp&sort_order=desc", "", 200},
		{"getAuditStats", "GET", "/api/v1/audit-logs/stats", "", 200},
		{"exportAuditLogs", "GET", "/api/v1/audit-logs/export", "", 200},
		{"exportAuditLogs", "GET", "/api/v1/audit-logs/export?format=csv", "", 200},
		{"getAuditEntry", "GET", "/api/v1/audit-logs/some-id", "", 404},

		// Metrics
		{"getMetricsRoot", "GET", "/api/v1/metrics", "", 200},
		{"getMetricsOverview", "GET", "/api/v1/metrics/overview", "", 200},
		{"getMetricsHistory", "GET", "/api/v1/metrics/history?duration=1h&resolution=5m", "", 200},
		{"getNodeMetrics", "GET", "/api/v1/metrics/nodes/contract-overwatch", "", 200},
		{"getRegionMetrics", "GET", "/api/v1/metrics/regions/us-east", "", 200},
		{"getRoutingStats", "GET", "/api/v1/metrics/routing", "", 200},

		// Config and preferences
		{"getPreferences", "GET", "/api/v1/preferences", "", 200},
		{"updatePreferences", "PUT", "/api/v1/preferences", `{"theme":"dark"}`, 500},
		{"patchPreferences", "PATCH", "/api/v1/preferences", `{"theme":"dark"}`, 500},
		{"getConfigRoot", "GET", "/api/v1/config", "", 200},
		{"getSystemConfig", "GET", "/api/v1/config/system", "", 200},
		{"getDnsConfig", "GET", "/api/v1/config/dns", "", 200},
		{"updateDnsConfig", "PUT", "/api/v1/config/dns", `{"default_ttl":10}`, 500},
		{"patchDnsConfig", "PATCH", "/api/v1/config/dns", `{"default_ttl":10}`, 500},
		{"getHealthCheckConfig", "GET", "/api/v1/config/health-check", "", 200},
		{"updateHealthCheckConfig", "PUT", "/api/v1/config/health-check", `{}`, 500},
		{"patchHealthCheckConfig", "PATCH", "/api/v1/config/health-check", `{}`, 500},
		{"getLoggingConfig", "GET", "/api/v1/config/logging", "", 200},
		{"updateLoggingConfig", "PUT", "/api/v1/config/logging", `{"level":"debug"}`, 500},
		{"patchLoggingConfig", "PATCH", "/api/v1/config/logging", `{"level":"debug"}`, 500},

		// Routing (stub provider)
		{"listRoutingAlgorithms", "GET", "/api/v1/routing/algorithms", "", 200},
		{"getRoutingAlgorithm", "GET", "/api/v1/routing/algorithms/round-robin", "", 200},
		{"getRoutingAlgorithm", "GET", "/api/v1/routing/algorithms/nope", "", 404},
		{"testRouting", "POST", "/api/v1/routing/test", `{"domain":"app.gslb.example.com","client_ip":"192.0.2.1"}`, 200},
		{"testRouting", "POST", "/api/v1/routing/test", `{"domain":"app.gslb.example.com"}`, 400},
		{"listRoutingDecisions", "GET", "/api/v1/routing/decisions", "", 200},
		{"listRoutingFlows", "GET", "/api/v1/routing/flows", "", 200},

		// DNSSEC: not attached by the binary, so the real overwatch answers 404.
		{"getDnssecRoot", "GET", "/api/v1/dnssec", "", 404},
		{"getDnssecStatus", "GET", "/api/v1/dnssec/status", "", 404},
		{"getDnssecDs", "GET", "/api/v1/dnssec/ds", "", 404},
		{"getDnssecKeys", "GET", "/api/v1/dnssec/keys", "", 404},
		{"triggerDnssecSync", "POST", "/api/v1/dnssec/sync", "", 405},
	}

	// The same unwired handlers, attached to a bare API server.
	unwiredCalls := []contractCall{
		{"getSimpleHealth", "GET", "/api/health", "", 200},
		{"getDnssecRoot", "GET", "/api/v1/dnssec", "", 200},
		{"getDnssecStatus", "GET", "/api/v1/dnssec/status", "", 200},
		{"getDnssecDs", "GET", "/api/v1/dnssec/ds", "", 200},
		{"getDnssecKeys", "GET", "/api/v1/dnssec/keys", "", 200},
		{"triggerDnssecSync", "POST", "/api/v1/dnssec/sync", "", 400},
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	called := map[string]bool{}
	success := map[string]bool{}

	run := func(baseURL string, c contractCall) {
		t.Helper()
		if _, ok := spec.Operation(c.op); !ok {
			t.Errorf("%s %s: operation %q is not in the spec", c.method, c.path, c.op)
			return
		}
		var body io.Reader
		if c.body != "" {
			body = bytes.NewBufferString(c.body)
		}
		req, err := http.NewRequest(c.method, baseURL+c.path, body)
		if err != nil {
			t.Errorf("%s %s: %v", c.method, c.path, err)
			return
		}
		if c.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("%s %s: %v", c.method, c.path, err)
			return
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		called[c.op] = true
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			success[c.op] = true
		}
		if resp.StatusCode != c.want {
			t.Errorf("%s %s (%s): status %d, want %d; body: %s", c.method, c.path, c.op, resp.StatusCode, c.want, respBody)
		}
		if err := spec.ValidateResponse(c.op, resp.StatusCode, resp.Header.Get("Content-Type"), respBody); err != nil {
			t.Errorf("%s %s (%s): %v", c.method, c.path, c.op, err)
		}
	}

	for _, c := range calls {
		run(base, c)
	}
	for _, c := range unwiredCalls {
		run(unwired, c)
	}

	// Operations whose success response needs state this harness cannot
	// create. Each entry states why.
	noSuccessInHarness := map[string]string{
		"getAgentNode":      "needs a live gossip agent; static backends have an empty agent id",
		"triggerDnssecSync": "needs DNSSEC enabled with a key syncer, which the binary never constructs",
	}

	// Every operation in the spec must have been called, and every operation
	// that is expected to work must have produced a success response.
	for _, op := range spec.Operations() {
		if !called[op.ID] {
			t.Errorf("spec operation %s (%s %s) was never called by the contract test", op.ID, op.Method, op.Path)
			continue
		}
		switch op.Status {
		case "", "stub", "unwired":
			if _, exempt := noSuccessInHarness[op.ID]; !success[op.ID] && !exempt {
				t.Errorf("spec operation %s (%s %s) never returned a success response; if it cannot succeed, mark it x-status: not-implemented, unavailable or broken", op.ID, op.Method, op.Path)
			}
		case "not-implemented", "unavailable", "broken":
			if success[op.ID] {
				t.Errorf("spec operation %s is marked x-status: %s but returned a success response; update the spec", op.ID, op.Status)
			}
		default:
			t.Errorf("spec operation %s has unknown x-status %q", op.ID, op.Status)
		}
	}
}

// TestAPIContractACL pins the one non-JSON error the API produces: a client
// outside api.allowed_networks gets a plain-text 403.
func TestAPIContractACL(t *testing.T) {
	spec, err := apicontract.Load(specPath)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	port := contractFreePort(t)
	srv, err := api.NewServer(api.ServerConfig{
		Address:         fmt.Sprintf("127.0.0.1:%d", port),
		AllowedNetworks: []string{"192.0.2.0/24"},
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, api.NewHandlers(nil, nil, nil, nil))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Start(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForAPI(t, base)

	resp, err := http.Get(base + "/api/v1/health/servers")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
	if err := spec.ValidateResponse("getHealthServers", resp.StatusCode, resp.Header.Get("Content-Type"), body); err != nil {
		t.Error(err)
	}
}
