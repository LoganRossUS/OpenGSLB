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

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/health"
)

// Mock implementations

type mockHealthProvider struct {
	snapshots []health.Snapshot
	count     int
}

func (m *mockHealthProvider) GetAllStatus() []health.Snapshot {
	return m.snapshots
}

func (m *mockHealthProvider) ServerCount() int {
	return m.count
}

type mockReadinessChecker struct {
	dnsReady    bool
	healthReady bool
}

func (m *mockReadinessChecker) IsDNSReady() bool {
	return m.dnsReady
}

func (m *mockReadinessChecker) IsHealthCheckReady() bool {
	return m.healthReady
}

func TestACLMiddleware_AllowedIP(t *testing.T) {
	mw, err := NewACLMiddleware([]string{"192.168.1.0/24"}, false, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestACLMiddleware_DeniedIP(t *testing.T) {
	mw, err := NewACLMiddleware([]string{"192.168.1.0/24"}, false, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestACLMiddleware_XForwardedFor(t *testing.T) {
	mw, err := NewACLMiddleware([]string{"192.168.1.0/24"}, true, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "192.168.1.50, 10.0.0.1")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 with trusted X-Forwarded-For, got %d", rr.Code)
	}
}

func TestACLMiddleware_XForwardedForNotTrusted(t *testing.T) {
	mw, err := NewACLMiddleware([]string{"192.168.1.0/24"}, false, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "192.168.1.50")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403 when proxy headers not trusted, got %d", rr.Code)
	}
}

func TestACLMiddleware_SingleIP(t *testing.T) {
	mw, err := NewACLMiddleware([]string{"127.0.0.1"}, false, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestACLMiddleware_EmptyNetworksDenyAll(t *testing.T) {
	mw, err := NewACLMiddleware([]string{}, false, nil)
	if err != nil {
		t.Fatalf("failed to create middleware: %v", err)
	}

	handler := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403 when no networks configured (fail-closed), got %d", rr.Code)
	}
}

// Handler Tests

func TestHealthServers_Success(t *testing.T) {
	now := time.Now()
	hp := &mockHealthProvider{
		snapshots: []health.Snapshot{
			{
				Address:           "10.0.1.10:80",
				Status:            health.StatusHealthy,
				LastCheck:         now,
				LastHealthy:       now,
				ConsecutiveFails:  0,
				ConsecutivePasses: 5,
				LastError:         nil,
			},
			{
				Address:           "10.0.1.11:80",
				Status:            health.StatusUnhealthy,
				LastCheck:         now,
				ConsecutiveFails:  3,
				ConsecutivePasses: 0,
				LastError:         errors.New("connection refused"),
			},
		},
		count: 2,
	}

	rc := &mockReadinessChecker{dnsReady: true, healthReady: true}
	handlers := NewHandlers(hp, rc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health/servers", nil)
	rr := httptest.NewRecorder()
	handlers.HealthServers(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(resp.Servers))
	}

	if !resp.Servers[0].Healthy {
		t.Error("expected first server to be healthy")
	}
	if resp.Servers[0].Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp.Servers[0].Status)
	}

	if resp.Servers[1].Healthy {
		t.Error("expected second server to be unhealthy")
	}
	if resp.Servers[1].LastError != "connection refused" {
		t.Errorf("expected 'connection refused' error, got '%s'", resp.Servers[1].LastError)
	}
}

func TestHealthServers_MethodNotAllowed(t *testing.T) {
	hp := &mockHealthProvider{}
	rc := &mockReadinessChecker{}
	handlers := NewHandlers(hp, rc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/health/servers", nil)
	rr := httptest.NewRecorder()
	handlers.HealthServers(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr.Code)
	}
}

func TestReady_AllReady(t *testing.T) {
	hp := &mockHealthProvider{}
	rc := &mockReadinessChecker{dnsReady: true, healthReady: true}
	handlers := NewHandlers(hp, rc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil)
	rr := httptest.NewRecorder()
	handlers.Ready(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp ReadyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Ready {
		t.Error("expected ready=true")
	}
	if !resp.DNSReady {
		t.Error("expected dns_ready=true")
	}
	if !resp.HealthReady {
		t.Error("expected health_ready=true")
	}
}

func TestReady_NotReady(t *testing.T) {
	hp := &mockHealthProvider{}
	rc := &mockReadinessChecker{dnsReady: true, healthReady: false}
	handlers := NewHandlers(hp, rc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil)
	rr := httptest.NewRecorder()
	handlers.Ready(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	var resp ReadyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Ready {
		t.Error("expected ready=false")
	}
	if resp.Message == "" {
		t.Error("expected message to be populated when not ready")
	}
}

func TestLive_Success(t *testing.T) {
	hp := &mockHealthProvider{}
	rc := &mockReadinessChecker{}
	handlers := NewHandlers(hp, rc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/live", nil)
	rr := httptest.NewRecorder()
	handlers.Live(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp LiveResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Alive {
		t.Error("expected alive=true")
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input    string
		wantAddr string
		wantPort int
	}{
		{"10.0.1.10:80", "10.0.1.10", 80},
		{"192.168.1.1:8080", "192.168.1.1", 8080},
		{"10.0.1.10", "10.0.1.10", 0},
		{"invalid", "invalid", 0},
	}

	for _, tt := range tests {
		addr, port := parseAddress(tt.input)
		if addr != tt.wantAddr {
			t.Errorf("parseAddress(%s) addr = %s, want %s", tt.input, addr, tt.wantAddr)
		}
		if port != tt.wantPort {
			t.Errorf("parseAddress(%s) port = %d, want %d", tt.input, port, tt.wantPort)
		}
	}
}
