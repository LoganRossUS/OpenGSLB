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
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// emptyRegistry is a RegistryInterface with no dynamic backends, so results
// come from the config file alone.
type emptyRegistry struct{}

func (emptyRegistry) GetAllBackends() []*overwatch.Backend    { return nil }
func (emptyRegistry) GetBackends(string) []*overwatch.Backend { return nil }
func (emptyRegistry) GetBackend(string, string, int) (*overwatch.Backend, bool) {
	return nil, false
}

// sharedRegionConfig has two domains in the same two regions; each region
// holds one server per domain, tied to it by the server's service field.
func sharedRegionConfig() *config.Config {
	return &config.Config{
		Regions: []config.Region{
			{Name: "us-east", Servers: []config.Server{
				{Address: "10.0.0.1", Port: 80, Weight: 100, Service: "app.example.com"},
				{Address: "10.0.0.2", Port: 80, Weight: 100, Service: "api.example.com"},
			}},
			{Name: "eu-west", Servers: []config.Server{
				// Service spelled differently from the domain: matched case-insensitively,
				// trailing dot ignored.
				{Address: "10.0.1.1", Port: 80, Weight: 100, Service: "APP.example.com."},
				{Address: "10.0.1.2", Port: 80, Weight: 100, Service: "api.example.com"},
			}},
		},
		Domains: []config.Domain{
			{Name: "app.example.com", Regions: []string{"us-east", "eu-west"}},
			{Name: "api.example.com", Regions: []string{"us-east", "eu-west"}},
		},
	}
}

func TestGetDomainBackends_FiltersConfigServersByService(t *testing.T) {
	p := NewRegistryDomainProvider(emptyRegistry{}, sharedRegionConfig(), nil)

	tests := map[string][]string{
		"app.example.com": {"10.0.0.1", "10.0.1.1"},
		"api.example.com": {"10.0.0.2", "10.0.1.2"},
	}
	for domain, want := range tests {
		backends, err := p.GetDomainBackends(domain)
		if err != nil {
			t.Fatalf("GetDomainBackends(%q): %v", domain, err)
		}
		got := make([]string, 0, len(backends))
		for _, b := range backends {
			got = append(got, b.Address)
			if !strings.HasPrefix(b.ID, domain+":") {
				t.Errorf("%s: backend ID %q does not start with the domain", domain, b.ID)
			}
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("GetDomainBackends(%q) addresses = %v, want %v", domain, got, want)
		}
	}
}

func TestDomainBackendCounts_FilterConfigServersByService(t *testing.T) {
	p := NewRegistryDomainProvider(emptyRegistry{}, sharedRegionConfig(), nil)

	for _, d := range p.ListDomains() {
		if d.BackendCount != 2 || d.HealthyBackends != 2 {
			t.Errorf("ListDomains %s: BackendCount=%d HealthyBackends=%d, want 2/2",
				d.Name, d.BackendCount, d.HealthyBackends)
		}
	}

	for _, name := range []string{"app.example.com", "api.example.com"} {
		d, err := p.GetDomain(name)
		if err != nil {
			t.Fatalf("GetDomain(%q): %v", name, err)
		}
		if d.BackendCount != 2 || d.HealthyBackends != 2 {
			t.Errorf("GetDomain %s: BackendCount=%d HealthyBackends=%d, want 2/2",
				name, d.BackendCount, d.HealthyBackends)
		}
	}
}

func TestRemoveDomainBackend_OtherDomainsConfigServerIsNotFound(t *testing.T) {
	p := NewRegistryDomainProvider(emptyRegistry{}, sharedRegionConfig(), nil)
	p.SetStore(newTestStore(t))

	// A config server of this domain cannot be removed via the API.
	err := p.RemoveDomainBackend("app.example.com", "app.example.com:10.0.0.1:80")
	if err == nil || !strings.Contains(err.Error(), "config-based") {
		t.Errorf("own config backend: err = %v, want config-based error", err)
	}

	// api.example.com's server is not a backend of app.example.com at all.
	err = p.RemoveDomainBackend("app.example.com", "app.example.com:10.0.0.2:80")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("other domain's server: err = %v, want not found", err)
	}
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewBboltStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewBboltStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
