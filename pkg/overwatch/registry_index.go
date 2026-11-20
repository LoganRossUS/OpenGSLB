// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import "sort"

// endpointKey identifies a backend by address and port, whatever its service.
// The DNS query path asks about health and latency with only these two.
type endpointKey struct {
	address string
	port    int
}

// putBackend stores backend under key and adds it to the endpoint index.
// Every write to r.backends goes through putBackend and removeBackend so that
// the index cannot drift from the map (a test enforces this).
// The caller must hold r.mu for writing.
func (r *Registry) putBackend(key string, backend *Backend) {
	if previous, exists := r.backends[key]; exists {
		r.unindexBackend(previous)
	}
	r.backends[key] = backend

	ek := endpointKey{address: backend.Address, port: backend.Port}
	list := r.byEndpoint[ek]
	// Keep the backends of an endpoint ordered by service so that lookups
	// give the same answer every time.
	i := sort.Search(len(list), func(i int) bool { return list[i].Service >= backend.Service })
	list = append(list, nil)
	copy(list[i+1:], list[i:])
	list[i] = backend
	r.byEndpoint[ek] = list
}

// removeBackend deletes the backend stored under key from the registry and
// from the endpoint index. It does nothing when there is none.
// The caller must hold r.mu for writing.
func (r *Registry) removeBackend(key string) {
	backend, exists := r.backends[key]
	if !exists {
		return
	}
	delete(r.backends, key)
	r.unindexBackend(backend)
}

// unindexBackend removes backend from the endpoint index.
// The caller must hold r.mu for writing.
func (r *Registry) unindexBackend(backend *Backend) {
	ek := endpointKey{address: backend.Address, port: backend.Port}
	list := r.byEndpoint[ek]
	for i, b := range list {
		if b != backend {
			continue
		}
		if len(list) == 1 {
			delete(r.byEndpoint, ek)
			return
		}
		copy(list[i:], list[i+1:])
		list[len(list)-1] = nil
		r.byEndpoint[ek] = list[:len(list)-1]
		return
	}
}

// servable reports whether a backend may be handed out in DNS answers: it is
// healthy and not draining.
func servable(b *Backend) bool {
	return !b.Draining && b.EffectiveStatus == StatusHealthy
}

// GetBackendByEndpoint returns a copy of the backend registered at an address
// and port, found through an index: the cost does not depend on the number of
// registered backends. It is the lookup of the DNS query path, which knows a
// server's address and port but not its service.
//
// The same address and port can be registered under more than one service.
// In that case the first backend, in order of service name, that is unhealthy
// or draining is returned, and the first of all when none is. An endpoint
// therefore counts as healthy only when every backend registered at it is.
func (r *Registry) GetBackendByEndpoint(address string, port int) (Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := r.byEndpoint[endpointKey{address: address, port: port}]
	if len(list) == 0 {
		return Backend{}, false
	}
	for _, b := range list {
		if !servable(b) {
			return *b, true
		}
	}
	return *list[0], true
}
