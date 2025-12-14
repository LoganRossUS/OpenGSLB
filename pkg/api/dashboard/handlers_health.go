// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ValidationJob represents an in-progress or completed validation job.
type ValidationJob struct {
	ID         string
	Status     string
	StartTime  time.Time
	EndTime    *time.Time
	Scope      string
	Backends   []string
	Service    string
	Region     string
	Results    []ValidationResult
	TotalCount int
	Passed     int
	Failed     int
}

// validationJobs stores validation jobs in memory.
var (
	validationJobs   = make(map[string]*ValidationJob)
	validationJobsMu sync.RWMutex
)

// handleHealthValidate handles POST /api/health/validate
func (h *Handlers) handleHealthValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	var req ValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
		return
	}

	// Default scope to "all"
	if req.Scope == "" {
		req.Scope = "all"
	}

	// Validate scope
	validScopes := map[string]bool{
		"all":       true,
		"unhealthy": true,
		"selected":  true,
	}
	if !validScopes[req.Scope] {
		writeError(w, http.StatusBadRequest, "invalid scope", "INVALID_SCOPE")
		return
	}

	// Generate validation ID
	validationID := uuid.New().String()[:8]

	// Create validation job
	job := &ValidationJob{
		ID:        validationID,
		Status:    "in_progress",
		StartTime: time.Now(),
		Scope:     req.Scope,
		Backends:  req.Backends,
		Service:   req.Service,
		Region:    req.Region,
		Results:   make([]ValidationResult, 0),
	}

	validationJobsMu.Lock()
	validationJobs[validationID] = job
	validationJobsMu.Unlock()

	// Start validation in background
	go h.runValidation(job)

	// Log audit entry
	user := getUser(r)
	h.auditLogger.Log(user, AuditActionCreate, AuditCategoryHealth, validationID,
		fmt.Sprintf("Started validation job %s with scope %s", validationID, req.Scope),
		map[string]interface{}{
			"scope":   req.Scope,
			"service": req.Service,
			"region":  req.Region,
		},
		AuditSeverityInfo, r.RemoteAddr)

	writeJSON(w, http.StatusOK, ValidationStartResponse{
		ValidationID: validationID,
		Status:       "in_progress",
	})
}

// runValidation performs the actual validation work.
func (h *Handlers) runValidation(job *ValidationJob) {
	registry := h.dataProvider.GetBackendRegistry()
	validator := h.dataProvider.GetValidator()

	if registry == nil {
		job.Status = "failed"
		endTime := time.Now()
		job.EndTime = &endTime
		return
	}

	backends := registry.GetAllBackends()

	// Filter backends based on scope
	for _, b := range backends {
		// Apply filters
		if job.Service != "" && b.Service != job.Service {
			continue
		}
		if job.Region != "" && b.Region != job.Region {
			continue
		}
		if job.Scope == "unhealthy" && b.EffectiveStatus == "healthy" {
			continue
		}
		if job.Scope == "selected" && len(job.Backends) > 0 {
			found := false
			backendID := fmt.Sprintf("%s:%d", b.Address, b.Port)
			for _, id := range job.Backends {
				if id == backendID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		job.TotalCount++

		// Perform validation
		var result ValidationResult
		result.BackendID = fmt.Sprintf("%s:%d", b.Address, b.Port)

		if validator != nil {
			// Use the validator to check health
			err := validator.ValidateBackend(b.Service, b.Address, b.Port)
			if err != nil {
				result.Healthy = false
				result.Error = err.Error()
				job.Failed++
			} else {
				result.Healthy = true
				job.Passed++
			}
		} else {
			// Fallback: assume healthy if validation is not available
			result.Healthy = b.EffectiveStatus == "healthy"
			if result.Healthy {
				job.Passed++
			} else {
				job.Failed++
				result.Error = "no validator available"
			}
		}

		result.Latency = int(b.SmoothedLatency.Milliseconds())
		job.Results = append(job.Results, result)
	}

	// Mark as complete
	endTime := time.Now()
	job.EndTime = &endTime
	job.Status = "completed"
}

// handleValidationStatus handles GET /api/health/validation/:id
func (h *Handlers) handleValidationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	// Parse validation ID from path
	id := parsePathParam(r.URL.Path, "/api/health/validation/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation ID is required", "MISSING_PARAM")
		return
	}

	validationJobsMu.RLock()
	job, exists := validationJobs[id]
	validationJobsMu.RUnlock()

	if !exists {
		writeError(w, http.StatusNotFound, "validation not found", "VALIDATION_NOT_FOUND")
		return
	}

	var duration int
	if job.EndTime != nil {
		duration = int(job.EndTime.Sub(job.StartTime).Milliseconds())
	} else {
		duration = int(time.Since(job.StartTime).Milliseconds())
	}

	status := ValidationStatus{
		ValidationID:      job.ID,
		Status:            job.Status,
		BackendsValidated: job.TotalCount,
		ValidationsPassed: job.Passed,
		ValidationsFailed: job.Failed,
		Duration:          duration,
		Results:           job.Results,
	}

	writeJSON(w, http.StatusOK, status)
}

// handleHealthStatus handles GET /api/health/status
func (h *Handlers) handleHealthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
		return
	}

	servers := make([]Backend, 0)

	registry := h.dataProvider.GetBackendRegistry()
	if registry != nil {
		backends := registry.GetAllBackends()
		for _, b := range backends {
			servers = append(servers, backendToServer(b))
		}
	}

	// Find the most recent validation job
	var latestValidation *ValidationJob
	validationJobsMu.RLock()
	for _, job := range validationJobs {
		if latestValidation == nil || job.StartTime.After(latestValidation.StartTime) {
			latestValidation = job
		}
	}
	validationJobsMu.RUnlock()

	response := HealthStatusResponse{
		Backends: servers,
	}

	if latestValidation != nil {
		var duration int
		if latestValidation.EndTime != nil {
			duration = int(latestValidation.EndTime.Sub(latestValidation.StartTime).Milliseconds())
		}
		response.ValidationStatus = ValidationStatus{
			ValidationID:      latestValidation.ID,
			Status:            latestValidation.Status,
			BackendsValidated: latestValidation.TotalCount,
			ValidationsPassed: latestValidation.Passed,
			ValidationsFailed: latestValidation.Failed,
			Duration:          duration,
		}
	}

	writeJSON(w, http.StatusOK, response)
}
