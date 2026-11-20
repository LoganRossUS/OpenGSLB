// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"fmt"
	"sync"

	"github.com/loganrossus/OpenGSLB/pkg/api"
	"github.com/loganrossus/OpenGSLB/pkg/config"
	"github.com/loganrossus/OpenGSLB/pkg/dns"
	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// DNS state has three sources:
//
//  1. the configuration file (domains, and the static servers of each region),
//  2. the store (domains and domain backends created through the REST API),
//  3. the backend registry (static, API-registered and agent-registered
//     servers, which is also what the store's backends are loaded into).
//
// buildDNSRegistry merges the three. Startup and reload both call it, so a
// reload cannot serve anything other than what a restart on the same file and
// store would serve.

// buildDNSRegistry returns a new DNS registry holding the merged state for cfg.
//
// Precedence matches what the REST API reports: a stored domain replaces a
// configuration file domain of the same name.
//
// When tolerateStoreErrors is false a store read failure is returned as an
// error, so that a reload keeps the state it is serving instead of dropping
// every API-created domain. Startup has no previous state to keep and passes
// true: the failure is logged and the configuration file domains are served.
func (a *Application) buildDNSRegistry(cfg *config.Config, tolerateStoreErrors bool) (*dns.Registry, error) {
	// 1. Configuration file.
	registry, err := dns.BuildRegistry(cfg, a.routerFactory.NewRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to build DNS registry: %w", err)
	}

	// 2. Domains and domain backends created through the API. They are only
	// served while the API is enabled, which is what startup has always done.
	if cfg.API.Enabled && a.overwatchStore != nil {
		loader := api.NewRegistryDomainProvider(a.backendRegistry, cfg, a.logger)
		loader.SetStore(a.overwatchStore)
		loader.SetDNSRegistry(registry)
		loader.SetRouterFactory(a.newDynamicRouter)
		if err := loader.LoadStoredDomainsIntoDNS(); err != nil {
			if !tolerateStoreErrors {
				return nil, fmt.Errorf("failed to load stored domains: %w", err)
			}
			a.logger.Warn("failed to load stored domains into DNS registry", "error", err)
		}
	}

	// 3. Backend registry. At startup it holds the static servers only: the
	// backends persisted in the store are loaded when the registry starts and
	// reach the DNS registry through its status callback. On a reload it
	// holds every backend. Static servers that cfg no longer defines are left
	// out here and removed by pruneStaticBackends.
	//
	// Like the status callback, this registers a backend with the domain named
	// by its service whatever the domain's region list says.
	if a.backendRegistry != nil {
		static := staticBackendKeys(cfg)
		for _, b := range a.backendRegistry.GetAllBackends() {
			if b.Source == overwatch.SourceStatic && !static[staticBackendKey(b.Service, b.Address, b.Port)] {
				continue
			}
			if err := registry.RegisterServer(b.Service, b.Address, b.Port, b.Weight, b.Region); err != nil {
				a.logger.Debug("backend not added to DNS registry",
					"service", b.Service,
					"address", b.Address,
					"port", b.Port,
					"error", err,
				)
			}
		}
	}

	return registry, nil
}

// newDynamicRouter adapts the shared router factory to the untyped factory
// the API layer and the DNS registry use for domains created at runtime.
func (a *Application) newDynamicRouter(algorithm string) (interface{}, error) {
	return a.routerFactory.NewRouter(algorithm)
}

// staticBackendKey identifies a static server the way the backend registry
// does (service, address, port).
func staticBackendKey(service, address string, port int) string {
	return fmt.Sprintf("%s:%s:%d", service, address, port)
}

// staticBackendKeys returns the static servers cfg defines.
func staticBackendKeys(cfg *config.Config) map[string]bool {
	keys := make(map[string]bool)
	for _, region := range cfg.Regions {
		for _, server := range region.Servers {
			keys[staticBackendKey(server.Service, server.Address, server.Port)] = true
		}
	}
	return keys
}

// pruneStaticBackends removes from the backend registry (and with it from the
// store) every static server that cfg does not define. The configuration file
// is the only source of static servers, so one that left the file must stop
// being validated, listed and served. It returns the number removed.
func (a *Application) pruneStaticBackends(cfg *config.Config) int {
	if a.backendRegistry == nil {
		return 0
	}
	static := staticBackendKeys(cfg)
	removed := 0
	for _, b := range a.backendRegistry.GetAllBackends() {
		if b.Source != overwatch.SourceStatic || static[staticBackendKey(b.Service, b.Address, b.Port)] {
			continue
		}
		if err := a.backendRegistry.Deregister(b.Service, b.Address, b.Port); err != nil {
			a.logger.Warn("failed to remove static server that is no longer configured",
				"service", b.Service,
				"address", b.Address,
				"port", b.Port,
				"error", err,
			)
			continue
		}
		removed++
		a.logger.Info("removed static server that is no longer configured",
			"service", b.Service,
			"address", b.Address,
			"port", b.Port,
		)
	}
	return removed
}

// dnsStateSync is the write path to the live DNS registry for everything that
// changes it at runtime: the REST API, agent gossip and the backend registry
// status callback. It lets a reload replace the registry contents without
// losing a change that arrives while the replacement is being built.
//
// A reload takes its snapshot of the store and the backend registry without
// holding any lock here (the status callback can run under the backend
// registry lock, so holding a lock across the snapshot could deadlock).
// Instead, every change made while a rebuild is in flight is applied to the
// live registry and also recorded. Before the new contents are published the
// recorded changes are replayed onto them, in order, under the same lock that
// serializes the changes. A change is therefore either in the snapshot, or
// replayed, or made after the swap. Replaying a change the snapshot already
// reflects is harmless: registering a server is an upsert and removing an
// absent server or domain is a no-op.
type dnsStateSync struct {
	mu         sync.Mutex
	live       *dns.Registry
	rebuilding bool
	journal    []func(*dns.Registry)
}

func newDNSStateSync(live *dns.Registry) *dnsStateSync {
	return &dnsStateSync{live: live}
}

// apply runs op on the live registry and records it when a rebuild is in flight.
func (s *dnsStateSync) apply(op func(*dns.Registry) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rebuilding {
		s.journal = append(s.journal, func(r *dns.Registry) { _ = op(r) })
	}
	return op(s.live)
}

// rebuild replaces the contents of the live registry with what build returns.
// If build fails the live registry is left untouched. Calls must not overlap;
// Application.Reload serializes them.
func (s *dnsStateSync) rebuild(build func() (*dns.Registry, error)) error {
	s.mu.Lock()
	s.rebuilding = true
	s.journal = nil
	s.mu.Unlock()

	fresh, err := build()

	s.mu.Lock()
	defer s.mu.Unlock()
	journal := s.journal
	s.rebuilding = false
	s.journal = nil
	if err != nil {
		return err
	}

	for _, op := range journal {
		op(fresh)
	}

	names := fresh.Domains()
	entries := make([]*dns.DomainEntry, 0, len(names))
	for _, name := range names {
		if entry := fresh.Lookup(name); entry != nil {
			entries = append(entries, entry)
		}
	}
	s.live.ReplaceAll(entries)
	return nil
}

// RegisterDomainDynamic implements api.DNSRegistryInterface.
func (s *dnsStateSync) RegisterDomainDynamic(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error {
	return s.apply(func(r *dns.Registry) error {
		return r.RegisterDomainDynamic(name, ttl, algorithm, routerFactory)
	})
}

// UpdateDomainSettings implements api.DNSRegistryInterface.
func (s *dnsStateSync) UpdateDomainSettings(name string, ttl uint32, algorithm string, routerFactory func(string) (interface{}, error)) error {
	return s.apply(func(r *dns.Registry) error {
		return r.UpdateDomainSettings(name, ttl, algorithm, routerFactory)
	})
}

// Remove implements api.DNSRegistryInterface.
func (s *dnsStateSync) Remove(name string) {
	_ = s.apply(func(r *dns.Registry) error {
		r.Remove(name)
		return nil
	})
}

// RegisterServer implements api.DNSRegistryInterface and overwatch.DNSRegistry.
func (s *dnsStateSync) RegisterServer(service string, address string, port int, weight int, region string) error {
	return s.apply(func(r *dns.Registry) error {
		return r.RegisterServer(service, address, port, weight, region)
	})
}

// DeregisterServer implements api.DNSRegistryInterface and overwatch.DNSRegistry.
func (s *dnsStateSync) DeregisterServer(service string, address string, port int) error {
	return s.apply(func(r *dns.Registry) error {
		return r.DeregisterServer(service, address, port)
	})
}

var (
	_ api.DNSRegistryInterface = (*dnsStateSync)(nil)
	_ overwatch.DNSRegistry    = (*dnsStateSync)(nil)
)
