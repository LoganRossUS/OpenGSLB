// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"testing"
	"time"
)

func TestNewAuditLogger(t *testing.T) {
	logger := NewAuditLogger(nil)

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestAuditLogger_Log(t *testing.T) {
	logger := NewAuditLogger(nil)

	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "test-domain",
		"Created test domain", map[string]interface{}{"name": "test"}, AuditSeveritySuccess, "127.0.0.1")

	entries, total := logger.GetLogs("", "", "", "", 10, 0, nil, nil)
	if total != 1 {
		t.Fatalf("expected 1 entry, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry returned, got %d", len(entries))
	}

	entry := entries[0]
	if entry.User != "admin" {
		t.Errorf("expected user 'admin', got '%s'", entry.User)
	}
	if entry.Action != AuditActionCreate {
		t.Errorf("expected action '%s', got '%s'", AuditActionCreate, entry.Action)
	}
	if entry.Category != AuditCategoryDomain {
		t.Errorf("expected category '%s', got '%s'", AuditCategoryDomain, entry.Category)
	}
	if entry.Resource != "test-domain" {
		t.Errorf("expected resource 'test-domain', got '%s'", entry.Resource)
	}
	if entry.Severity != AuditSeveritySuccess {
		t.Errorf("expected severity '%s', got '%s'", AuditSeveritySuccess, entry.Severity)
	}
	if entry.IPAddress != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got '%s'", entry.IPAddress)
	}
}

func TestAuditLogger_Log_IDIncrement(t *testing.T) {
	logger := NewAuditLogger(nil)

	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain 1", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain2",
		"Created domain 2", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain3",
		"Created domain 3", nil, AuditSeveritySuccess, "127.0.0.1")

	entries, total := logger.GetLogs("", "", "", "", 10, 0, nil, nil)
	if total != 3 {
		t.Fatalf("expected 3 entries, got %d", total)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries returned, got %d", len(entries))
	}

	// IDs should be 1, 2, 3 (in reverse order since GetLogs returns newest first)
	if entries[0].ID != 3 {
		t.Errorf("expected ID 3, got %d", entries[0].ID)
	}
	if entries[1].ID != 2 {
		t.Errorf("expected ID 2, got %d", entries[1].ID)
	}
	if entries[2].ID != 1 {
		t.Errorf("expected ID 1, got %d", entries[2].ID)
	}
}

func TestAuditLogger_GetLogs_Pagination(t *testing.T) {
	logger := NewAuditLogger(nil)

	for i := 0; i < 20; i++ {
		logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain",
			"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	}

	// Get first 5
	entries, total := logger.GetLogs("", "", "", "", 5, 0, nil, nil)
	if total != 20 {
		t.Errorf("expected total 20, got %d", total)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
	if entries[0].ID != 20 {
		t.Errorf("expected first ID 20, got %d", entries[0].ID)
	}

	// Get next 5
	entries, _ = logger.GetLogs("", "", "", "", 5, 5, nil, nil)
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
	if entries[0].ID != 15 {
		t.Errorf("expected first ID 15, got %d", entries[0].ID)
	}
}

func TestAuditLogger_GetLogs_Filter(t *testing.T) {
	logger := NewAuditLogger(nil)

	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("user", AuditActionUpdate, AuditCategoryServer, "server1",
		"Updated server", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionDelete, AuditCategoryOverride, "override1",
		"Deleted override", nil, AuditSeverityWarning, "127.0.0.1")

	// Filter by category
	entries, total := logger.GetLogs("domain", "", "", "", 10, 0, nil, nil)
	if total != 1 {
		t.Errorf("expected 1 entry with category 'domain', got %d", total)
	}
	if len(entries) != 1 || entries[0].Category != AuditCategoryDomain {
		t.Errorf("expected category 'domain', got '%s'", entries[0].Category)
	}

	// Filter by user
	entries, total = logger.GetLogs("", "", "", "admin", 10, 0, nil, nil)
	if total != 2 {
		t.Errorf("expected 2 entries from 'admin', got %d", total)
	}

	// Filter by action
	entries, total = logger.GetLogs("", "CREATE", "", "", 10, 0, nil, nil)
	if total != 1 {
		t.Errorf("expected 1 entry with action 'CREATE', got %d", total)
	}

	// Filter by severity
	entries, total = logger.GetLogs("", "", "warning", "", 10, 0, nil, nil)
	if total != 1 {
		t.Errorf("expected 1 entry with severity 'warning', got %d", total)
	}
}

func TestAuditLogger_GetLogs_TimeFilter(t *testing.T) {
	logger := NewAuditLogger(nil)

	// Add an entry
	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")

	// Filter by start date in the future (should return nothing)
	future := time.Now().Add(time.Hour)
	entries, total := logger.GetLogs("", "", "", "", 10, 0, &future, nil)
	if total != 0 {
		t.Errorf("expected 0 entries in future, got %d", total)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries returned, got %d", len(entries))
	}

	// Filter by start date in the past (should return the entry)
	past := time.Now().Add(-time.Hour)
	entries, total = logger.GetLogs("", "", "", "", 10, 0, &past, nil)
	if total != 1 {
		t.Errorf("expected 1 entry from past, got %d", total)
	}
}

func TestAuditLogger_GetLogByID(t *testing.T) {
	logger := NewAuditLogger(nil)

	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain 1", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain2",
		"Created domain 2", nil, AuditSeveritySuccess, "127.0.0.1")

	entry := logger.GetLogByID(1)
	if entry == nil {
		t.Fatal("expected to find entry with ID 1")
	}
	if entry.Resource != "domain1" {
		t.Errorf("expected resource 'domain1', got '%s'", entry.Resource)
	}

	entry = logger.GetLogByID(2)
	if entry == nil {
		t.Fatal("expected to find entry with ID 2")
	}
	if entry.Resource != "domain2" {
		t.Errorf("expected resource 'domain2', got '%s'", entry.Resource)
	}

	entry = logger.GetLogByID(999)
	if entry != nil {
		t.Error("expected not to find entry with ID 999")
	}
}

func TestAuditLogger_GetStats(t *testing.T) {
	logger := NewAuditLogger(nil)

	// Add entries with different actions and categories
	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionCreate, AuditCategoryServer, "server1",
		"Created server", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionUpdate, AuditCategoryDomain, "domain1",
		"Updated domain", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionDelete, AuditCategoryOverride, "override1",
		"Deleted override", nil, AuditSeverityWarning, "127.0.0.1")

	stats := logger.GetStats()

	if stats.Total != 4 {
		t.Errorf("expected total 4, got %d", stats.Total)
	}
	if stats.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", stats.Warnings)
	}
	if stats.Last24h != 4 {
		t.Errorf("expected 4 in last 24h, got %d", stats.Last24h)
	}
	if stats.LastHour != 4 {
		t.Errorf("expected 4 in last hour, got %d", stats.LastHour)
	}

	// Check by category
	if stats.ByCategory["domain"] != 2 {
		t.Errorf("expected 2 domain entries, got %d", stats.ByCategory["domain"])
	}
	if stats.ByCategory["server"] != 1 {
		t.Errorf("expected 1 server entry, got %d", stats.ByCategory["server"])
	}

	// Check by action
	if stats.ByAction["CREATE"] != 2 {
		t.Errorf("expected 2 CREATE actions, got %d", stats.ByAction["CREATE"])
	}
	if stats.ByAction["UPDATE"] != 1 {
		t.Errorf("expected 1 UPDATE action, got %d", stats.ByAction["UPDATE"])
	}
	if stats.ByAction["DELETE"] != 1 {
		t.Errorf("expected 1 DELETE action, got %d", stats.ByAction["DELETE"])
	}
}

func TestAuditLogger_Export(t *testing.T) {
	logger := NewAuditLogger(nil)

	logger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	logger.Log("admin", AuditActionCreate, AuditCategoryServer, "server1",
		"Created server", nil, AuditSeveritySuccess, "127.0.0.1")

	// Export all
	entries, err := logger.Export("", "json")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Export by category
	entries, err = logger.Export("domain", "json")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestAuditConstants(t *testing.T) {
	// Test action constants
	if AuditActionCreate != "CREATE" {
		t.Errorf("expected AuditActionCreate='CREATE', got '%s'", AuditActionCreate)
	}
	if AuditActionUpdate != "UPDATE" {
		t.Errorf("expected AuditActionUpdate='UPDATE', got '%s'", AuditActionUpdate)
	}
	if AuditActionDelete != "DELETE" {
		t.Errorf("expected AuditActionDelete='DELETE', got '%s'", AuditActionDelete)
	}

	// Test category constants
	if AuditCategoryDomain != "domain" {
		t.Errorf("expected AuditCategoryDomain='domain', got '%s'", AuditCategoryDomain)
	}
	if AuditCategoryServer != "server" {
		t.Errorf("expected AuditCategoryServer='server', got '%s'", AuditCategoryServer)
	}

	// Test severity constants
	if AuditSeverityInfo != "info" {
		t.Errorf("expected AuditSeverityInfo='info', got '%s'", AuditSeverityInfo)
	}
	if AuditSeveritySuccess != "success" {
		t.Errorf("expected AuditSeveritySuccess='success', got '%s'", AuditSeveritySuccess)
	}
	if AuditSeverityWarning != "warning" {
		t.Errorf("expected AuditSeverityWarning='warning', got '%s'", AuditSeverityWarning)
	}
}
