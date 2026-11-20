// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// The records below are byte for byte what version 1.1.10 wrote.
const (
	legacyKeyPrefix = "domain_backends/web.example.com/web.example.com:"
	legacyEnabled   = `{"id":"web.example.com:10.0.0.3:8080","address":"10.0.0.3","port":8080,"weight":10,"priority":1,"region":"us-east","healthy":true,"enabled":true,"last_check":"0001-01-01T00:00:00Z"}`
	legacyDisabled  = `{"id":"web.example.com:10.0.0.4:8080","address":"10.0.0.4","port":8080,"weight":0,"priority":0,"region":"eu-west","healthy":true,"enabled":false,"last_check":"0001-01-01T00:00:00Z"}`
	legacyDuplicate = `{"id":"web.example.com:10.0.0.6:8080","address":"10.0.0.6","port":8080,"weight":5,"priority":0,"region":"us-east","healthy":true,"enabled":true,"last_check":"0001-01-01T00:00:00Z"}`
	existingServer  = `{"service":"web.example.com","address":"10.0.0.6","port":8080,"weight":50,"region":"eu-west","source":"api","created_at":"2026-11-20T20:34:18.939007833Z","updated_at":"2026-11-20T20:34:18.939007833Z","agent_healthy":false,"agent_last_seen":"2026-11-20T20:34:18.939007833Z","validation_last_check":"0001-01-01T00:00:00Z","override_at":"0001-01-01T00:00:00Z","draining":false,"draining_at":"0001-01-01T00:00:00Z","effective_status":"healthy","authority":"static"}`
)

func TestMigrateLegacyDomainBackends(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewBboltStore(filepath.Join(t.TempDir(), "overwatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seed := map[string]string{
		legacyKeyPrefix + "10.0.0.3:8080":        legacyEnabled,
		legacyKeyPrefix + "10.0.0.4:8080":        legacyDisabled,
		legacyKeyPrefix + "10.0.0.6:8080":        legacyDuplicate,
		"backends/web.example.com:10.0.0.6:8080": existingServer,
		legacyKeyPrefix + "broken":               `{"id":`,
		legacyKeyPrefix + "not-an-address:8080":  `{"id":"x","address":"backend.internal","port":8080}`,
		"domain_backends/no-slash":               legacyEnabled,
		"domains/web.example.com":                `{"name":"web.example.com","ttl":20,"routing_policy":"weighted"}`,
		"regions/ap-south":                       `{"id":"ap-south"}`,
		"backends/other.example.com:10.0.0.9:80": `{"service":"other.example.com","address":"10.0.0.9","port":80,"source":"static"}`,
	}
	for k, v := range seed {
		if err := st.Set(ctx, k, []byte(v)); err != nil {
			t.Fatal(err)
		}
	}

	migrated, err := MigrateLegacyDomainBackends(ctx, st, nil)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2 (the duplicate is dropped, three records are not readable)", migrated)
	}

	read := func(key string) Backend {
		t.Helper()
		data, err := st.Get(ctx, key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		var b Backend
		if err := json.Unmarshal(data, &b); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		return b
	}

	b := read("backends/web.example.com:10.0.0.3:8080")
	if b.Service != "web.example.com" || b.Address != "10.0.0.3" || b.Port != 8080 || b.Weight != 10 ||
		b.Region != "us-east" || b.Priority != 1 || b.Source != SourceAPI || b.Disabled {
		t.Errorf("migrated backend: %+v", b)
	}
	if b.CreatedAt.IsZero() || b.EffectiveStatus != StatusHealthy || b.HealthCheck != nil {
		t.Errorf("migrated backend bookkeeping: %+v", b)
	}

	// "enabled": false in the old record never had an effect and is not
	// carried over; weight 0 becomes the default weight 1.
	b = read("backends/web.example.com:10.0.0.4:8080")
	if b.Disabled || b.Weight != 1 || b.Region != "eu-west" {
		t.Errorf("migrated backend with enabled=false and weight 0: %+v", b)
	}

	// A server record for the same address and port is left as it is.
	b = read("backends/web.example.com:10.0.0.6:8080")
	if b.Weight != 50 || b.Region != "eu-west" {
		t.Errorf("an existing server record was overwritten: %+v", b)
	}

	left, err := st.List(ctx, store.PrefixDomainBackends)
	if err != nil {
		t.Fatal(err)
	}
	leftKeys := map[string]bool{}
	for _, pair := range left {
		leftKeys[pair.Key] = true
	}
	// Migrated and duplicate records are deleted; unreadable ones stay for
	// the operator to look at.
	for key, want := range map[string]bool{
		legacyKeyPrefix + "10.0.0.3:8080":       false,
		legacyKeyPrefix + "10.0.0.4:8080":       false,
		legacyKeyPrefix + "10.0.0.6:8080":       false,
		legacyKeyPrefix + "broken":              true,
		legacyKeyPrefix + "not-an-address:8080": true,
		"domain_backends/no-slash":              true,
	} {
		if leftKeys[key] != want {
			t.Errorf("legacy record %s: still in the store = %v, want %v", key, leftKeys[key], want)
		}
	}

	// Other records are untouched.
	for _, key := range []string{"domains/web.example.com", "regions/ap-south", "backends/other.example.com:10.0.0.9:80"} {
		data, err := st.Get(ctx, key)
		if err != nil || string(data) != seed[key] {
			t.Errorf("%s was changed: %s, %v", key, data, err)
		}
	}

	// A second run finds nothing to do.
	migrated, err = MigrateLegacyDomainBackends(ctx, st, nil)
	if err != nil || migrated != 0 {
		t.Errorf("second run: migrated = %d, err = %v; want 0, nil", migrated, err)
	}

	// The registry loads the migrated records as API servers.
	registry := NewRegistry(DefaultRegistryConfig(), st)
	if err := registry.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registry.Stop() }()
	got := registry.GetBackends("web.example.com")
	if len(got) != 3 {
		t.Fatalf("the registry loaded %d backends of web.example.com, want 3", len(got))
	}
	for _, backend := range got {
		if backend.Source != SourceAPI || backend.EffectiveStatus != StatusHealthy {
			t.Errorf("loaded backend %s:%d: source %s status %s", backend.Address, backend.Port, backend.Source, backend.EffectiveStatus)
		}
	}
}

func TestMigrateLegacyDomainBackends_NoStore(t *testing.T) {
	migrated, err := MigrateLegacyDomainBackends(context.Background(), nil, nil)
	if migrated != 0 || err != nil {
		t.Errorf("without a store: %d, %v", migrated, err)
	}
}
