// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Address != ":3001" {
		t.Errorf("expected default address :3001, got %s", cfg.Address)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "*" {
		t.Errorf("expected default allowed origins [*], got %v", cfg.AllowedOrigins)
	}
}

func TestNewServer(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := newMockDataProvider()

	server, err := NewServer(cfg, provider)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestServerCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		origin         string
		allowedOrigins []string
		expectedStatus int
		expectCORS     bool
	}{
		{
			name:           "preflight request allowed",
			method:         http.MethodOptions,
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"http://localhost:3000"},
			expectedStatus: http.StatusNoContent,
			expectCORS:     true,
		},
		{
			name:           "preflight request wildcard",
			method:         http.MethodOptions,
			origin:         "http://example.com",
			allowedOrigins: []string{"*"},
			expectedStatus: http.StatusNoContent,
			expectCORS:     true,
		},
		{
			name:           "regular request with CORS",
			method:         http.MethodGet,
			origin:         "http://localhost:3000",
			allowedOrigins: []string{"http://localhost:3000"},
			expectedStatus: http.StatusOK,
			expectCORS:     true,
		},
		{
			name:           "request without origin uses wildcard",
			method:         http.MethodGet,
			origin:         "",
			allowedOrigins: []string{"*"},
			expectedStatus: http.StatusOK,
			expectCORS:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultServerConfig()
			cfg.AllowedOrigins = tt.allowedOrigins
			provider := newMockDataProvider()

			server, _ := NewServer(cfg, provider)

			handler := server.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "GET")
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			hasCORS := rr.Header().Get("Access-Control-Allow-Origin") != ""
			if hasCORS != tt.expectCORS {
				t.Errorf("expected CORS headers=%v, got=%v", tt.expectCORS, hasCORS)
			}
		})
	}
}

func TestServerACLMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		allowedNetworks []string
		clientIP        string
		expectedStatus  int
	}{
		{
			name:            "no ACL allows all",
			allowedNetworks: nil,
			clientIP:        "192.168.1.1:12345",
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "IP in allowed range",
			allowedNetworks: []string{"192.168.0.0/16"},
			clientIP:        "192.168.1.1:12345",
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "IP not in allowed range",
			allowedNetworks: []string{"10.0.0.0/8"},
			clientIP:        "192.168.1.1:12345",
			expectedStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultServerConfig()
			cfg.AllowedNetworks = tt.allowedNetworks
			provider := newMockDataProvider()

			server, _ := NewServer(cfg, provider)

			handler := server.aclMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.clientIP

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestParsePathParam(t *testing.T) {
	tests := []struct {
		path     string
		prefix   string
		expected string
	}{
		{"/api/domains/test-domain", "/api/domains/", "test-domain"},
		{"/api/servers/10.0.1.10:80", "/api/servers/", "10.0.1.10:80"},
		{"/api/domains/", "/api/domains/", ""},
		{"/api/other", "/api/domains/", ""},
	}

	for _, tt := range tests {
		result := parsePathParam(tt.path, tt.prefix)
		if result != tt.expected {
			t.Errorf("parsePathParam(%s, %s) = %s, want %s", tt.path, tt.prefix, result, tt.expected)
		}
	}
}

func TestParseSubPath(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		expID  string
		expSub string
	}{
		{"/api/domains/test-domain/backends", "/api/domains/", "test-domain", "backends"},
		{"/api/servers/10.0.1.10:80/health-check", "/api/servers/", "10.0.1.10:80", "health-check"},
		{"/api/domains/test-domain", "/api/domains/", "test-domain", ""},
		{"/api/domains/", "/api/domains/", "", ""},
	}

	for _, tt := range tests {
		id, sub := parseSubPath(tt.path, tt.prefix)
		if id != tt.expID || sub != tt.expSub {
			t.Errorf("parseSubPath(%s, %s) = (%s, %s), want (%s, %s)",
				tt.path, tt.prefix, id, sub, tt.expID, tt.expSub)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]string{"test": "value"}

	writeJSON(rr, http.StatusOK, data)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if !contains(rr.Body.String(), `"test":"value"`) {
		t.Errorf("expected JSON body, got %s", rr.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()

	writeError(rr, http.StatusBadRequest, "test error", "TEST_ERROR")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !contains(body, `"error":true`) {
		t.Errorf("expected error:true in body, got %s", body)
	}
	if !contains(body, `"message":"test error"`) {
		t.Errorf("expected message in body, got %s", body)
	}
	if !contains(body, `"code":"TEST_ERROR"`) {
		t.Errorf("expected code in body, got %s", body)
	}
}

func TestGetUser(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{"with user header", "admin@example.com", "admin@example.com"},
		{"without header", "", "api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.header != "" {
				req.Header.Set("X-User", tt.header)
			}
			result := getUser(req)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.Address = ":0" // Use random available port
	provider := newMockDataProvider()

	server, err := NewServer(cfg, provider)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	cancel()
}

func TestHandleHealth(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealth, http.MethodGet, "/api/health", "")
	assertStatus(t, rr, http.StatusOK)

	var resp HealthCheckResponse
	decodeJSON(t, rr, &resp)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp.Status)
	}
}

func TestHandleHealth_POST_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleHealth, http.MethodPost, "/api/health", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

// helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
