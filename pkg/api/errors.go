// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package api

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is a provider error that carries the HTTP status it maps to. Handlers
// answer with Status and Message as they are, so the message must be fit for
// an API client to read.
type Error struct {
	Status  int
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// errInvalid returns a 400: the request is wrong and repeating it will not help.
func errInvalid(format string, args ...any) *Error {
	return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

// errMissing returns a 404: the addressed object does not exist.
func errMissing(format string, args ...any) *Error {
	return &Error{Status: http.StatusNotFound, Message: fmt.Sprintf(format, args...)}
}

// errConflict returns a 409: the request is well formed but cannot be applied
// to the object in its current state (it exists already, or it is owned by
// the configuration file or by an agent).
func errConflict(format string, args ...any) *Error {
	return &Error{Status: http.StatusConflict, Message: fmt.Sprintf(format, args...)}
}

// persistence describes whether a provider can store what the API creates.
// When it cannot, every write is refused: accepting a change that is gone
// after the next restart is worse than refusing it.
type persistence struct {
	// unavailable is the reason there is no store. Empty when none was given.
	unavailable string
}

// errNoStore returns the 503 that answers a write when there is no store.
func (p persistence) errNoStore() *Error {
	reason := p.unavailable
	if reason == "" {
		reason = "no persistent store is configured"
	}
	return &Error{
		Status: http.StatusServiceUnavailable,
		Message: "this overwatch is running without its persistent store, so the change was refused and nothing was changed: " +
			reason + ". Point overwatch.data_dir at a directory the overwatch can write and restart it.",
	}
}

// errorStatus maps a provider error to the HTTP status and message of the
// response. action names what failed ("create domain") and is used only for
// errors that carry no status of their own.
func errorStatus(err error, action string) (int, string) {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr.Status, apiErr.Message
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound, "not found"
	}
	return http.StatusInternalServerError, "failed to " + action + ": " + err.Error()
}
