// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// An API-registered backend has no agent and sends no heartbeat. It must not
// be treated as a stale agent backend and removed after remove_after.
func TestRegistry_APIBackendIsNotRemovedAsStale(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		StaleThreshold: 10 * time.Millisecond,
		RemoveAfter:    20 * time.Millisecond,
	}, nil)

	if err := registry.RegisterAPI("web", "192.168.1.1", 80, 100, "us-east"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("agent-1", "us-east", "web", "192.168.1.2", 80, 100, true); err != nil {
		t.Fatal(err)
	}

	time.Sleep(40 * time.Millisecond)
	registry.checkStaleBackends()

	b, ok := registry.GetBackend("web", "192.168.1.1", 80)
	if !ok {
		t.Fatal("the API-registered backend was removed as stale")
	}
	if b.EffectiveStatus != StatusHealthy {
		t.Errorf("API backend status = %s, want healthy", b.EffectiveStatus)
	}
	if _, ok := registry.GetBackend("web", "192.168.1.2", 80); ok {
		t.Error("the agent backend without heartbeats should have been removed")
	}
}

func TestRegistry_RegisterAPIBackend(t *testing.T) {
	registry := NewRegistry(RegistryConfig{}, nil)

	var events []string
	registry.OnStatusChange(func(b *Backend, oldStatus, newStatus BackendStatus) {
		events = append(events, string(oldStatus)+">"+string(newStatus))
	})

	spec := APIBackend{
		Service: "web", Address: "192.168.1.1", Port: 80, Weight: 5, Region: "us-east",
		Priority: 2, Disabled: true,
		HealthCheck: &BackendHealthCheck{Type: "tcp", Interval: 7 * time.Second, Timeout: time.Second},
	}
	if err := registry.RegisterAPIBackend(spec); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterAPIBackend(spec); !errors.Is(err, ErrBackendExists) {
		t.Fatalf("second registration: err = %v, want ErrBackendExists", err)
	}

	b, ok := registry.GetBackend("web", "192.168.1.1", 80)
	if !ok {
		t.Fatal("backend not registered")
	}
	if b.Source != SourceAPI || !b.Disabled || b.Priority != 2 || b.Weight != 5 {
		t.Errorf("unexpected backend: %+v", b)
	}
	if b.HealthCheck == nil || b.HealthCheck.Type != "tcp" || b.HealthCheck.Interval != 7*time.Second {
		t.Errorf("health check not stored: %+v", b.HealthCheck)
	}
	if b.CheckType() != "tcp" {
		t.Errorf("CheckType = %s, want tcp", b.CheckType())
	}
	// The caller's struct is not aliased.
	spec.HealthCheck.Type = "http"
	if b2, _ := registry.GetBackend("web", "192.168.1.1", 80); b2.HealthCheck.Type != "tcp" {
		t.Error("the registry kept a reference to the caller's health check")
	}
	if len(events) != 1 || events[0] != ">healthy" {
		t.Errorf("callback events = %v, want one registration", events)
	}
}

func TestRegistry_Modify(t *testing.T) {
	registry := NewRegistry(RegistryConfig{}, nil)
	if err := registry.RegisterAPI("web", "192.168.1.1", 80, 100, "us-east"); err != nil {
		t.Fatal(err)
	}

	var notified []Backend
	registry.OnStatusChange(func(b *Backend, _, _ BackendStatus) { notified = append(notified, *b) })

	// A failing fn changes nothing and notifies nobody.
	wantErr := errors.New("refused")
	if _, err := registry.Modify("web", "192.168.1.1", 80, func(b *Backend) error {
		b.Weight = 1
		b.HealthCheck = &BackendHealthCheck{Type: "tcp"}
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the error of fn", err)
	}
	if b, _ := registry.GetBackend("web", "192.168.1.1", 80); b.Weight != 100 || b.HealthCheck != nil || len(notified) != 0 {
		t.Fatalf("a refused modification was applied: %+v (notified %d)", b, len(notified))
	}

	// A successful one is applied, and listeners hear about it although the
	// effective status did not change.
	got, err := registry.Modify("web", "192.168.1.1", 80, func(b *Backend) error {
		b.Weight = 70
		b.Region = "eu-west"
		b.Disabled = true
		b.Service = "other" // identity is not for fn to change
		b.Source = SourceStatic
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Weight != 70 || got.Region != "eu-west" || !got.Disabled || got.Service != "web" || got.Source != SourceAPI {
		t.Errorf("unexpected result: %+v", got)
	}
	if len(notified) != 1 || notified[0].Weight != 70 || !notified[0].Disabled {
		t.Errorf("listeners were not told about the modification: %+v", notified)
	}

	if _, err := registry.Modify("web", "192.168.1.9", 80, func(*Backend) error { return nil }); !errors.Is(err, ErrBackendNotFound) {
		t.Errorf("unknown backend: err = %v, want ErrBackendNotFound", err)
	}
	if err := registry.Deregister("web", "192.168.1.9", 80); !errors.Is(err, ErrBackendNotFound) {
		t.Errorf("Deregister of an unknown backend: err = %v, want ErrBackendNotFound", err)
	}
}

// Per-backend thresholds take precedence over the node-wide ones, and turning
// the check off forgets the verdict.
func TestRegistry_PerBackendThresholds(t *testing.T) {
	registry := NewRegistry(RegistryConfig{ValidationFailureThreshold: 1, ValidationSuccessThreshold: 1}, nil)
	if err := registry.RegisterAPIBackend(APIBackend{
		Service: "web", Address: "192.168.1.1", Port: 80, Weight: 1,
		HealthCheck: &BackendHealthCheck{UnhealthyThreshold: 2, HealthyThreshold: 2},
	}); err != nil {
		t.Fatal(err)
	}
	status := func() BackendStatus {
		b, _ := registry.GetBackend("web", "192.168.1.1", 80)
		return b.EffectiveStatus
	}
	validate := func(healthy bool) {
		if err := registry.UpdateValidation("web", "192.168.1.1", 80, healthy, ""); err != nil {
			t.Fatal(err)
		}
	}

	validate(false)
	if status() != StatusHealthy {
		t.Fatal("one failure must not be enough with unhealthy_threshold 2")
	}
	validate(false)
	if status() != StatusUnhealthy {
		t.Fatal("two failures must turn the backend unhealthy")
	}
	validate(true)
	if status() != StatusUnhealthy {
		t.Fatal("one pass must not be enough with healthy_threshold 2")
	}
	validate(true)
	if status() != StatusHealthy {
		t.Fatal("two passes must turn the backend healthy")
	}

	validate(false)
	validate(false)
	if _, err := registry.Modify("web", "192.168.1.1", 80, func(b *Backend) error {
		b.HealthCheck.Disabled = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b, _ := registry.GetBackend("web", "192.168.1.1", 80)
	if b.ValidationHealthy != nil || b.EffectiveStatus != StatusHealthy || b.Authority != AuthorityStatic {
		t.Errorf("a backend with its check turned off should be healthy by default, got %s/%s", b.EffectiveStatus, b.Authority)
	}
}

// The API fields survive a restart, and records written before they existed
// read as enabled with the node-wide check.
func TestRegistry_APIBackendPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overwatch.db")
	st, err := store.NewBboltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(RegistryConfig{}, st)
	if err := registry.RegisterAPIBackend(APIBackend{
		Service: "web", Address: "192.168.1.1", Port: 80, Weight: 5, Region: "us-east", Disabled: true, Priority: 3,
		HealthCheck: &BackendHealthCheck{Type: "https", Path: "/healthz", Host: "web.example.com", Interval: 9 * time.Second},
	}); err != nil {
		t.Fatal(err)
	}
	// A record as version 1.1.10 wrote it: no disabled, priority or health_check.
	old, _ := json.Marshal(map[string]any{
		"service": "web", "address": "192.168.1.2", "port": 80, "weight": 10, "region": "us-east",
		"source": "api", "agent_healthy": false, "effective_status": "healthy", "authority": "static",
		"agent_last_seen": time.Now().Add(-time.Hour), "created_at": time.Now().Add(-time.Hour), "updated_at": time.Now().Add(-time.Hour),
	})
	if err := st.Set(context.Background(), "backends/web:192.168.1.2:80", old); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.NewBboltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	reloaded := NewRegistry(RegistryConfig{StaleThreshold: time.Millisecond, RemoveAfter: time.Millisecond}, st)
	if err := reloaded.loadFromStore(); err != nil {
		t.Fatal(err)
	}
	reloaded.checkStaleBackends()

	b, ok := reloaded.GetBackend("web", "192.168.1.1", 80)
	if !ok {
		t.Fatal("API backend not loaded")
	}
	if !b.Disabled || b.Priority != 3 || b.HealthCheck == nil || b.HealthCheck.Path != "/healthz" ||
		b.HealthCheck.Host != "web.example.com" || b.HealthCheck.Interval != 9*time.Second || b.CheckType() != "https" {
		t.Errorf("API fields lost across a restart: %+v %+v", b, b.HealthCheck)
	}
	legacy, ok := reloaded.GetBackend("web", "192.168.1.2", 80)
	if !ok {
		t.Fatal("the API backend stored by an older version was removed after the restart")
	}
	if legacy.Disabled || legacy.HealthCheck != nil || legacy.CheckType() != "http" {
		t.Errorf("an old record must read as enabled with the node-wide check: %+v", legacy)
	}
}
