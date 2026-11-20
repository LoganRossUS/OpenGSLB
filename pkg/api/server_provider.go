// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
)

// ServerRegistry is what the server provider needs from the overwatch backend
// registry: the read methods of RegistryInterface and the writes for servers
// created through the API.
type ServerRegistry interface {
	RegistryInterface
	RegisterAPIBackend(spec overwatch.APIBackend) error
	Modify(service, address string, port int, fn func(b *overwatch.Backend) error) (*overwatch.Backend, error)
	Deregister(service, address string, port int) error
}

// HealthCheckDefaults are the node-wide validation settings
// (overwatch.validation in the configuration file). They apply to every
// server that has no setting of its own.
type HealthCheckDefaults struct {
	// Enabled is false when the overwatch runs without external validation.
	// No server is probed then, whatever its own health check says.
	Enabled            bool
	Interval           time.Duration
	Timeout            time.Duration
	HealthyThreshold   int
	UnhealthyThreshold int
}

// Limits for a health check set through the API. The lower bounds are those
// of the configuration file (pkg/config); the upper bounds keep a typing
// error from turning a check off in practice.
const (
	minHealthCheckInterval = time.Second
	maxHealthCheckInterval = time.Hour
	minHealthCheckTimeout  = 100 * time.Millisecond
	maxHealthCheckTimeout  = time.Minute
	maxHealthCheckThresh   = 100
	maxServerWeight        = 1000000
)

// RegistryServerProvider implements BackendServerProvider on the overwatch
// backend registry. The registry is the one place a server lives, whether it
// comes from the configuration file, from an agent or from the API, and
// whether the API call was made under /servers or under
// /domains/{name}/backends.
type RegistryServerProvider struct {
	registry    ServerRegistry
	logger      *slog.Logger
	defaults    HealthCheckDefaults
	persistent  bool
	persistence persistence

	// domainExists reports whether a name is a domain that is served. nil
	// accepts every name.
	domainExists func(name string) bool
	// probe asks for an immediate health check of a server. nil does nothing.
	probe func(service, address string, port int)
}

// NewRegistryServerProvider creates a new RegistryServerProvider.
func NewRegistryServerProvider(registry ServerRegistry, defaults HealthCheckDefaults, logger *slog.Logger) *RegistryServerProvider {
	if logger == nil {
		logger = slog.Default()
	}
	if defaults.HealthyThreshold < 1 {
		defaults.HealthyThreshold = 1
	}
	if defaults.UnhealthyThreshold < 1 {
		defaults.UnhealthyThreshold = 1
	}
	return &RegistryServerProvider{
		registry: registry,
		defaults: defaults,
		logger:   logger,
	}
}

// SetPersistent tells the provider whether the registry writes to a store.
// Without one, creating, changing and deleting servers is refused.
func (p *RegistryServerProvider) SetPersistent(persistent bool) {
	p.persistent = persistent
}

// SetStoreUnavailable records why there is no store. The reason is part of
// the 503 that answers every write while the provider is not persistent.
func (p *RegistryServerProvider) SetStoreUnavailable(reason string) {
	p.persistence.unavailable = reason
}

// SetDomainLookup sets the function that says whether a domain exists. A
// server can only be created for a domain that does.
func (p *RegistryServerProvider) SetDomainLookup(exists func(name string) bool) {
	p.domainExists = exists
}

// SetProbe sets the function that starts an immediate health check of a
// server. It is called after a server was created or its check was changed.
func (p *RegistryServerProvider) SetProbe(probe func(service, address string, port int)) {
	p.probe = probe
}

// serverID builds the ID of a server.
func serverID(service, address string, port int) string {
	return fmt.Sprintf("%s:%s:%d", service, address, port)
}

// parseServerID splits "{service}:{address}:{port}". The service ends at the
// first colon and the port starts after the last one, so an IPv6 address in
// the middle keeps its colons.
func parseServerID(id string) (service, address string, port int, err error) {
	first := strings.Index(id, ":")
	last := strings.LastIndex(id, ":")
	if first <= 0 || last <= first+1 || last == len(id)-1 {
		return "", "", 0, fmt.Errorf("server ID %q is not of the form service:address:port", id)
	}
	port, err = strconv.Atoi(id[last+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", "", 0, fmt.Errorf("server ID %q does not end in a port number", id)
	}
	return id[:first], id[first+1 : last], port, nil
}

// ListServers returns every server in the registry, sorted by ID.
func (p *RegistryServerProvider) ListServers() []BackendServer {
	return p.toServers(p.registry.GetAllBackends())
}

// listByService returns the servers of one service (domain), sorted by ID.
func (p *RegistryServerProvider) listByService(service string) []BackendServer {
	return p.toServers(p.registry.GetBackends(service))
}

func (p *RegistryServerProvider) toServers(backends []*overwatch.Backend) []BackendServer {
	servers := make([]BackendServer, 0, len(backends))
	for _, backend := range backends {
		servers = append(servers, p.toServer(backend))
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].ID < servers[j].ID })
	return servers
}

// lookup finds the registry backend behind a server ID.
func (p *RegistryServerProvider) lookup(id string) (*overwatch.Backend, error) {
	service, address, port, err := parseServerID(id)
	if err != nil {
		return nil, errMissing("server %q not found: a server ID has the form service:address:port", id)
	}
	backend, ok := p.registry.GetBackend(service, address, port)
	if !ok {
		return nil, errMissing("server %q not found", id)
	}
	return backend, nil
}

// GetServer returns a specific server by ID (format: service:address:port).
func (p *RegistryServerProvider) GetServer(id string) (*BackendServer, error) {
	backend, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	server := p.toServer(backend)
	return &server, nil
}

// CreateServer registers a server created through the API (source "api").
func (p *RegistryServerProvider) CreateServer(spec ServerSpec) (*BackendServer, error) {
	if !p.persistent {
		return nil, p.persistence.errNoStore()
	}

	if spec.Service == "" {
		return nil, errInvalid("service is required: the name of the domain this server belongs to")
	}
	if strings.ContainsAny(spec.Service, ":/ \t\r\n") {
		return nil, errInvalid("service %q is not a domain name", spec.Service)
	}
	ip := net.ParseIP(spec.Address)
	if ip == nil {
		return nil, errInvalid("address %q is not an IPv4 or IPv6 address", spec.Address)
	}
	if spec.Port < 1 || spec.Port > 65535 {
		return nil, errInvalid("port must be between 1 and 65535 (got %d)", spec.Port)
	}
	if err := validateWeight(spec.Weight); err != nil {
		return nil, err
	}
	weight := spec.Weight
	if weight == 0 {
		weight = 1 // Default weight
	}
	if p.domainExists != nil && !p.domainExists(spec.Service) {
		return nil, errMissing("domain %q not found: create the domain before its servers", spec.Service)
	}

	check, err := p.mergeHealthCheck(nil, "", spec.HealthCheck, true)
	if err != nil {
		return nil, err
	}

	// The canonical text form, so that "::1" and "0:0:0:0:0:0:0:1" are one server.
	address := ip.String()
	if err := p.registry.RegisterAPIBackend(overwatch.APIBackend{
		Service:     spec.Service,
		Address:     address,
		Port:        spec.Port,
		Weight:      weight,
		Region:      spec.Region,
		Priority:    spec.Priority,
		Disabled:    !spec.Enabled,
		HealthCheck: check,
	}); err != nil {
		if errors.Is(err, overwatch.ErrBackendExists) {
			return nil, errConflict("server %q already exists", serverID(spec.Service, address, spec.Port))
		}
		return nil, err
	}
	p.requestProbe(spec.Service, address, spec.Port)

	backend, ok := p.registry.GetBackend(spec.Service, address, spec.Port)
	if !ok {
		return nil, fmt.Errorf("server %q disappeared right after its creation", serverID(spec.Service, address, spec.Port))
	}
	server := p.toServer(backend)
	return &server, nil
}

// UpdateServer changes a server that was created through the API.
func (p *RegistryServerProvider) UpdateServer(id string, update ServerUpdate) (*BackendServer, error) {
	current, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	if !p.persistent {
		return nil, p.persistence.errNoStore()
	}
	if update.Weight != nil {
		if err := validateWeight(*update.Weight); err != nil {
			return nil, err
		}
	}

	checkChanged := false
	backend, err := p.registry.Modify(current.Service, current.Address, current.Port, func(b *overwatch.Backend) error {
		if err := requireAPISource(b, "changed"); err != nil {
			return err
		}
		if update.Weight != nil {
			b.Weight = *update.Weight
		}
		if update.Region != nil {
			b.Region = *update.Region
		}
		if update.Priority != nil {
			b.Priority = *update.Priority
		}
		if update.Enabled != nil {
			b.Disabled = !*update.Enabled
		}
		if update.HealthCheck != nil {
			check, err := p.mergeHealthCheck(b.HealthCheck, b.HealthCheckType, update.HealthCheck, false)
			if err != nil {
				return err
			}
			b.HealthCheck = check
			checkChanged = true
		}
		return nil
	})
	if err != nil {
		return nil, p.registryError(err, id)
	}
	if checkChanged {
		p.requestProbe(backend.Service, backend.Address, backend.Port)
	}

	server := p.toServer(backend)
	return &server, nil
}

// DeleteServer removes a server. A server of the configuration file cannot be
// removed through the API. A server registered by an agent can, but it comes
// back with the agent's next heartbeat for as long as the agent runs.
func (p *RegistryServerProvider) DeleteServer(id string) error {
	backend, err := p.lookup(id)
	if err != nil {
		return err
	}
	if backend.Source == overwatch.SourceStatic {
		return errConflict("server %q is defined in the configuration file and cannot be deleted through the API; remove it from the file and reload", id)
	}
	if backend.Source == overwatch.SourceAPI && !p.persistent {
		return p.persistence.errNoStore()
	}
	if err := p.registry.Deregister(backend.Service, backend.Address, backend.Port); err != nil {
		return p.registryError(err, id)
	}
	return nil
}

// deleteAPIServersOf removes the servers that were created through the API
// for one service. It is used when the domain is deleted.
func (p *RegistryServerProvider) deleteAPIServersOf(service string) {
	for _, backend := range p.registry.GetBackends(service) {
		if backend.Source != overwatch.SourceAPI {
			continue
		}
		if err := p.registry.Deregister(backend.Service, backend.Address, backend.Port); err != nil {
			p.logger.Warn("failed to delete server of deleted domain",
				"domain", service,
				"address", backend.Address,
				"port", backend.Port,
				"error", err,
			)
		}
	}
}

// GetServerHealthCheck returns the health check in effect for a server.
func (p *RegistryServerProvider) GetServerHealthCheck(id string) (*ServerHealthCheck, error) {
	backend, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	check := p.effectiveHealthCheck(backend)
	return &check, nil
}

// UpdateServerHealthCheck changes the health check of a server created
// through the API.
func (p *RegistryServerProvider) UpdateServerHealthCheck(id string, req ServerHealthCheckRequest, replace bool) (*ServerHealthCheck, error) {
	current, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	if !p.persistent {
		return nil, p.persistence.errNoStore()
	}

	backend, err := p.registry.Modify(current.Service, current.Address, current.Port, func(b *overwatch.Backend) error {
		if err := requireAPISource(b, "changed"); err != nil {
			return err
		}
		check, err := p.mergeHealthCheck(b.HealthCheck, b.HealthCheckType, &req, replace)
		if err != nil {
			return err
		}
		b.HealthCheck = check
		return nil
	})
	if err != nil {
		return nil, p.registryError(err, id)
	}
	p.requestProbe(backend.Service, backend.Address, backend.Port)

	check := p.effectiveHealthCheck(backend)
	return &check, nil
}

// requireAPISource refuses a write to a server the API does not own.
func requireAPISource(b *overwatch.Backend, verb string) error {
	id := serverID(b.Service, b.Address, b.Port)
	switch b.Source {
	case overwatch.SourceAPI:
		return nil
	case overwatch.SourceStatic:
		return errConflict("server %q is defined in the configuration file and cannot be %s through the API; change the file and reload. To take it out of DNS now, set an override.", id, verb)
	default:
		return errConflict("server %q is registered by an agent, which reports its weight, region and health; it cannot be %s through the API. To take it out of DNS now, set an override.", id, verb)
	}
}

// registryError maps an error of the registry to an API error.
func (p *RegistryServerProvider) registryError(err error, id string) error {
	if errors.Is(err, overwatch.ErrBackendNotFound) {
		return errMissing("server %q not found", id)
	}
	return err
}

func (p *RegistryServerProvider) requestProbe(service, address string, port int) {
	if p.probe != nil {
		p.probe(service, address, port)
	}
}

func validateWeight(weight int) error {
	if weight < 0 || weight > maxServerWeight {
		return errInvalid("weight must be between 0 and %d (got %d)", maxServerWeight, weight)
	}
	return nil
}

// mergeHealthCheck applies a health check request to the current health check
// of a server and returns the result, or nil when every setting is the
// node-wide one. With replace, fields the request does not set go back to the
// node-wide setting; without it they keep their current value. regionType is
// the check type a static server inherits from its region.
func (p *RegistryServerProvider) mergeHealthCheck(current *overwatch.BackendHealthCheck, regionType string, req *ServerHealthCheckRequest, replace bool) (*overwatch.BackendHealthCheck, error) {
	var check overwatch.BackendHealthCheck
	if !replace && current != nil {
		check = *current
	}
	if req == nil {
		req = &ServerHealthCheckRequest{}
	}

	if req.Enabled != nil {
		check.Disabled = !*req.Enabled
	}
	if req.Type != nil {
		checkType := strings.ToLower(strings.TrimSpace(*req.Type))
		switch checkType {
		case "http", "https", "tcp":
			check.Type = checkType
		default:
			return nil, errInvalid("health_check.type %q: must be http, https or tcp", *req.Type)
		}
	}
	effectiveType := check.Type
	if effectiveType == "" {
		effectiveType = regionType
	}
	if effectiveType == "" {
		effectiveType = "http"
	}

	if req.Path != nil {
		path := *req.Path
		if path != "" && effectiveType == "tcp" {
			return nil, errInvalid("health_check.path applies to http and https checks only; this check is of type tcp")
		}
		if path != "" && !strings.HasPrefix(path, "/") {
			return nil, errInvalid("health_check.path %q must start with \"/\"", path)
		}
		if strings.ContainsAny(path, " \t\r\n") {
			return nil, errInvalid("health_check.path %q must not contain white space", path)
		}
		check.Path = path
	}
	if req.Host != nil {
		host := strings.TrimSpace(*req.Host)
		if host != "" && effectiveType == "tcp" {
			return nil, errInvalid("health_check.host applies to http and https checks only; this check is of type tcp")
		}
		if strings.ContainsAny(host, "/ \t\r\n") {
			return nil, errInvalid("health_check.host %q is not a host name", host)
		}
		check.Host = host
	}
	if effectiveType == "tcp" {
		// A check that was switched to tcp has no use for its former path.
		check.Path = ""
		check.Host = ""
	}

	if req.Interval != nil {
		interval := time.Duration(*req.Interval)
		if interval < minHealthCheckInterval || interval > maxHealthCheckInterval {
			return nil, errInvalid("health_check.interval must be between %s and %s (got %s)",
				Duration(minHealthCheckInterval), Duration(maxHealthCheckInterval), *req.Interval)
		}
		check.Interval = interval
	}
	if req.Timeout != nil {
		timeout := time.Duration(*req.Timeout)
		if timeout < minHealthCheckTimeout || timeout > maxHealthCheckTimeout {
			return nil, errInvalid("health_check.timeout must be between %s and %s (got %s)",
				Duration(minHealthCheckTimeout), Duration(maxHealthCheckTimeout), *req.Timeout)
		}
		check.Timeout = timeout
	}
	if req.Interval != nil || req.Timeout != nil {
		// A check that may run longer than the time to the next one is a
		// typing error (units swapped, or the two values swapped). Each value
		// is the server's own when set, the node-wide one otherwise.
		interval, intervalFrom := check.Interval, "health_check.interval"
		if interval == 0 {
			interval, intervalFrom = p.defaults.Interval, "the node-wide interval"
		}
		timeout, timeoutFrom := check.Timeout, "health_check.timeout"
		if timeout == 0 {
			timeout, timeoutFrom = p.defaults.Timeout, "the node-wide timeout"
		}
		if interval > 0 && timeout > interval {
			return nil, errInvalid("%s (%s) is longer than %s (%s); a check must be able to finish before the next one is due. Send both health_check.interval and health_check.timeout.",
				timeoutFrom, Duration(timeout), intervalFrom, Duration(interval))
		}
	}
	if req.HealthyThreshold != nil {
		if *req.HealthyThreshold < 1 || *req.HealthyThreshold > maxHealthCheckThresh {
			return nil, errInvalid("health_check.healthy_threshold must be between 1 and %d (got %d)", maxHealthCheckThresh, *req.HealthyThreshold)
		}
		check.HealthyThreshold = *req.HealthyThreshold
	}
	if req.UnhealthyThreshold != nil {
		if *req.UnhealthyThreshold < 1 || *req.UnhealthyThreshold > maxHealthCheckThresh {
			return nil, errInvalid("health_check.unhealthy_threshold must be between 1 and %d (got %d)", maxHealthCheckThresh, *req.UnhealthyThreshold)
		}
		check.UnhealthyThreshold = *req.UnhealthyThreshold
	}

	if check == (overwatch.BackendHealthCheck{}) {
		return nil, nil
	}
	// A check of its own on a node that probes nothing would be accepted and
	// never run. Turning the check off is the one setting that still holds.
	if !p.defaults.Enabled && !check.Disabled {
		return nil, errConflict("external validation is turned off on this overwatch (overwatch.validation.enabled: false), so a health check cannot be applied to a server; leave health_check out or enable validation")
	}
	return &check, nil
}

// effectiveHealthCheck returns the check the validator runs for a backend.
func (p *RegistryServerProvider) effectiveHealthCheck(b *overwatch.Backend) ServerHealthCheck {
	check := ServerHealthCheck{
		Enabled:            p.defaults.Enabled,
		Type:               b.CheckType(),
		Interval:           Duration(p.defaults.Interval),
		Timeout:            Duration(p.defaults.Timeout),
		HealthyThreshold:   p.defaults.HealthyThreshold,
		UnhealthyThreshold: p.defaults.UnhealthyThreshold,
	}
	if hc := b.HealthCheck; hc != nil {
		if hc.Disabled {
			check.Enabled = false
		}
		if hc.Interval > 0 {
			check.Interval = Duration(hc.Interval)
		}
		if hc.Timeout > 0 {
			check.Timeout = Duration(hc.Timeout)
		}
		if hc.HealthyThreshold > 0 {
			check.HealthyThreshold = hc.HealthyThreshold
		}
		if hc.UnhealthyThreshold > 0 {
			check.UnhealthyThreshold = hc.UnhealthyThreshold
		}
		if check.Type != "tcp" {
			check.Path = hc.Path
			check.Host = hc.Host
		}
	}
	if check.Type != "tcp" && check.Path == "" {
		check.Path = "/"
	}
	return check
}

// toServer converts a registry backend to its API representation.
func (p *RegistryServerProvider) toServer(backend *overwatch.Backend) BackendServer {
	agentHealthy := "unhealthy"
	if backend.AgentHealthy {
		agentHealthy = "healthy"
	}

	return BackendServer{
		ID:       serverID(backend.Service, backend.Address, backend.Port),
		Service:  backend.Service,
		Name:     backend.Service,
		Address:  backend.Address,
		Port:     backend.Port,
		Protocol: "tcp",
		Weight:   backend.Weight,
		Priority: backend.Priority,
		Region:   backend.Region,
		Enabled:  !backend.Disabled,
		Healthy:  backend.EffectiveStatus == overwatch.StatusHealthy,
		Source:   string(backend.Source),
		Metadata: map[string]string{
			"service":          backend.Service,
			"source":           string(backend.Source),
			"effective_status": string(backend.EffectiveStatus),
			"agent_healthy":    agentHealthy,
			"agent_last_seen":  backend.AgentLastSeen.Format(time.RFC3339),
			"validation_check": backend.ValidationLastCheck.Format(time.RFC3339),
			"smoothed_latency": backend.SmoothedLatency.String(),
		},
		CreatedAt:   backend.CreatedAt,
		UpdatedAt:   backend.UpdatedAt,
		LastCheck:   backend.ValidationLastCheck,
		HealthCheck: p.effectiveHealthCheck(backend),
	}
}
