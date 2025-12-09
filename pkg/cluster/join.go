// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB \u2013 https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// JoinRequest is sent to request joining a cluster.
type JoinRequest struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	RaftAddress string `json:"raft_address"`
}

// JoinResponse is returned by the cluster join API.
type JoinResponse struct {
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	LeaderID      string `json:"leader_id,omitempty"`
	LeaderAddress string `json:"leader_address,omitempty"`
}

// JoinCluster attempts to join an existing cluster by contacting nodes in the
// provided addresses list. It will try each address, following redirects to
// the leader if necessary.
func JoinCluster(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	if len(cfg.Join) == 0 {
		return fmt.Errorf("no join addresses provided")
	}

	if logger == nil {
		logger = slog.Default()
	}

	req := JoinRequest{
		NodeID:      cfg.GetNodeID(),
		NodeName:    cfg.NodeName,
		RaftAddress: cfg.GetAdvertiseAddress(),
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		// Don't auto-follow redirects - we handle them manually
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Try each join address
	var lastErr error
	for _, addr := range cfg.Join {
		logger.Debug("attempting to join cluster", "target", addr)

		resp, err := tryJoin(ctx, client, addr, reqBody, logger)
		if err != nil {
			logger.Warn("join attempt failed", "target", addr, "error", err)
			lastErr = err
			continue
		}

		if resp.Success {
			logger.Info("successfully joined cluster",
				"leader_id", resp.LeaderID,
				"message", resp.Message,
			)
			return nil
		}

		// If we got a redirect, try the leader address
		if resp.LeaderAddress != "" && resp.LeaderAddress != addr {
			logger.Debug("following redirect to leader", "leader", resp.LeaderAddress)
			resp, err = tryJoin(ctx, client, resp.LeaderAddress, reqBody, logger)
			if err != nil {
				lastErr = err
				continue
			}
			if resp.Success {
				logger.Info("successfully joined cluster via redirect",
					"leader_id", resp.LeaderID,
				)
				return nil
			}
		}

		lastErr = fmt.Errorf("join failed: %s", resp.Message)
	}

	return fmt.Errorf("failed to join cluster after trying all addresses: %w", lastErr)
}

// tryJoin attempts to join at a specific API address.
func tryJoin(ctx context.Context, client *http.Client, apiAddr string, body []byte, logger *slog.Logger) (*JoinResponse, error) {
	// Construct the join URL
	// The apiAddr might be just host:port or a full URL
	url := apiAddr
	if url[0:4] != "http" {
		url = "http://" + apiAddr
	}
	url = url + "/api/v1/cluster/join"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var joinResp JoinResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(respBody))
	}

	// Handle HTTP-level redirects (307)
	if resp.StatusCode == http.StatusTemporaryRedirect {
		// Response body contains leader info
		return &joinResp, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &joinResp, fmt.Errorf("server returned %d: %s", resp.StatusCode, joinResp.Message)
	}

	return &joinResp, nil
}

// JoinWithRetry attempts to join a cluster with exponential backoff.
func JoinWithRetry(ctx context.Context, cfg *Config, logger *slog.Logger, maxRetries int) error {
	if maxRetries <= 0 {
		maxRetries = 5
	}

	backoff := 500 * time.Millisecond
	maxBackoff := 30 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := JoinCluster(ctx, cfg, logger)
		if err == nil {
			return nil
		}

		if attempt == maxRetries {
			return fmt.Errorf("failed to join cluster after %d attempts: %w", maxRetries, err)
		}

		logger.Warn("join attempt failed, retrying",
			"attempt", attempt,
			"max_retries", maxRetries,
			"backoff", backoff,
			"error", err,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff with cap
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("exhausted retries")
}
