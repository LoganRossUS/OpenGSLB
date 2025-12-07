// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HTTPChecker performs HTTP health checks.
type HTTPChecker struct {
	client *http.Client

	// ValidStatusCodes defines which HTTP status codes indicate healthy.
	// If empty, defaults to 2xx range.
	ValidStatusCodes []int

	// FollowRedirects controls whether redirects are followed.
	FollowRedirects bool

	// InsecureSkipVerify skips TLS certificate validation.
	InsecureSkipVerify bool
}

// HTTPCheckerOption configures an HTTPChecker.
type HTTPCheckerOption func(*HTTPChecker)

// WithValidStatusCodes sets the valid status codes for healthy responses.
func WithValidStatusCodes(codes ...int) HTTPCheckerOption {
	return func(c *HTTPChecker) {
		c.ValidStatusCodes = codes
	}
}

// WithFollowRedirects enables following HTTP redirects.
func WithFollowRedirects(follow bool) HTTPCheckerOption {
	return func(c *HTTPChecker) {
		c.FollowRedirects = follow
	}
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify(skip bool) HTTPCheckerOption {
	return func(c *HTTPChecker) {
		c.InsecureSkipVerify = skip
	}
}

// NewHTTPChecker creates a new HTTP health checker.
func NewHTTPChecker(opts ...HTTPCheckerOption) *HTTPChecker {
	c := &HTTPChecker{
		ValidStatusCodes: nil, // nil means accept 2xx
		FollowRedirects:  false,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Build HTTP client with appropriate settings
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		DisableKeepAlives:     true, // Don't reuse connections for health checks
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.InsecureSkipVerify,
		},
	}

	c.client = &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second, // Overall timeout, overridden by context
	}

	if !c.FollowRedirects {
		c.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return c
}

// Type returns "http".
func (c *HTTPChecker) Type() string {
	return "http"
}

// Check performs an HTTP health check.
func (c *HTTPChecker) Check(ctx context.Context, target Target) Result {
	start := time.Now()

	result := Result{
		Timestamp: start,
	}

	// Build URL
	scheme := target.Scheme
	if scheme == "" {
		scheme = "http"
	}
	path := target.Path
	if path == "" {
		path = "/"
	}
	url := fmt.Sprintf("%s://%s:%d%s", scheme, target.Address, target.Port, path)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to create request: %w", err)
		result.Latency = time.Since(start)
		return result
	}

	req.Header.Set("User-Agent", "OpenGSLB-HealthCheck/1.0")
	req.Header.Set("Connection", "close")

	// Perform request
	resp, err := c.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = fmt.Errorf("request failed: %w", err)
		return result
	}
	defer resp.Body.Close()

	// Check status code
	if c.isValidStatus(resp.StatusCode) {
		result.Healthy = true
	} else {
		result.Error = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return result
}

// isValidStatus checks if the status code indicates a healthy response.
func (c *HTTPChecker) isValidStatus(code int) bool {
	if len(c.ValidStatusCodes) == 0 {
		// Default: accept 2xx
		return code >= 200 && code < 300
	}

	for _, valid := range c.ValidStatusCodes {
		if code == valid {
			return true
		}
	}
	return false
}
