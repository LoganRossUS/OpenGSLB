// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// mockDataProvider implements DataProvider for testing.
type mockDataProvider struct {
	config   *config.Config
	registry *overwatch.Registry
}

func newMockDataProvider() *mockDataProvider {
	return &mockDataProvider{
		config: &config.Config{
			DNS: config.DNSConfig{
				ListenAddress: ":53",
				DefaultTTL:    300,
			},
			API: config.APIConfig{
				Enabled: true,
				Address: ":8080",
			},
			Domains: []config.Domain{
				{
					Name:             "api.example.com",
					RoutingAlgorithm: "latency",
					Regions:          []string{"us-east-1", "eu-west-1"},
					TTL:              60,
				},
				{
					Name:             "web.example.com",
					RoutingAlgorithm: "geolocation",
					Regions:          []string{"us-east-1"},
					TTL:              120,
				},
			},
			Regions: []config.Region{
				{
					Name: "us-east-1",
					Servers: []config.Server{
						{Address: "10.0.1.10", Port: 80, Weight: 100},
						{Address: "10.0.1.11", Port: 80, Weight: 100},
					},
					HealthCheck: config.HealthCheck{
						Type:     "http",
						Path:     "/health",
						Interval: 30 * time.Second,
						Timeout:  5 * time.Second,
					},
				},
				{
					Name: "eu-west-1",
					Servers: []config.Server{
						{Address: "10.0.2.10", Port: 80, Weight: 100},
					},
					HealthCheck: config.HealthCheck{
						Type:     "http",
						Path:     "/health",
						Interval: 30 * time.Second,
						Timeout:  5 * time.Second,
					},
				},
			},
			Overwatch: config.OverwatchConfig{
				Identity: config.OverwatchIdentityConfig{
					NodeID: "test-node-1",
					Region: "us-east-1",
				},
				Validation: config.ValidationConfig{
					Enabled:       true,
					CheckInterval: 30 * time.Second,
					CheckTimeout:  5 * time.Second,
				},
				Stale: config.StaleConfig{
					Threshold:   30 * time.Second,
					RemoveAfter: 5 * time.Minute,
				},
				Geolocation: config.GeolocationConfig{
					DefaultRegion: "us-east-1",
					ECSEnabled:    true,
				},
				Gossip: config.OverwatchGossipConfig{
					BindAddress:    "0.0.0.0",
					ProbeInterval:  1 * time.Second,
					ProbeTimeout:   500 * time.Millisecond,
					GossipInterval: 200 * time.Millisecond,
				},
			},
		},
	}
}

func (m *mockDataProvider) GetConfig() *config.Config {
	return m.config
}

func (m *mockDataProvider) UpdateConfig(cfg *config.Config) error {
	m.config = cfg
	return nil
}

func (m *mockDataProvider) GetBackendRegistry() *overwatch.Registry {
	return m.registry
}

func (m *mockDataProvider) GetValidator() *overwatch.Validator {
	return nil
}

// resetGlobalState clears all shared state between tests to prevent race conditions.
func resetGlobalState() {
	// Reset validation jobs
	validationJobsMu.Lock()
	validationJobs = make(map[string]*ValidationJob)
	validationJobsMu.Unlock()

	// Reset override store
	overrideStoreMu.Lock()
	overrideStore = make(map[string]Override)
	overrideStoreMu.Unlock()

	// Reset gossip node store
	gossipNodeStoreMu.Lock()
	gossipNodeStore = make(map[string]GossipNode)
	gossipNodeStoreMu.Unlock()

	// Reset geo mapping store
	geoMappingStoreMu.Lock()
	geoMappingStore = make(map[string]GeoMapping)
	geoMappingStoreMu.Unlock()

	// Reset DNSSEC key store
	dnssecKeyStoreMu.Lock()
	dnssecKeyStore = make(map[string]DNSKey)
	dnssecKeyStoreMu.Unlock()
}

// testHandlers creates a Handlers instance with mock dependencies for testing.
func testHandlers() *Handlers {
	resetGlobalState()
	return &Handlers{
		dataProvider: newMockDataProvider(),
		auditLogger:  NewAuditLogger(nil),
	}
}

// makeRequest is a helper to create and execute HTTP requests for testing.
func makeRequest(t *testing.T, handler http.HandlerFunc, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// assertStatus checks that the response has the expected status code.
func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rr.Code != expected {
		t.Errorf("expected status %d, got %d. Body: %s", expected, rr.Code, rr.Body.String())
	}
}

// assertJSONField checks that a JSON response contains a specific field.
func assertJSONField(t *testing.T, rr *httptest.ResponseRecorder, field string) {
	t.Helper()
	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if _, exists := result[field]; !exists {
		t.Errorf("expected field '%s' in response, got: %v", field, result)
	}
}

// decodeJSON decodes the response body into the provided interface.
func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode JSON: %v. Body: %s", err, rr.Body.String())
	}
}
