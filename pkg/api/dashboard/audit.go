// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// AuditAction represents the type of action performed.
type AuditAction string

const (
	AuditActionCreate AuditAction = "CREATE"
	AuditActionUpdate AuditAction = "UPDATE"
	AuditActionDelete AuditAction = "DELETE"
)

// AuditCategory represents the category of the audited resource.
type AuditCategory string

const (
	AuditCategoryDomain   AuditCategory = "domain"
	AuditCategoryServer   AuditCategory = "server"
	AuditCategoryOverride AuditCategory = "override"
	AuditCategoryHealth   AuditCategory = "health"
	AuditCategoryDNSSEC   AuditCategory = "dnssec"
	AuditCategoryConfig   AuditCategory = "config"
	AuditCategoryGossip   AuditCategory = "gossip"
	AuditCategoryAuth     AuditCategory = "auth"
	AuditCategoryGeo      AuditCategory = "geo"
	AuditCategoryRegion   AuditCategory = "region"
	AuditCategoryAgent    AuditCategory = "agent"
)

// AuditSeverity represents the severity of the audit event.
type AuditSeverity string

const (
	AuditSeverityInfo    AuditSeverity = "info"
	AuditSeverityWarning AuditSeverity = "warning"
	AuditSeverityError   AuditSeverity = "error"
	AuditSeveritySuccess AuditSeverity = "success"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID          int64                  `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	User        string                 `json:"user"`
	Action      AuditAction            `json:"action"`
	Category    AuditCategory          `json:"category"`
	Resource    string                 `json:"resource"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Severity    AuditSeverity          `json:"severity"`
	IPAddress   string                 `json:"ipAddress,omitempty"`
}

// AuditLogger provides audit logging functionality.
type AuditLogger struct {
	logger  *slog.Logger
	entries []AuditEntry
	mu      sync.RWMutex
	nextID  int64
	maxSize int // Maximum number of entries to keep in memory
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditLogger{
		logger:  logger,
		entries: make([]AuditEntry, 0, 1000),
		maxSize: 10000, // Keep last 10000 entries in memory
	}
}

// Log creates a new audit log entry.
func (a *AuditLogger) Log(user string, action AuditAction, category AuditCategory, resource, description string, details map[string]interface{}, severity AuditSeverity, ipAddress string) {
	entry := AuditEntry{
		ID:          atomic.AddInt64(&a.nextID, 1),
		Timestamp:   time.Now().UTC(),
		User:        user,
		Action:      action,
		Category:    category,
		Resource:    resource,
		Description: description,
		Details:     details,
		Severity:    severity,
		IPAddress:   ipAddress,
	}

	a.mu.Lock()
	a.entries = append(a.entries, entry)
	// Trim old entries if we exceed max size
	if len(a.entries) > a.maxSize {
		a.entries = a.entries[len(a.entries)-a.maxSize:]
	}
	a.mu.Unlock()

	// Also log to structured logger
	a.logger.Info("audit",
		"id", entry.ID,
		"user", user,
		"action", action,
		"category", category,
		"resource", resource,
		"description", description,
		"severity", severity,
		"ip", ipAddress,
	)
}

// GetLogs returns audit logs with optional filtering.
func (a *AuditLogger) GetLogs(category string, action string, severity string, user string, limit, offset int, startDate, endDate *time.Time) ([]AuditEntry, int) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Apply filters
	var filtered []AuditEntry
	for i := len(a.entries) - 1; i >= 0; i-- { // Reverse order (newest first)
		entry := a.entries[i]

		if category != "" && string(entry.Category) != category {
			continue
		}
		if action != "" && string(entry.Action) != action {
			continue
		}
		if severity != "" && string(entry.Severity) != severity {
			continue
		}
		if user != "" && entry.User != user {
			continue
		}
		if startDate != nil && entry.Timestamp.Before(*startDate) {
			continue
		}
		if endDate != nil && entry.Timestamp.After(*endDate) {
			continue
		}

		filtered = append(filtered, entry)
	}

	total := len(filtered)

	// Apply pagination
	if offset >= len(filtered) {
		return []AuditEntry{}, total
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end], total
}

// GetLogByID returns a specific audit log entry by ID.
func (a *AuditLogger) GetLogByID(id int64) *AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, entry := range a.entries {
		if entry.ID == id {
			return &entry
		}
	}
	return nil
}

// GetStats returns audit log statistics.
func (a *AuditLogger) GetStats() AuditLogsStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	now := time.Now().UTC()
	hourAgo := now.Add(-1 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	stats := AuditLogsStats{
		Total:      len(a.entries),
		ByCategory: make(map[string]int),
		ByAction:   make(map[string]int),
	}

	for _, entry := range a.entries {
		// Count by category
		stats.ByCategory[string(entry.Category)]++
		// Count by action
		stats.ByAction[string(entry.Action)]++

		// Time-based counts
		if entry.Timestamp.After(dayAgo) {
			stats.Last24h++
		}
		if entry.Timestamp.After(hourAgo) {
			stats.LastHour++
		}
		if entry.Severity == AuditSeverityWarning || entry.Severity == AuditSeverityError {
			stats.Warnings++
		}
	}

	return stats
}

// Export exports audit logs in the specified format.
func (a *AuditLogger) Export(category string, format string) ([]AuditEntry, error) {
	logs, _ := a.GetLogs(category, "", "", "", 10000, 0, nil, nil)
	return logs, nil
}
