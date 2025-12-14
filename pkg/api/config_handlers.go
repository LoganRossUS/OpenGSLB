// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ConfigProvider defines the interface for configuration operations.
type ConfigProvider interface {
	// GetPreferences returns user preferences.
	GetPreferences() (*Preferences, error)
	// UpdatePreferences updates user preferences.
	UpdatePreferences(prefs Preferences) error

	// GetSystemConfig returns the system configuration.
	GetSystemConfig() (*SystemConfig, error)

	// GetDNSConfig returns DNS-specific configuration.
	GetDNSConfig() (*DNSConfig, error)
	// UpdateDNSConfig updates DNS configuration.
	UpdateDNSConfig(config DNSConfig) error

	// GetHealthCheckConfig returns health check configuration.
	GetHealthCheckConfig() (*HealthCheckConfig, error)
	// UpdateHealthCheckConfig updates health check configuration.
	UpdateHealthCheckConfig(config HealthCheckConfig) error

	// GetLoggingConfig returns logging configuration.
	GetLoggingConfig() (*LoggingConfig, error)
	// UpdateLoggingConfig updates logging configuration.
	UpdateLoggingConfig(config LoggingConfig) error
}

// Preferences represents user preferences for the dashboard.
type Preferences struct {
	Theme                string            `json:"theme"` // light, dark, auto
	Language             string            `json:"language"`
	Timezone             string            `json:"timezone"`
	DateFormat           string            `json:"date_format"`
	TimeFormat           string            `json:"time_format"`
	RefreshInterval      int               `json:"refresh_interval_seconds"`
	NotificationsEnabled bool              `json:"notifications_enabled"`
	DefaultView          string            `json:"default_view"`
	CustomSettings       map[string]string `json:"custom_settings,omitempty"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// SystemConfig represents the overall system configuration.
type SystemConfig struct {
	Version    string    `json:"version"`
	BuildInfo  BuildInfo `json:"build_info"`
	Mode       string    `json:"mode"` // overwatch, agent
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	Region     string    `json:"region"`
	Datacenter string    `json:"datacenter,omitempty"`
	Features   []string  `json:"features"`
	StartTime  time.Time `json:"start_time"`
	ConfigPath string    `json:"config_path,omitempty"`
	DataDir    string    `json:"data_dir,omitempty"`
}

// BuildInfo contains build information.
type BuildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// DNSConfig represents DNS server configuration.
type DNSConfig struct {
	ListenAddress    string   `json:"listen_address"`
	ListenPort       int      `json:"listen_port"`
	EnableTCP        bool     `json:"enable_tcp"`
	EnableUDP        bool     `json:"enable_udp"`
	EnableDOT        bool     `json:"enable_dot"`
	EnableDOH        bool     `json:"enable_doh"`
	DOTPort          int      `json:"dot_port,omitempty"`
	DOHPort          int      `json:"doh_port,omitempty"`
	MaxUDPSize       int      `json:"max_udp_size"`
	DefaultTTL       int      `json:"default_ttl"`
	MinTTL           int      `json:"min_ttl"`
	MaxTTL           int      `json:"max_ttl"`
	NegativeTTL      int      `json:"negative_ttl"`
	CacheEnabled     bool     `json:"cache_enabled"`
	CacheSize        int      `json:"cache_size"`
	RecursionEnabled bool     `json:"recursion_enabled"`
	Forwarders       []string `json:"forwarders,omitempty"`
	RateLimitEnabled bool     `json:"rate_limit_enabled"`
	RateLimitQPS     int      `json:"rate_limit_qps"`
}

// HealthCheckConfig represents health check configuration.
type HealthCheckConfig struct {
	DefaultInterval     int    `json:"default_interval_seconds"`
	DefaultTimeout      int    `json:"default_timeout_seconds"`
	HealthyThreshold    int    `json:"healthy_threshold"`
	UnhealthyThreshold  int    `json:"unhealthy_threshold"`
	MaxConcurrentChecks int    `json:"max_concurrent_checks"`
	EnablePassiveChecks bool   `json:"enable_passive_checks"`
	DefaultProtocol     string `json:"default_protocol"`
	TCPConnectTimeout   int    `json:"tcp_connect_timeout_ms"`
	HTTPFollowRedirects bool   `json:"http_follow_redirects"`
	HTTPMaxRedirects    int    `json:"http_max_redirects"`
	RetryEnabled        bool   `json:"retry_enabled"`
	RetryCount          int    `json:"retry_count"`
	RetryDelay          int    `json:"retry_delay_ms"`
}

// LoggingConfig represents logging configuration.
type LoggingConfig struct {
	Level          string `json:"level"`  // debug, info, warn, error
	Format         string `json:"format"` // json, text
	Output         string `json:"output"` // stdout, file, syslog
	FilePath       string `json:"file_path,omitempty"`
	MaxSize        int    `json:"max_size_mb,omitempty"`
	MaxBackups     int    `json:"max_backups,omitempty"`
	MaxAge         int    `json:"max_age_days,omitempty"`
	Compress       bool   `json:"compress,omitempty"`
	EnableAudit    bool   `json:"enable_audit"`
	AuditRetention int    `json:"audit_retention_days"`
}

// PreferencesResponse is the response for preferences operations.
type PreferencesResponse struct {
	Preferences Preferences `json:"preferences"`
}

// SystemConfigResponse is the response for GET /api/v1/config/system.
type SystemConfigResponse struct {
	Config SystemConfig `json:"config"`
}

// DNSConfigResponse is the response for DNS config operations.
type DNSConfigResponse struct {
	Config DNSConfig `json:"config"`
}

// HealthCheckConfigResponse is the response for health check config operations.
type HealthCheckConfigResponse struct {
	Config HealthCheckConfig `json:"config"`
}

// LoggingConfigResponse is the response for logging config operations.
type LoggingConfigResponse struct {
	Config LoggingConfig `json:"config"`
}

// ConfigHandlers provides HTTP handlers for configuration API endpoints.
type ConfigHandlers struct {
	provider ConfigProvider
	logger   *slog.Logger
}

// NewConfigHandlers creates a new ConfigHandlers instance.
func NewConfigHandlers(provider ConfigProvider, logger *slog.Logger) *ConfigHandlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConfigHandlers{
		provider: provider,
		logger:   logger,
	}
}

// HandlePreferences handles /api/v1/preferences requests.
func (h *ConfigHandlers) HandlePreferences(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "config provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		prefs, err := h.provider.GetPreferences()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "failed to get preferences: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, PreferencesResponse{Preferences: *prefs})

	case http.MethodPut, http.MethodPatch:
		var prefs Preferences
		if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		prefs.UpdatedAt = time.Now().UTC()

		if err := h.provider.UpdatePreferences(prefs); err != nil {
			h.logger.Error("failed to update preferences", "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update preferences: "+err.Error())
			return
		}

		h.logger.Info("preferences updated")
		h.writeJSON(w, http.StatusOK, PreferencesResponse{Preferences: prefs})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleConfig routes /api/v1/config requests based on HTTP method and path.
func (h *ConfigHandlers) HandleConfig(w http.ResponseWriter, r *http.Request) {
	// Parse the path to determine which handler to use
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/config")
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		h.handleSystemConfig(w, r)
		return
	}

	switch path {
	case "system":
		h.handleSystemConfig(w, r)
	case "dns":
		h.handleDNSConfig(w, r)
	case "health-check":
		h.handleHealthCheckConfig(w, r)
	case "logging":
		h.handleLoggingConfig(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "unknown config endpoint: "+path)
	}
}

// handleSystemConfig handles GET /api/v1/config/system.
func (h *ConfigHandlers) handleSystemConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "config provider not configured")
		return
	}

	config, err := h.provider.GetSystemConfig()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to get system config: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, SystemConfigResponse{Config: *config})
}

// handleDNSConfig handles /api/v1/config/dns requests.
func (h *ConfigHandlers) handleDNSConfig(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "config provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := h.provider.GetDNSConfig()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "failed to get DNS config: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, DNSConfigResponse{Config: *config})

	case http.MethodPut, http.MethodPatch:
		var config DNSConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.provider.UpdateDNSConfig(config); err != nil {
			h.logger.Error("failed to update DNS config", "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update DNS config: "+err.Error())
			return
		}

		h.logger.Info("DNS config updated")
		h.writeJSON(w, http.StatusOK, DNSConfigResponse{Config: config})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleHealthCheckConfig handles /api/v1/config/health-check requests.
func (h *ConfigHandlers) handleHealthCheckConfig(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "config provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := h.provider.GetHealthCheckConfig()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "failed to get health check config: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, HealthCheckConfigResponse{Config: *config})

	case http.MethodPut, http.MethodPatch:
		var config HealthCheckConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.provider.UpdateHealthCheckConfig(config); err != nil {
			h.logger.Error("failed to update health check config", "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update health check config: "+err.Error())
			return
		}

		h.logger.Info("health check config updated")
		h.writeJSON(w, http.StatusOK, HealthCheckConfigResponse{Config: config})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleLoggingConfig handles /api/v1/config/logging requests.
func (h *ConfigHandlers) handleLoggingConfig(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		h.writeError(w, http.StatusServiceUnavailable, "config provider not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := h.provider.GetLoggingConfig()
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "failed to get logging config: "+err.Error())
			return
		}
		h.writeJSON(w, http.StatusOK, LoggingConfigResponse{Config: *config})

	case http.MethodPut, http.MethodPatch:
		var config LoggingConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}

		if err := h.provider.UpdateLoggingConfig(config); err != nil {
			h.logger.Error("failed to update logging config", "error", err)
			h.writeError(w, http.StatusInternalServerError, "failed to update logging config: "+err.Error())
			return
		}

		h.logger.Info("logging config updated")
		h.writeJSON(w, http.StatusOK, LoggingConfigResponse{Config: config})

	default:
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *ConfigHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes a JSON error response.
func (h *ConfigHandlers) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error: message,
		Code:  status,
	})
}
