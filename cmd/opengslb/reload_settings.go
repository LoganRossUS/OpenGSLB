// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"reflect"
	"sort"
	"strings"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

// reloadableSettings are the configuration settings Application.Reload
// applies to a running process, named by their YAML path. Every other setting
// is read once at startup: changing it in the file and reloading has no
// effect, and Reload reports it instead of pretending it was applied.
//
// A setting that is added to the configuration is treated as needing a
// restart until it is listed here, which is the safe default.
var reloadableSettings = map[string]bool{
	// Overwatch mode.
	"regions":         true, // servers, health checks, geolocation countries and continents
	"domains":         true,
	"dns.default_ttl": true, // used when the registry is rebuilt
	// Applied to the running DNS handler by reloadDNSSettings.
	"dns.zones":               true,
	"dns.max_udp_size":        true,
	"dns.return_last_healthy": true,

	// Agent mode.
	"agent.backends": true,

	// Not a setting of its own: it names files whose contents are compared
	// setting by setting.
	"includes": true,
}

// configSetting is one configuration setting: a top-level key of the file, or
// a key directly below a top-level section.
type configSetting struct {
	path    string // YAML path, for example "dns.listen_address"
	section string // top-level key, for example "dns"
	index   []int  // field index within config.Config
}

// configSettings lists the settings of config.Config.
func configSettings() []configSetting {
	var settings []configSetting
	root := reflect.TypeOf(config.Config{})
	for i := 0; i < root.NumField(); i++ {
		field := root.Field(i)
		section := yamlName(field)
		if section == "" {
			continue
		}
		if field.Type.Kind() != reflect.Struct {
			settings = append(settings, configSetting{path: section, section: section, index: []int{i}})
			continue
		}
		for j := 0; j < field.Type.NumField(); j++ {
			if name := yamlName(field.Type.Field(j)); name != "" {
				settings = append(settings, configSetting{path: section + "." + name, section: section, index: []int{i, j}})
			}
		}
	}
	return settings
}

// restartRequiredSettings returns the YAML paths of the settings that differ
// between running, the configuration the process started with, and next, a
// newly loaded configuration, and that a reload does not apply. The result is
// sorted. Sections that the running mode does not use are ignored.
func restartRequiredSettings(running, next *config.Config) []string {
	agentMode := running.GetEffectiveMode() == config.ModeAgent
	var names []string

	oldV, newV := reflect.ValueOf(running).Elem(), reflect.ValueOf(next).Elem()
	for _, s := range configSettings() {
		if s.path == "mode" || reloadableSettings[s.path] { // a mode change is rejected by handleReload
			continue
		}
		// Agent mode uses the agent section, overwatch mode every other one.
		// Logging and metrics belong to both.
		if agentMode != (s.section == "agent") && s.section != "logging" && s.section != "metrics" {
			continue
		}
		if !reflect.DeepEqual(oldV.FieldByIndex(s.index).Interface(), newV.FieldByIndex(s.index).Interface()) {
			names = append(names, s.path)
		}
	}

	if agentMode {
		// The latency collector is given the backend ports when it starts.
		if running.Agent.LatencyLearning.Enabled && next.Agent.LatencyLearning.Enabled &&
			!reflect.DeepEqual(agentBackendPorts(running), agentBackendPorts(next)) {
			names = append(names, "agent.latency_learning (the ports it watches follow agent.backends only at startup)")
		}
	} else if len(running.Regions) > 0 && len(next.Regions) > 0 {
		// The health manager takes its thresholds from the first region when
		// it is created.
		oldHC, newHC := running.Regions[0].HealthCheck, next.Regions[0].HealthCheck
		if oldHC.FailureThreshold != newHC.FailureThreshold || oldHC.SuccessThreshold != newHC.SuccessThreshold {
			names = append(names, "regions[0].health_check.failure_threshold and success_threshold")
		}
	}

	sort.Strings(names)
	return names
}

// yamlName returns the YAML key of a configuration struct field, or "" for a
// field that is not part of the file.
func yamlName(field reflect.StructField) string {
	if !field.IsExported() {
		return ""
	}
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return strings.ToLower(field.Name)
	}
	return name
}

// agentBackendPorts returns the sorted distinct ports of the agent backends.
func agentBackendPorts(cfg *config.Config) []int {
	seen := make(map[int]bool)
	ports := make([]int, 0, len(cfg.Agent.Backends))
	for _, backend := range cfg.Agent.Backends {
		if !seen[backend.Port] {
			seen[backend.Port] = true
			ports = append(ports, backend.Port)
		}
	}
	sort.Ints(ports)
	return ports
}
