// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func overwatchSettingsConfig() *config.Config {
	return &config.Config{
		Mode: config.ModeOverwatch,
		DNS:  config.DNSConfig{ListenAddress: ":53", DefaultTTL: 30},
		API:  config.APIConfig{Enabled: true, Address: "127.0.0.1:8080"},
		Regions: []config.Region{{
			Name:        "us-east",
			Servers:     []config.Server{{Address: "10.0.0.1", Port: 80, Weight: 100, Service: "app.example.com"}},
			HealthCheck: config.HealthCheck{Type: "http", FailureThreshold: 3, SuccessThreshold: 2},
		}},
		Domains: []config.Domain{{Name: "app.example.com", Regions: []string{"us-east"}}},
		Logging: config.LoggingConfig{Level: "info", Format: "text"},
	}
}

func agentSettingsConfig() *config.Config {
	return &config.Config{
		Mode: config.ModeAgent,
		Agent: config.AgentConfig{
			Identity:  config.AgentIdentityConfig{ServiceToken: "0123456789abcdef", Region: "us-east"},
			Backends:  []config.AgentBackend{{Service: "app.example.com", Address: "10.0.0.1", Port: 80, Weight: 100}},
			Gossip:    config.AgentGossipConfig{EncryptionKey: "key", OverwatchNodes: []string{"ow-1:7946"}},
			Heartbeat: config.HeartbeatConfig{Interval: 10 * time.Second, MissedThreshold: 3},
		},
		Logging: config.LoggingConfig{Level: "info", Format: "text"},
	}
}

func TestRestartRequiredSettings(t *testing.T) {
	tests := []struct {
		name   string
		base   func() *config.Config
		change func(*config.Config)
		want   string
	}{
		{"overwatch: nothing changed", overwatchSettingsConfig, func(*config.Config) {}, ""},
		{"overwatch: servers and domains are reloadable", overwatchSettingsConfig, func(c *config.Config) {
			c.Regions[0].Servers = append(c.Regions[0].Servers, config.Server{Address: "10.0.0.2", Port: 80, Service: "app.example.com"})
			c.Regions[0].Countries = []string{"US"}
			c.Regions[0].HealthCheck.Path = "/ready"
			c.Domains = append(c.Domains, config.Domain{Name: "b.example.com", Regions: []string{"us-east"}})
			c.DNS.DefaultTTL = 60
			c.Includes = []string{"conf.d/*.yaml"}
		}, ""},
		{"overwatch: listen address", overwatchSettingsConfig, func(c *config.Config) {
			c.DNS.ListenAddress = ":5353"
		}, "dns.listen_address"},
		{"overwatch: several sections, sorted", overwatchSettingsConfig, func(c *config.Config) {
			c.Logging.Level = "debug"
			c.API.Address = "0.0.0.0:8080"
			c.Overwatch.Validation.CheckInterval = time.Minute
			c.Metrics.Enabled = true
		}, "api.address|logging.level|metrics.enabled|overwatch.validation"},
		{"overwatch: health manager thresholds", overwatchSettingsConfig, func(c *config.Config) {
			c.Regions[0].HealthCheck.FailureThreshold = 5
		}, "regions[0].health_check.failure_threshold and success_threshold"},
		{"overwatch: the agent section is not used", overwatchSettingsConfig, func(c *config.Config) {
			c.Agent.Heartbeat.Interval = time.Minute
		}, ""},
		{"agent: nothing changed", agentSettingsConfig, func(*config.Config) {}, ""},
		{"agent: backends are reloadable", agentSettingsConfig, func(c *config.Config) {
			c.Agent.Backends = append(c.Agent.Backends, config.AgentBackend{Service: "b.example.com", Address: "10.0.0.2", Port: 81})
		}, ""},
		{"agent: everything else needs a restart", agentSettingsConfig, func(c *config.Config) {
			c.Agent.Gossip.OverwatchNodes = []string{"ow-2:7946"}
			c.Agent.Heartbeat.Interval = time.Minute
			c.Agent.Identity.Region = "eu-west"
			c.Agent.Predictive.Enabled = true
			c.Logging.Format = "json"
		}, "agent.gossip|agent.heartbeat|agent.identity|agent.predictive|logging.format"},
		{"agent: overwatch sections are not used", agentSettingsConfig, func(c *config.Config) {
			c.DNS.ListenAddress = ":5353"
			c.API.Enabled = true
			c.Overwatch.DataDir = "/elsewhere"
			c.Domains = []config.Domain{{Name: "x.example.com"}}
		}, ""},
		{"agent: latency learning ports follow the backends only at startup", func() *config.Config {
			c := agentSettingsConfig()
			c.Agent.LatencyLearning.Enabled = true
			return c
		}, func(c *config.Config) {
			c.Agent.Backends = append(c.Agent.Backends, config.AgentBackend{Service: "b.example.com", Address: "10.0.0.2", Port: 81})
		}, "agent.latency_learning (the ports it watches follow agent.backends only at startup)"},
		{"agent: latency learning with the same ports", func() *config.Config {
			c := agentSettingsConfig()
			c.Agent.LatencyLearning.Enabled = true
			return c
		}, func(c *config.Config) {
			c.Agent.Backends = append(c.Agent.Backends, config.AgentBackend{Service: "b.example.com", Address: "10.0.0.2", Port: 80})
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			running, next := tt.base(), tt.base()
			tt.change(next)
			if got := strings.Join(restartRequiredSettings(running, next), "|"); got != tt.want {
				t.Errorf("restartRequiredSettings = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every setting named in reloadableSettings must exist in the configuration,
// so that a renamed YAML key cannot silently turn into "needs a restart".
func TestReloadableSettingsExist(t *testing.T) {
	known := map[string]bool{}
	for _, s := range configSettings() {
		known[s.path] = true
	}
	for name := range reloadableSettings {
		if !known[name] {
			t.Errorf("reloadableSettings names %q, which is not a configuration setting", name)
		}
	}
	for _, name := range []string{"mode", "dns.listen_address", "overwatch.validation", "agent.gossip", "logging.level"} {
		if !known[name] {
			t.Errorf("configSettings does not list %q", name)
		}
	}
}
