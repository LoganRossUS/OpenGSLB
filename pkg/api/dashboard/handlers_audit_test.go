// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"net/http"
	"testing"
)

func TestHandleAuditLogs_GET(t *testing.T) {
	h := testHandlers()

	// First create some audit logs
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "test-domain",
		"Created test domain", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionUpdate, AuditCategoryServer, "10.0.1.10:80",
		"Updated server", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionDelete, AuditCategoryOverride, "override-1",
		"Deleted override", nil, AuditSeverityWarning, "127.0.0.1")

	rr := makeRequest(t, h.handleAuditLogs, http.MethodGet, "/api/audit-logs", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Logs) < 3 {
		t.Errorf("expected at least 3 logs, got %d", len(resp.Logs))
	}
}

func TestHandleAuditLogs_GET_WithCategoryFilter(t *testing.T) {
	h := testHandlers()

	// Create some logs
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryServer, "server1",
		"Created server", nil, AuditSeveritySuccess, "127.0.0.1")

	rr := makeRequest(t, h.handleAuditLogs, http.MethodGet, "/api/audit-logs?category=domain", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsResponse
	decodeJSON(t, rr, &resp)

	for _, log := range resp.Logs {
		if log.Category != "domain" {
			t.Errorf("expected category 'domain', got '%s'", log.Category)
		}
	}
}

func TestHandleAuditLogs_GET_WithUserFilter(t *testing.T) {
	h := testHandlers()

	// Create logs from different users
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("user", AuditActionCreate, AuditCategoryDomain, "domain2",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")

	rr := makeRequest(t, h.handleAuditLogs, http.MethodGet, "/api/audit-logs?user=admin", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsResponse
	decodeJSON(t, rr, &resp)

	for _, log := range resp.Logs {
		if log.User != "admin" {
			t.Errorf("expected user 'admin', got '%s'", log.User)
		}
	}
}

func TestHandleAuditLogs_GET_WithSeverityFilter(t *testing.T) {
	h := testHandlers()

	// Create logs with different severities
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionDelete, AuditCategoryServer, "server1",
		"Deleted server", nil, AuditSeverityWarning, "127.0.0.1")

	rr := makeRequest(t, h.handleAuditLogs, http.MethodGet, "/api/audit-logs?severity=warning", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsResponse
	decodeJSON(t, rr, &resp)

	for _, log := range resp.Logs {
		if log.Severity != "warning" {
			t.Errorf("expected severity 'warning', got '%s'", log.Severity)
		}
	}
}

func TestHandleAuditLogs_GET_WithPagination(t *testing.T) {
	h := testHandlers()

	// Create multiple logs
	for i := 0; i < 10; i++ {
		h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain",
			"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	}

	rr := makeRequest(t, h.handleAuditLogs, http.MethodGet, "/api/audit-logs?limit=5", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsResponse
	decodeJSON(t, rr, &resp)

	if len(resp.Logs) > 5 {
		t.Errorf("expected max 5 logs, got %d", len(resp.Logs))
	}
}

func TestHandleAuditLogs_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAuditLogs, http.MethodPost, "/api/audit-logs", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}

func TestHandleAuditLogByID_GET(t *testing.T) {
	h := testHandlers()

	// Create a log
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "test-domain",
		"Created test domain", nil, AuditSeveritySuccess, "127.0.0.1")

	// Get the log by ID (ID 1 is the first entry)
	rr := makeRequest(t, h.handleAuditLogByID, http.MethodGet, "/api/audit-logs/1", "")

	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogResponse
	decodeJSON(t, rr, &resp)

	if resp.Log.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.Log.ID)
	}
}

func TestHandleAuditLogByID_GET_NotFound(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAuditLogByID, http.MethodGet, "/api/audit-logs/99999", "")

	assertStatus(t, rr, http.StatusNotFound)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "LOG_NOT_FOUND" {
		t.Errorf("expected code 'LOG_NOT_FOUND', got '%s'", resp.Code)
	}
}

func TestHandleAuditLogByID_GET_InvalidID(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAuditLogByID, http.MethodGet, "/api/audit-logs/invalid", "")

	assertStatus(t, rr, http.StatusBadRequest)

	var resp ErrorResponse
	decodeJSON(t, rr, &resp)

	if resp.Code != "INVALID_ID" {
		t.Errorf("expected code 'INVALID_ID', got '%s'", resp.Code)
	}
}

func TestHandleAuditLogsStats_GET(t *testing.T) {
	h := testHandlers()

	// Create some logs
	h.auditLogger.Log("admin", AuditActionCreate, AuditCategoryDomain, "domain1",
		"Created domain", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionUpdate, AuditCategoryServer, "server1",
		"Updated server", nil, AuditSeveritySuccess, "127.0.0.1")
	h.auditLogger.Log("admin", AuditActionDelete, AuditCategoryOverride, "override1",
		"Deleted override", nil, AuditSeverityWarning, "127.0.0.1")

	rr := makeRequest(t, h.handleAuditLogsStats, http.MethodGet, "/api/audit-logs/stats", "")
	assertStatus(t, rr, http.StatusOK)

	var resp AuditLogsStats
	decodeJSON(t, rr, &resp)

	if resp.Total < 3 {
		t.Errorf("expected at least 3 total logs, got %d", resp.Total)
	}
	if resp.ByCategory == nil {
		t.Error("expected non-nil ByCategory")
	}
	if resp.ByAction == nil {
		t.Error("expected non-nil ByAction")
	}
}

func TestHandleAuditLogsStats_MethodNotAllowed(t *testing.T) {
	h := testHandlers()

	rr := makeRequest(t, h.handleAuditLogsStats, http.MethodPost, "/api/audit-logs/stats", "")
	assertStatus(t, rr, http.StatusMethodNotAllowed)
}
