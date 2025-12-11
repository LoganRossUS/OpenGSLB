// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// APIClient is the client for Overwatch API communication.
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewAPIClient creates a new API client.
func NewAPIClient() *APIClient {
	return &APIClient{
		BaseURL: apiEndpoint,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// Get performs a GET request to the API.
func (c *APIClient) Get(path string, result interface{}) error {
	url := c.BaseURL + path
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleErrorResponse(resp)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// Put performs a PUT request to the API.
func (c *APIClient) Put(path string, body interface{}, result interface{}) error {
	return c.doJSON(http.MethodPut, path, body, result)
}

// Post performs a POST request to the API.
func (c *APIClient) Post(path string, body interface{}, result interface{}) error {
	return c.doJSON(http.MethodPost, path, body, result)
}

// Delete performs a DELETE request to the API.
func (c *APIClient) Delete(path string) error {
	url := c.BaseURL + path
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.handleErrorResponse(resp)
	}

	return nil
}

// doJSON performs a JSON request.
func (c *APIClient) doJSON(method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return c.handleErrorResponse(resp)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// handleErrorResponse parses error responses from the API.
func (c *APIClient) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as JSON error
	var errResp struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, errResp.Error)
	}

	// Fall back to raw body or status
	if len(body) > 0 {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}
	return fmt.Errorf("API error: %s", resp.Status)
}

// URLEncode URL-encodes a string for path use.
func URLEncode(s string) string {
	return url.PathEscape(s)
}
