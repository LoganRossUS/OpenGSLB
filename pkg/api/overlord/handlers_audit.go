// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overlord

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// handleAuditLogs handles GET /api/audit-logs
func (h *Handlers) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Parse query parameters
	category := r.URL.Query().Get("category")
	action := r.URL.Query().Get("action")
	severity := r.URL.Query().Get("severity")
	user := r.URL.Query().Get("user")

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	var startDate, endDate *time.Time
	if sd := r.URL.Query().Get("startDate"); sd != "" {
		if parsed, err := time.Parse("2006-01-02", sd); err == nil {
			startDate = &parsed
		}
	}
	if ed := r.URL.Query().Get("endDate"); ed != "" {
		if parsed, err := time.Parse("2006-01-02", ed); err == nil {
			// End of day
			parsed = parsed.Add(24*time.Hour - time.Second)
			endDate = &parsed
		}
	}

	// Get logs from audit logger
	entries, total := h.auditLogger.GetLogs(category, action, severity, user, limit, offset, startDate, endDate)

	// Convert to response type
	logs := make([]AuditLog, 0, len(entries))
	for _, entry := range entries {
		logs = append(logs, AuditLog{
			ID:          entry.ID,
			Timestamp:   entry.Timestamp,
			User:        entry.User,
			Action:      string(entry.Action),
			Category:    string(entry.Category),
			Resource:    entry.Resource,
			Description: entry.Description,
			Details:     entry.Details,
			Severity:    string(entry.Severity),
			IPAddress:   entry.IPAddress,
		})
	}

	writeJSON(w, http.StatusOK, AuditLogsResponse{
		Logs:  logs,
		Total: total,
	})
}

// handleAuditLogByID handles GET /api/audit-logs/:id
func (h *Handlers) handleAuditLogByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	idStr := parsePathParam(r.URL.Path, "/api/audit-logs/")
	if idStr == "" || idStr == "stats" || idStr == "export" {
		// These are handled by other handlers
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid log ID", "INVALID_ID")
		return
	}

	entry := h.auditLogger.GetLogByID(id)
	if entry == nil {
		writeError(w, http.StatusNotFound, "audit log not found", "LOG_NOT_FOUND")
		return
	}

	log := AuditLog{
		ID:          entry.ID,
		Timestamp:   entry.Timestamp,
		User:        entry.User,
		Action:      string(entry.Action),
		Category:    string(entry.Category),
		Resource:    entry.Resource,
		Description: entry.Description,
		Details:     entry.Details,
		Severity:    string(entry.Severity),
		IPAddress:   entry.IPAddress,
	}

	writeJSON(w, http.StatusOK, AuditLogResponse{Log: log})
}

// handleAuditLogsStats handles GET /api/audit-logs/stats
func (h *Handlers) handleAuditLogsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	stats := h.auditLogger.GetStats()
	writeJSON(w, http.StatusOK, stats)
}

// handleAuditLogsExport handles POST /api/audit-logs/export
func (h *Handlers) handleAuditLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	category := r.URL.Query().Get("category")

	entries, err := h.auditLogger.Export(category, format)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "EXPORT_FAILED")
		return
	}

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryConfig, "audit-export",
		fmt.Sprintf("Exported audit logs in %s format", format),
		map[string]interface{}{
			"format":   format,
			"category": category,
			"count":    len(entries),
		},
		AuditSeverityInfo, r.RemoteAddr)

	if format == "csv" {
		// Export as CSV
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-logs.csv")

		buf := bytes.Buffer{}
		writer := csv.NewWriter(&buf)

		// Write header
		writer.Write([]string{"ID", "Timestamp", "User", "Action", "Category", "Resource", "Description", "Severity", "IP Address"})

		// Write records
		for _, entry := range entries {
			writer.Write([]string{
				fmt.Sprintf("%d", entry.ID),
				entry.Timestamp.Format(time.RFC3339),
				entry.User,
				string(entry.Action),
				string(entry.Category),
				entry.Resource,
				entry.Description,
				string(entry.Severity),
				entry.IPAddress,
			})
		}

		writer.Flush()
		w.Write(buf.Bytes())
		return
	}

	// Export as JSON
	logs := make([]AuditLog, 0, len(entries))
	for _, entry := range entries {
		logs = append(logs, AuditLog{
			ID:          entry.ID,
			Timestamp:   entry.Timestamp,
			User:        entry.User,
			Action:      string(entry.Action),
			Category:    string(entry.Category),
			Resource:    entry.Resource,
			Description: entry.Description,
			Details:     entry.Details,
			Severity:    string(entry.Severity),
			IPAddress:   entry.IPAddress,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit-logs.json")
	json.NewEncoder(w).Encode(AuditLogsResponse{Logs: logs, Total: len(logs)})
}
