// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	mu      sync.RWMutex
	data    map[string][]byte
	setErr  error
	getErr  error
	delErr  error
	listErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
	}
}

func (m *mockStore) Get(ctx context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.data[key]; ok {
		return val, nil
	}
	return nil, nil
}

func (m *mockStore) Set(ctx context.Context, key string, value []byte) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockStore) Delete(ctx context.Context, key string) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

type kvPair struct {
	Key   string
	Value []byte
}

func (m *mockStore) List(ctx context.Context, prefix string) ([]kvPair, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []kvPair
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			result = append(result, kvPair{Key: k, Value: v})
		}
	}
	return result, nil
}

func (m *mockStore) Watch(ctx context.Context, prefix string) (<-chan interface{}, error) {
	return nil, nil
}

func (m *mockStore) Close() error {
	return nil
}

// TestOverrideManager_SetOverride tests setting an override.
func TestOverrideManager_SetOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	override, err := manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "High latency", "cloudwatch", "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if override.Service != "myapp" {
		t.Errorf("expected service 'myapp', got '%s'", override.Service)
	}
	if override.Address != "10.0.1.10:8080" {
		t.Errorf("expected address '10.0.1.10:8080', got '%s'", override.Address)
	}
	if override.Healthy {
		t.Error("expected healthy=false")
	}
	if override.Reason != "High latency" {
		t.Errorf("expected reason 'High latency', got '%s'", override.Reason)
	}
	if override.Source != "cloudwatch" {
		t.Errorf("expected source 'cloudwatch', got '%s'", override.Source)
	}
	if override.Authority != OverrideAuthority {
		t.Errorf("expected authority '%s', got '%s'", OverrideAuthority, override.Authority)
	}
	if override.CreatedAt.IsZero() {
		t.Error("expected created_at to be set")
	}
}

// TestOverrideManager_SetOverride_ValidationErrors tests validation errors.
func TestOverrideManager_SetOverride_ValidationErrors(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	tests := []struct {
		name    string
		service string
		address string
		wantErr bool
	}{
		{"empty service", "", "10.0.1.10:8080", true},
		{"empty address", "myapp", "", true},
		{"valid", "myapp", "10.0.1.10:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.SetOverride(context.Background(), tt.service, tt.address, false, "test", "test", "")
			if (err != nil) != tt.wantErr {
				t.Errorf("SetOverride() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestOverrideManager_GetOverride tests retrieving an override.
func TestOverrideManager_GetOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Get non-existent override
	_, exists := manager.GetOverride("myapp", "10.0.1.10:8080")
	if exists {
		t.Error("expected override to not exist")
	}

	// Set and get
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "")

	override, exists := manager.GetOverride("myapp", "10.0.1.10:8080")
	if !exists {
		t.Error("expected override to exist")
	}
	if override.Service != "myapp" {
		t.Errorf("expected service 'myapp', got '%s'", override.Service)
	}
}

// TestOverrideManager_ClearOverride tests clearing an override.
func TestOverrideManager_ClearOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Clear non-existent override
	err := manager.ClearOverride(context.Background(), "myapp", "10.0.1.10:8080", "")
	if err == nil {
		t.Error("expected error when clearing non-existent override")
	}

	// Set and clear
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "")

	err = manager.ClearOverride(context.Background(), "myapp", "10.0.1.10:8080", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cleared
	_, exists := manager.GetOverride("myapp", "10.0.1.10:8080")
	if exists {
		t.Error("expected override to be cleared")
	}
}

// TestOverrideManager_ListOverrides tests listing all overrides.
func TestOverrideManager_ListOverrides(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Empty list
	overrides := manager.ListOverrides()
	if len(overrides) != 0 {
		t.Errorf("expected empty list, got %d", len(overrides))
	}

	// Add overrides
	manager.SetOverride(context.Background(), "app1", "10.0.1.10:8080", false, "test1", "src1", "")
	manager.SetOverride(context.Background(), "app2", "10.0.1.11:8080", true, "test2", "src2", "")

	overrides = manager.ListOverrides()
	if len(overrides) != 2 {
		t.Errorf("expected 2 overrides, got %d", len(overrides))
	}
}

// TestOverrideManager_IsOverridden tests checking if a server is overridden.
func TestOverrideManager_IsOverridden(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Not overridden
	_, hasOverride := manager.IsOverridden("myapp", "10.0.1.10:8080")
	if hasOverride {
		t.Error("expected no override")
	}

	// Set unhealthy override
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "")

	healthy, hasOverride := manager.IsOverridden("myapp", "10.0.1.10:8080")
	if !hasOverride {
		t.Error("expected override to exist")
	}
	if healthy {
		t.Error("expected healthy=false")
	}

	// Set healthy override
	manager.SetOverride(context.Background(), "myapp2", "10.0.1.11:8080", true, "test", "test", "")

	healthy, hasOverride = manager.IsOverridden("myapp2", "10.0.1.11:8080")
	if !hasOverride {
		t.Error("expected override to exist")
	}
	if !healthy {
		t.Error("expected healthy=true")
	}
}

// TestOverrideManager_AuditLogger tests audit logging callback.
func TestOverrideManager_AuditLogger(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	var auditLogs []string
	manager.SetAuditLogger(func(action, service, address, reason, source, clientIP string) {
		auditLogs = append(auditLogs, action+":"+service+":"+address)
	})

	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "192.168.1.1")
	manager.ClearOverride(context.Background(), "myapp", "10.0.1.10:8080", "192.168.1.1")

	if len(auditLogs) != 2 {
		t.Errorf("expected 2 audit logs, got %d", len(auditLogs))
	}
}

// TestOverrideManager_Count tests counting overrides.
func TestOverrideManager_Count(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	if manager.Count() != 0 {
		t.Errorf("expected count 0, got %d", manager.Count())
	}

	manager.SetOverride(context.Background(), "app1", "10.0.1.10:8080", false, "test", "test", "")
	manager.SetOverride(context.Background(), "app2", "10.0.1.11:8080", false, "test", "test", "")

	if manager.Count() != 2 {
		t.Errorf("expected count 2, got %d", manager.Count())
	}
}

// TestOverrideManager_Concurrent tests concurrent access.
func TestOverrideManager_Concurrent(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			service := "app"
			address := "10.0.1.10:8080"
			manager.SetOverride(context.Background(), service, address, idx%2 == 0, "test", "test", "")
			manager.GetOverride(service, address)
			manager.ListOverrides()
			manager.IsOverridden(service, address)
		}(i)
	}
	wg.Wait()
}

// TestOverrideHandlers_SetOverride tests PUT /api/v1/overrides/{service}/{address}.
func TestOverrideHandlers_SetOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	body := `{"healthy": false, "reason": "High latency detected", "source": "cloudwatch-alarm"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/overrides/myapp/10.0.1.10:8080", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.HandleSetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp OverrideResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Service != "myapp" {
		t.Errorf("expected service 'myapp', got '%s'", resp.Service)
	}
	if resp.Address != "10.0.1.10:8080" {
		t.Errorf("expected address '10.0.1.10:8080', got '%s'", resp.Address)
	}
	if resp.Healthy {
		t.Error("expected healthy=false")
	}
	if resp.Source != "cloudwatch-alarm" {
		t.Errorf("expected source 'cloudwatch-alarm', got '%s'", resp.Source)
	}
	if resp.Authority != OverrideAuthority {
		t.Errorf("expected authority '%s', got '%s'", OverrideAuthority, resp.Authority)
	}
}

// TestOverrideHandlers_SetOverride_MissingSource tests validation.
func TestOverrideHandlers_SetOverride_MissingSource(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	body := `{"healthy": false, "reason": "test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/overrides/myapp/10.0.1.10:8080", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.HandleSetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestOverrideHandlers_SetOverride_InvalidJSON tests invalid JSON handling.
func TestOverrideHandlers_SetOverride_InvalidJSON(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/overrides/myapp/10.0.1.10:8080", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.HandleSetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// TestOverrideHandlers_ClearOverride tests DELETE /api/v1/overrides/{service}/{address}.
func TestOverrideHandlers_ClearOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	// First set an override
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/overrides/myapp/10.0.1.10:8080", nil)
	w := httptest.NewRecorder()

	handlers.HandleClearOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	// Verify cleared
	_, exists := manager.GetOverride("myapp", "10.0.1.10:8080")
	if exists {
		t.Error("expected override to be cleared")
	}
}

// TestOverrideHandlers_ClearOverride_NotFound tests clearing non-existent override.
func TestOverrideHandlers_ClearOverride_NotFound(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/overrides/myapp/10.0.1.10:8080", nil)
	w := httptest.NewRecorder()

	handlers.HandleClearOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestOverrideHandlers_GetOverride tests GET /api/v1/overrides/{service}/{address}.
func TestOverrideHandlers_GetOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	// Set an override
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "High latency", "cloudwatch", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overrides/myapp/10.0.1.10:8080", nil)
	w := httptest.NewRecorder()

	handlers.HandleGetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp OverrideResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Service != "myapp" {
		t.Errorf("expected service 'myapp', got '%s'", resp.Service)
	}
}

// TestOverrideHandlers_GetOverride_NotFound tests getting non-existent override.
func TestOverrideHandlers_GetOverride_NotFound(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overrides/myapp/10.0.1.10:8080", nil)
	w := httptest.NewRecorder()

	handlers.HandleGetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// TestOverrideHandlers_ListOverrides tests GET /api/v1/overrides.
func TestOverrideHandlers_ListOverrides(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	// Add some overrides
	manager.SetOverride(context.Background(), "app1", "10.0.1.10:8080", false, "test1", "src1", "")
	manager.SetOverride(context.Background(), "app2", "10.0.1.11:8080", true, "test2", "src2", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overrides", nil)
	w := httptest.NewRecorder()

	handlers.HandleListOverrides(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp OverridesListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Overrides) != 2 {
		t.Errorf("expected 2 overrides, got %d", len(resp.Overrides))
	}
}

// TestOverrideHandlers_ListOverrides_Empty tests listing empty overrides.
func TestOverrideHandlers_ListOverrides_Empty(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overrides", nil)
	w := httptest.NewRecorder()

	handlers.HandleListOverrides(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp OverridesListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Overrides) != 0 {
		t.Errorf("expected 0 overrides, got %d", len(resp.Overrides))
	}
}

// TestOverrideHandlers_HandleOverrides_Routing tests path routing.
func TestOverrideHandlers_HandleOverrides_Routing(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "list overrides",
			method:     http.MethodGet,
			path:       "/api/v1/overrides",
			wantStatus: http.StatusOK,
		},
		{
			name:       "list overrides with trailing slash",
			method:     http.MethodGet,
			path:       "/api/v1/overrides/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "set override",
			method:     http.MethodPut,
			path:       "/api/v1/overrides/myapp/10.0.1.10:8080",
			body:       `{"healthy": false, "reason": "test", "source": "test"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid path - missing address",
			method:     http.MethodPut,
			path:       "/api/v1/overrides/myapp",
			body:       `{"healthy": false, "source": "test"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid path - empty service",
			method:     http.MethodPut,
			path:       "/api/v1/overrides//10.0.1.10:8080",
			body:       `{"healthy": false, "source": "test"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			w := httptest.NewRecorder()

			handlers.HandleOverrides(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

// TestOverrideHandlers_MethodNotAllowed tests method validation.
func TestOverrideHandlers_MethodNotAllowed(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"post to list", http.MethodPost, "/api/v1/overrides"},
		{"patch to override", http.MethodPatch, "/api/v1/overrides/myapp/10.0.1.10:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handlers.HandleOverrides(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
			}
		})
	}
}

// TestOverrideHandlers_ExtractClientIP tests client IP extraction.
func TestOverrideHandlers_ExtractClientIP(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	tests := []struct {
		name         string
		remoteAddr   string
		xff          string
		xri          string
		expectedIP   string
	}{
		{
			name:       "remote addr with port",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "remote addr without port",
			remoteAddr: "192.168.1.100",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "x-forwarded-for",
			remoteAddr: "10.0.0.1:12345",
			xff:        "192.168.1.50, 10.0.0.1",
			expectedIP: "192.168.1.50",
		},
		{
			name:       "x-real-ip",
			remoteAddr: "10.0.0.1:12345",
			xri:        "192.168.1.50",
			expectedIP: "192.168.1.50",
		},
		{
			name:       "x-forwarded-for takes precedence",
			remoteAddr: "10.0.0.1:12345",
			xff:        "192.168.1.60",
			xri:        "192.168.1.50",
			expectedIP: "192.168.1.60",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			ip := handlers.extractClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("expected IP '%s', got '%s'", tt.expectedIP, ip)
			}
		})
	}
}

// TestOverrideManager_UpdateOverride tests updating an existing override.
func TestOverrideManager_UpdateOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Set initial override
	manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "initial", "src1", "")

	// Update override
	override, err := manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", true, "updated", "src2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !override.Healthy {
		t.Error("expected healthy=true after update")
	}
	if override.Reason != "updated" {
		t.Errorf("expected reason 'updated', got '%s'", override.Reason)
	}

	// Verify only one override exists
	if manager.Count() != 1 {
		t.Errorf("expected count 1, got %d", manager.Count())
	}
}

// TestOverrideHandlers_HealthyOverride tests setting a healthy override.
func TestOverrideHandlers_HealthyOverride(t *testing.T) {
	manager := NewOverrideManager(nil, nil)
	handlers := NewOverrideHandlers(manager, nil)

	body := `{"healthy": true, "reason": "Manual override to healthy", "source": "admin-console"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/overrides/myapp/10.0.1.10:8080", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handlers.HandleSetOverride(w, req, "myapp", "10.0.1.10:8080")

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp OverrideResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
}

// TestOverrideManager_Clear tests clearing all overrides.
func TestOverrideManager_Clear(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	// Add some overrides
	manager.SetOverride(context.Background(), "app1", "10.0.1.10:8080", false, "test1", "src1", "")
	manager.SetOverride(context.Background(), "app2", "10.0.1.11:8080", false, "test2", "src2", "")

	if manager.Count() != 2 {
		t.Fatalf("expected 2 overrides, got %d", manager.Count())
	}

	// Clear all
	err := manager.Clear(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manager.Count() != 0 {
		t.Errorf("expected 0 overrides after clear, got %d", manager.Count())
	}
}

// TestOverrideResponse_Timestamp tests that timestamps are properly formatted.
func TestOverrideResponse_Timestamp(t *testing.T) {
	manager := NewOverrideManager(nil, nil)

	before := time.Now().UTC()
	override, _ := manager.SetOverride(context.Background(), "myapp", "10.0.1.10:8080", false, "test", "test", "")
	after := time.Now().UTC()

	if override.CreatedAt.Before(before) || override.CreatedAt.After(after) {
		t.Errorf("created_at should be between %v and %v, got %v", before, after, override.CreatedAt)
	}
}
