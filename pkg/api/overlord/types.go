// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package overlord provides the REST API endpoints for the Overlord dashboard.
// These endpoints power the comprehensive management UI for OpenGSLB.
package overlord

import (
	"time"
)

// ============================================================================
// Common Types
// ============================================================================

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// SuccessResponse represents a successful operation response.
type SuccessResponse struct {
	Success bool `json:"success"`
}

// ============================================================================
// Domain Types
// ============================================================================

// Domain represents a GSLB-managed domain.
type Domain struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	RoutingAlgorithm string    `json:"routingAlgorithm"`
	Regions          []string  `json:"regions"`
	TTL              int       `json:"ttl"`
	HealthyBackends  int       `json:"healthyBackends"`
	TotalBackends    int       `json:"totalBackends"`
	Description      string    `json:"description,omitempty"`
	Enabled          bool      `json:"enabled"`
	Service          string    `json:"service,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// DomainCreateRequest is the request body for creating a domain.
type DomainCreateRequest struct {
	Name             string   `json:"name"`
	RoutingAlgorithm string   `json:"routingAlgorithm"`
	Regions          []string `json:"regions"`
	TTL              int      `json:"ttl"`
	Description      string   `json:"description,omitempty"`
	Enabled          bool     `json:"enabled"`
	Service          string   `json:"service,omitempty"`
}

// DomainUpdateRequest is the request body for updating a domain.
type DomainUpdateRequest struct {
	RoutingAlgorithm string   `json:"routingAlgorithm,omitempty"`
	Regions          []string `json:"regions,omitempty"`
	TTL              int      `json:"ttl,omitempty"`
	Description      string   `json:"description,omitempty"`
	Enabled          *bool    `json:"enabled,omitempty"`
}

// DomainsResponse is the response for GET /api/domains.
type DomainsResponse struct {
	Domains []Domain `json:"domains"`
}

// DomainResponse is the response for single domain operations.
type DomainResponse struct {
	Domain Domain `json:"domain"`
}

// ============================================================================
// Server/Backend Types
// ============================================================================

// HealthCheckConfig represents health check configuration for a server.
type HealthCheckConfig struct {
	Enabled            bool   `json:"enabled"`
	Type               string `json:"type"`
	Path               string `json:"path,omitempty"`
	Interval           int    `json:"interval"`
	Timeout            int    `json:"timeout"`
	HealthyThreshold   int    `json:"healthyThreshold"`
	UnhealthyThreshold int    `json:"unhealthyThreshold"`
}

// Backend represents a backend server.
type Backend struct {
	ID                  string            `json:"id"`
	Service             string            `json:"service"`
	Address             string            `json:"address"`
	Port                int               `json:"port"`
	Weight              int               `json:"weight"`
	Region              string            `json:"region"`
	AgentID             string            `json:"agentId,omitempty"`
	EffectiveStatus     string            `json:"effectiveStatus"`
	AgentHealthy        bool              `json:"agentHealthy"`
	AgentLastSeen       *time.Time        `json:"agentLastSeen,omitempty"`
	ValidationHealthy   *bool             `json:"validationHealthy,omitempty"`
	ValidationLastCheck *time.Time        `json:"validationLastCheck,omitempty"`
	ValidationError     string            `json:"validationError,omitempty"`
	SmoothedLatency     int               `json:"smoothedLatency,omitempty"`
	LatencySamples      int               `json:"latencySamples,omitempty"`
	HealthCheck         HealthCheckConfig `json:"healthCheck,omitempty"`
}

// ServerCreateRequest is the request body for creating a server.
type ServerCreateRequest struct {
	Service     string            `json:"service"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Weight      int               `json:"weight"`
	Region      string            `json:"region"`
	AgentID     string            `json:"agentId,omitempty"`
	HealthCheck HealthCheckConfig `json:"healthCheck,omitempty"`
}

// ServerUpdateRequest is the request body for updating a server.
type ServerUpdateRequest struct {
	Service     string             `json:"service,omitempty"`
	Address     string             `json:"address,omitempty"`
	Port        int                `json:"port,omitempty"`
	Weight      int                `json:"weight,omitempty"`
	Region      string             `json:"region,omitempty"`
	AgentID     string             `json:"agentId,omitempty"`
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`
}

// ServersResponse is the response for GET /api/servers.
type ServersResponse struct {
	Servers []Backend `json:"servers"`
}

// ServerResponse is the response for single server operations.
type ServerResponse struct {
	Server Backend `json:"server"`
}

// ============================================================================
// Region Types
// ============================================================================

// Region represents a geographic region.
type Region struct {
	Name        string            `json:"name"`
	Countries   []string          `json:"countries,omitempty"`
	Continents  []string          `json:"continents,omitempty"`
	Servers     []string          `json:"servers,omitempty"`
	HealthCheck HealthCheckConfig `json:"healthCheck,omitempty"`
}

// RegionHealthSummary represents health summary for a region.
type RegionHealthSummary struct {
	Region          string  `json:"region"`
	TotalBackends   int     `json:"totalBackends"`
	HealthyBackends int     `json:"healthyBackends"`
	HealthPercent   float64 `json:"healthPercent"`
	AvgLatency      float64 `json:"avgLatency,omitempty"`
}

// RegionsResponse is the response for GET /api/regions.
type RegionsResponse struct {
	Regions []Region `json:"regions"`
}

// RegionResponse is the response for single region operations.
type RegionResponse struct {
	Region Region `json:"region"`
}

// RegionHealthResponse is the response for GET /api/regions/:name/health.
type RegionHealthResponse struct {
	HealthSummary RegionHealthSummary `json:"healthSummary"`
}

// ============================================================================
// Overwatch Node Types
// ============================================================================

// OverwatchNode represents an Overwatch node in the cluster.
type OverwatchNode struct {
	NodeID             string    `json:"nodeId"`
	Region             string    `json:"region"`
	Status             string    `json:"status"`
	LastSeen           time.Time `json:"lastSeen"`
	Version            string    `json:"version,omitempty"`
	Address            string    `json:"address"`
	BackendsManaged    int       `json:"backendsManaged"`
	DNSQueriesPerSec   float64   `json:"dnsQueriesPerSecond,omitempty"`
	Uptime             int64     `json:"uptime,omitempty"`
}

// OverwatchNodesResponse is the response for GET /api/nodes/overwatch.
type OverwatchNodesResponse struct {
	Nodes []OverwatchNode `json:"nodes"`
}

// OverwatchNodeResponse is the response for single overwatch node operations.
type OverwatchNodeResponse struct {
	Node OverwatchNode `json:"node"`
}

// ============================================================================
// Agent Node Types
// ============================================================================

// Certificate represents an agent certificate.
type Certificate struct {
	Fingerprint   string     `json:"fingerprint"`
	Subject       string     `json:"subject,omitempty"`
	Issuer        string     `json:"issuer,omitempty"`
	SerialNumber  string     `json:"serialNumber,omitempty"`
	NotBefore     *time.Time `json:"notBefore,omitempty"`
	NotAfter      *time.Time `json:"notAfter,omitempty"`
	Revoked       bool       `json:"revoked"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	RevokedReason string     `json:"revokedReason,omitempty"`
}

// AgentNode represents an agent node.
type AgentNode struct {
	AgentID           string       `json:"agentId"`
	Region            string       `json:"region"`
	Service           string       `json:"service,omitempty"`
	Status            string       `json:"status"`
	LastSeen          time.Time    `json:"lastSeen"`
	Version           string       `json:"version,omitempty"`
	Address           string       `json:"address,omitempty"`
	BackendsReporting int          `json:"backendsReporting"`
	Fingerprint       string       `json:"fingerprint,omitempty"`
	Certificate       *Certificate `json:"certificate,omitempty"`
}

// AgentNodesResponse is the response for GET /api/nodes/agent.
type AgentNodesResponse struct {
	Agents []AgentNode `json:"agents"`
}

// AgentNodeResponse is the response for single agent node operations.
type AgentNodeResponse struct {
	Agent AgentNode `json:"agent"`
}

// CertificateResponse is the response for certificate operations.
type CertificateResponse struct {
	Certificate Certificate `json:"certificate"`
}

// CertificateActionRequest is the request for certificate actions.
type CertificateActionRequest struct {
	Action string `json:"action"` // "rotate" or "revoke"
	Reason string `json:"reason,omitempty"`
}

// ============================================================================
// Override Types
// ============================================================================

// Override represents a backend health override.
type Override struct {
	ID        string    `json:"id"`
	Service   string    `json:"service"`
	Address   string    `json:"address"`
	Healthy   bool      `json:"healthy"`
	Reason    string    `json:"reason"`
	Source    string    `json:"source"`
	Authority string    `json:"authority"`
	CreatedAt time.Time `json:"createdAt"`
}

// OverrideCreateRequest is the request body for creating an override.
type OverrideCreateRequest struct {
	Service   string `json:"service"`
	Address   string `json:"address"`
	Healthy   bool   `json:"healthy"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
	Authority string `json:"authority"`
}

// OverridesResponse is the response for GET /api/overrides.
type OverridesResponse struct {
	Overrides []Override `json:"overrides"`
}

// OverrideResponse is the response for single override operations.
type OverrideResponse struct {
	Override Override `json:"override"`
}

// ============================================================================
// Health Validation Types
// ============================================================================

// ValidationRequest is the request body for POST /api/health/validate.
type ValidationRequest struct {
	Scope    string   `json:"scope"`    // "all", "unhealthy", "selected"
	Backends []string `json:"backends"` // List of backend IDs
	Service  string   `json:"service,omitempty"`
	Region   string   `json:"region,omitempty"`
}

// ValidationStartResponse is the response for starting a validation.
type ValidationStartResponse struct {
	ValidationID string `json:"validationId"`
	Status       string `json:"status"`
}

// ValidationResult represents a single backend validation result.
type ValidationResult struct {
	BackendID string  `json:"backendId"`
	Healthy   bool    `json:"healthy"`
	Latency   int     `json:"latency,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// ValidationStatus is the response for GET /api/health/validation/:id.
type ValidationStatus struct {
	ValidationID        string             `json:"validationId"`
	Status              string             `json:"status"`
	BackendsValidated   int                `json:"backendsValidated"`
	ValidationsPassed   int                `json:"validationsPassed"`
	ValidationsFailed   int                `json:"validationsFailed"`
	Duration            int                `json:"duration,omitempty"`
	Results             []ValidationResult `json:"results,omitempty"`
}

// HealthStatusResponse is the response for GET /api/health/status.
type HealthStatusResponse struct {
	Backends         []Backend        `json:"backends"`
	ValidationStatus ValidationStatus `json:"validationStatus,omitempty"`
}

// HealthStatusUpdateRequest is the request for updating backend health status.
type HealthStatusUpdateRequest struct {
	Status string `json:"status"` // "healthy", "unhealthy", "stale"
}

// ============================================================================
// Gossip Types
// ============================================================================

// GossipNode represents a node in the gossip cluster.
type GossipNode struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Address           string    `json:"address"`
	Port              int       `json:"port"`
	Role              string    `json:"role"` // "overwatch" or "agent"
	Region            string    `json:"region"`
	Status            string    `json:"status"` // "healthy", "suspect", "dead"
	EncryptionEnabled bool      `json:"encryptionEnabled"`
	LastSeen          time.Time `json:"lastSeen"`
	MessagesReceived  int64     `json:"messagesReceived"`
	MessagesSent      int64     `json:"messagesSent"`
	Peers             int       `json:"peers"`
}

// GossipConfig represents gossip configuration.
type GossipConfig struct {
	ID                string `json:"id"`
	BindAddress       string `json:"bindAddress"`
	BindPort          int    `json:"bindPort"`
	ProbeInterval     int    `json:"probeInterval"`
	ProbeTimeout      int    `json:"probeTimeout"`
	GossipInterval    int    `json:"gossipInterval"`
	EncryptionKey     string `json:"encryptionKey,omitempty"`
	EncryptionEnabled bool   `json:"encryptionEnabled"`
}

// GossipNodesResponse is the response for GET /api/gossip/nodes.
type GossipNodesResponse struct {
	Nodes []GossipNode `json:"nodes"`
}

// GossipNodeResponse is the response for single gossip node operations.
type GossipNodeResponse struct {
	Node GossipNode `json:"node"`
}

// GossipConfigResponse is the response for GET /api/gossip/config.
type GossipConfigResponse struct {
	Config GossipConfig `json:"config"`
}

// GenerateKeyResponse is the response for POST /api/gossip/config/generate-key.
type GenerateKeyResponse struct {
	EncryptionKey string `json:"encryptionKey"`
}

// ============================================================================
// Geo Mapping Types
// ============================================================================

// GeoMapping represents a CIDR-to-region mapping.
type GeoMapping struct {
	ID      string `json:"id"`
	CIDR    string `json:"cidr"`
	Region  string `json:"region"`
	Comment string `json:"comment,omitempty"`
}

// GeoMappingsResponse is the response for GET /api/geo-mappings.
type GeoMappingsResponse struct {
	Mappings []GeoMapping `json:"mappings"`
}

// GeoMappingResponse is the response for single geo mapping operations.
type GeoMappingResponse struct {
	Mapping GeoMapping `json:"mapping"`
}

// GeoConfig represents geolocation configuration.
type GeoConfig struct {
	DatabasePath  string `json:"databasePath,omitempty"`
	DefaultRegion string `json:"defaultRegion"`
	ECSEnabled    bool   `json:"ecsEnabled"`
}

// GeoConfigResponse is the response for GET /api/geolocation/config.
type GeoConfigResponse struct {
	Config GeoConfig `json:"config"`
}

// GeoLookupRequest is the request for POST /api/geolocation/lookup.
type GeoLookupRequest struct {
	IP string `json:"ip"`
}

// GeoLookupResponse is the response for POST /api/geolocation/lookup.
type GeoLookupResponse struct {
	Region  string `json:"region"`
	Country string `json:"country,omitempty"`
	City    string `json:"city,omitempty"`
}

// ============================================================================
// DNSSEC Types
// ============================================================================

// DNSKey represents a DNSSEC key.
type DNSKey struct {
	ID          string     `json:"id"`
	Zone        string     `json:"zone"`
	Algorithm   string     `json:"algorithm"`
	KeyTag      uint16     `json:"keyTag"`
	PublicKey   string     `json:"publicKey,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ActivatedAt *time.Time `json:"activatedAt,omitempty"`
	RetiredAt   *time.Time `json:"retiredAt,omitempty"`
	KeyType     string     `json:"keyType"` // "KSK" or "ZSK"
}

// DSRecord represents a DNSSEC DS record.
type DSRecord struct {
	KeyTag     uint16 `json:"keyTag"`
	Algorithm  uint8  `json:"algorithm"`
	DigestType uint8  `json:"digestType"`
	Digest     string `json:"digest"`
}

// DNSSECStatus is the response for GET /api/dnssec/status.
type DNSSECStatus struct {
	Enabled    bool       `json:"enabled"`
	Keys       []DNSKey   `json:"keys"`
	DSRecords  []DSRecord `json:"dsRecords"`
	SyncStatus string     `json:"syncStatus,omitempty"`
}

// DNSKeyGenerateRequest is the request for POST /api/dnssec/keys/generate.
type DNSKeyGenerateRequest struct {
	Zone      string `json:"zone"`
	Algorithm string `json:"algorithm"`
}

// DNSKeyResponse is the response for DNSSEC key operations.
type DNSKeyResponse struct {
	Key DNSKey `json:"key"`
}

// DNSSECSyncResponse is the response for POST /api/dnssec/sync.
type DNSSECSyncResponse struct {
	Status     string     `json:"status"`
	LastSync   *time.Time `json:"lastSync,omitempty"`
	SyncStatus string     `json:"syncStatus,omitempty"`
}

// ============================================================================
// Audit Log Types
// ============================================================================

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID          int64                  `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	User        string                 `json:"user"`
	Action      string                 `json:"action"` // "CREATE", "UPDATE", "DELETE"
	Category    string                 `json:"category"` // "domain", "server", "override", etc.
	Resource    string                 `json:"resource"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Severity    string                 `json:"severity"` // "info", "warning", "error", "success"
	IPAddress   string                 `json:"ipAddress,omitempty"`
}

// AuditLogsResponse is the response for GET /api/audit-logs.
type AuditLogsResponse struct {
	Logs  []AuditLog `json:"logs"`
	Total int        `json:"total"`
}

// AuditLogResponse is the response for GET /api/audit-logs/:id.
type AuditLogResponse struct {
	Log AuditLog `json:"log"`
}

// AuditLogsStats is the response for GET /api/audit-logs/stats.
type AuditLogsStats struct {
	Last24h    int            `json:"last24h"`
	LastHour   int            `json:"lastHour"`
	Warnings   int            `json:"warnings"`
	Total      int            `json:"total"`
	ByCategory map[string]int `json:"byCategory"`
	ByAction   map[string]int `json:"byAction"`
}

// ============================================================================
// Metrics Types
// ============================================================================

// SystemStats represents system-wide statistics.
type SystemStats struct {
	TotalBackends       int     `json:"totalBackends"`
	HealthyBackends     int     `json:"healthyBackends"`
	UnhealthyBackends   int     `json:"unhealthyBackends"`
	StaleBackends       int     `json:"staleBackends"`
	ActiveOverrides     int     `json:"activeOverrides"`
	TotalAgents         int     `json:"totalAgents"`
	ActiveAgents        int     `json:"activeAgents"`
	TotalOverwatches    int     `json:"totalOverwatches"`
	ActiveOverwatches   int     `json:"activeOverwatches"`
	TotalDomains        int     `json:"totalDomains"`
	TotalRegions        int     `json:"totalRegions"`
	DNSQueriesLast24h   int64   `json:"dnsQueriesLast24h"`
	AvgLatencyMs        float64 `json:"avgLatencyMs"`
}

// MetricsOverviewResponse is the response for GET /api/metrics/overview.
type MetricsOverviewResponse struct {
	SystemStats SystemStats `json:"systemStats"`
}

// MetricDataPoint represents a single metrics data point.
type MetricDataPoint struct {
	Timestamp              time.Time      `json:"timestamp"`
	Hour                   string         `json:"hour,omitempty"`
	Queries                int64          `json:"queries"`
	LatencyP50             float64        `json:"latencyP50,omitempty"`
	LatencyP95             float64        `json:"latencyP95,omitempty"`
	LatencyP99             float64        `json:"latencyP99,omitempty"`
	HealthCheckSuccessRate float64        `json:"healthCheckSuccessRate,omitempty"`
	RoutingDecisions       map[string]int `json:"routingDecisions,omitempty"`
}

// MetricsHistoryResponse is the response for GET /api/metrics/history.
type MetricsHistoryResponse struct {
	Metrics []MetricDataPoint `json:"metrics"`
}

// NodeMetrics represents metrics for a single node.
type NodeMetrics struct {
	Queries    int64   `json:"queries"`
	LatencyAvg float64 `json:"latencyAvg"`
	Healthy    bool    `json:"healthy"`
}

// PerNodeMetrics represents per-node metrics at a point in time.
type PerNodeMetrics struct {
	Timestamp time.Time              `json:"timestamp"`
	PerNode   map[string]NodeMetrics `json:"perNode"`
}

// MetricsPerNodeResponse is the response for GET /api/metrics/per-node.
type MetricsPerNodeResponse struct {
	Metrics []PerNodeMetrics `json:"metrics"`
}

// RegionMetrics represents metrics for a single region.
type RegionMetrics struct {
	Queries       int64   `json:"queries"`
	LatencyAvg    float64 `json:"latencyAvg"`
	HealthPercent float64 `json:"healthPercent"`
}

// PerRegionMetrics represents per-region metrics at a point in time.
type PerRegionMetrics struct {
	Timestamp time.Time                `json:"timestamp"`
	PerRegion map[string]RegionMetrics `json:"perRegion"`
}

// MetricsPerRegionResponse is the response for GET /api/metrics/per-region.
type MetricsPerRegionResponse struct {
	Metrics []PerRegionMetrics `json:"metrics"`
}

// HealthSummaryItem represents health summary for a region.
type HealthSummaryItem struct {
	Region          string  `json:"region"`
	TotalBackends   int     `json:"totalBackends"`
	HealthyBackends int     `json:"healthyBackends"`
	HealthPercent   float64 `json:"healthPercent"`
	AvgLatency      float64 `json:"avgLatency,omitempty"`
}

// MetricsHealthSummaryResponse is the response for GET /api/metrics/health-summary.
type MetricsHealthSummaryResponse struct {
	Regions []HealthSummaryItem `json:"regions"`
}

// RoutingDistributionItem represents routing distribution data.
type RoutingDistributionItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Color string `json:"color,omitempty"`
}

// MetricsRoutingDistributionResponse is the response for GET /api/metrics/routing-distribution.
type MetricsRoutingDistributionResponse struct {
	Distribution []RoutingDistributionItem `json:"distribution"`
}

// RoutingFlow represents a routing flow between regions.
type RoutingFlow struct {
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	Value         int    `json:"value"`
	IsCrossRegion bool   `json:"isCrossRegion"`
}

// MetricsRoutingFlowsResponse is the response for GET /api/metrics/routing-flows.
type MetricsRoutingFlowsResponse struct {
	Flows []RoutingFlow `json:"flows"`
}

// RoutingDestination represents a routing destination.
type RoutingDestination struct {
	Region        string  `json:"region"`
	Count         int     `json:"count"`
	Percentage    float64 `json:"percentage"`
	IsCrossRegion bool    `json:"isCrossRegion"`
}

// RoutingDecision represents routing decisions from a source region.
type RoutingDecision struct {
	SourceRegion  string               `json:"sourceRegion"`
	TotalRequests int                  `json:"totalRequests"`
	Destinations  []RoutingDestination `json:"destinations"`
}

// MetricsRoutingDecisionsResponse is the response for GET /api/metrics/routing-decisions.
type MetricsRoutingDecisionsResponse struct {
	Decisions []RoutingDecision `json:"decisions"`
}

// ============================================================================
// Configuration Types
// ============================================================================

// Preferences represents user preferences.
type Preferences struct {
	Theme         string `json:"theme,omitempty"`
	Language      string `json:"language,omitempty"`
	DefaultTTL    int    `json:"defaultTTL,omitempty"`
	AutoRefresh   bool   `json:"autoRefresh"`
	LogsRetention int    `json:"logsRetention,omitempty"`
}

// PreferencesResponse is the response for GET /api/preferences.
type PreferencesResponse struct {
	Preferences Preferences `json:"preferences"`
}

// APISettings represents API server configuration.
type APISettings struct {
	Enabled           bool     `json:"enabled"`
	Address           string   `json:"address"`
	AllowedNetworks   []string `json:"allowedNetworks,omitempty"`
	TrustProxyHeaders bool     `json:"trustProxyHeaders"`
}

// APISettingsResponse is the response for GET /api/config/api-settings.
type APISettingsResponse struct {
	Config APISettings `json:"config"`
}

// ValidationConfigResp represents validation configuration.
type ValidationConfigResp struct {
	Enabled       bool `json:"enabled"`
	CheckInterval int  `json:"checkInterval"`
	CheckTimeout  int  `json:"checkTimeout"`
}

// ValidationConfigResponse is the response for GET /api/config/validation.
type ValidationConfigResponse struct {
	Validation ValidationConfigResp `json:"validation"`
}

// StaleConfigResp represents stale handling configuration.
type StaleConfigResp struct {
	Threshold   int `json:"threshold"`
	RemoveAfter int `json:"removeAfter"`
}

// StaleConfigResponse is the response for GET /api/config/stale-handling.
type StaleConfigResponse struct {
	StaleConfig StaleConfigResp `json:"staleConfig"`
}

// ============================================================================
// Routing Types
// ============================================================================

// RoutingAlgorithm represents a routing algorithm.
type RoutingAlgorithm struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// RoutingAlgorithmsResponse is the response for GET /api/routing/algorithms.
type RoutingAlgorithmsResponse struct {
	Algorithms []RoutingAlgorithm `json:"algorithms"`
}

// RoutingTestRequest is the request for POST /api/routing/test.
type RoutingTestRequest struct {
	Domain       string `json:"domain"`
	ClientIP     string `json:"clientIp,omitempty"`
	SourceRegion string `json:"sourceRegion,omitempty"`
}

// RoutingTestResponse is the response for POST /api/routing/test.
type RoutingTestResponse struct {
	SelectedBackend string `json:"selectedBackend"`
	Algorithm       string `json:"algorithm"`
	Reasoning       string `json:"reasoning,omitempty"`
}

// ============================================================================
// Health & System Types
// ============================================================================

// HealthCheckResponse is the response for GET /api/health.
type HealthCheckResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  int64  `json:"uptime"`
}
