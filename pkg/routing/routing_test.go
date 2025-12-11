// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"testing"
)

func TestRoundRobinRouter_EmptyPool(t *testing.T) {
	router := NewRoundRobinRouter()
	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestRoundRobinRouter_SingleServer(t *testing.T) {
	router := NewRoundRobinRouter()
	server := &Server{Address: "10.0.0.1", Port: 80, Weight: 100}
	pool := NewSimpleServerPool([]*Server{server})

	for i := 0; i < 10; i++ {
		selected, err := router.Route(context.Background(), pool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.Address != "10.0.0.1" {
			t.Errorf("expected 10.0.0.1, got %s", selected.Address)
		}
	}
}

func TestRoundRobinRouter_Distribution(t *testing.T) {
	router := NewRoundRobinRouter()
	servers := []*Server{
		{Address: "10.0.0.1", Port: 80},
		{Address: "10.0.0.2", Port: 80},
		{Address: "10.0.0.3", Port: 80},
	}
	pool := NewSimpleServerPool(servers)

	counts := make(map[string]int)
	iterations := 300

	for i := 0; i < iterations; i++ {
		selected, err := router.Route(context.Background(), pool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[selected.Address]++
	}

	// Each server should be selected exactly 100 times
	for _, server := range servers {
		expected := iterations / len(servers)
		if counts[server.Address] != expected {
			t.Errorf("server %s: expected %d, got %d", server.Address, expected, counts[server.Address])
		}
	}
}

func TestRoundRobinRouter_Algorithm(t *testing.T) {
	router := NewRoundRobinRouter()
	if router.Algorithm() != AlgorithmRoundRobin {
		t.Errorf("expected %s, got %s", AlgorithmRoundRobin, router.Algorithm())
	}
}

func TestNewRouter_ValidAlgorithms(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"round-robin", AlgorithmRoundRobin},
		{"roundrobin", AlgorithmRoundRobin},
		{"rr", AlgorithmRoundRobin},
		{"weighted", AlgorithmWeighted},
		{"weight", AlgorithmWeighted},
		{"failover", AlgorithmFailover},
		{"active-standby", AlgorithmFailover},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			router, err := NewRouter(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if router.Algorithm() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, router.Algorithm())
			}
		})
	}
}

func TestNewRouter_InvalidAlgorithm(t *testing.T) {
	_, err := NewRouter("invalid")
	if err == nil {
		t.Error("expected error for invalid algorithm")
	}
}

func TestSimpleServerPool(t *testing.T) {
	servers := []*Server{
		{Address: "10.0.0.1", Port: 80},
		{Address: "10.0.0.2", Port: 80},
	}
	pool := NewSimpleServerPool(servers)

	result := pool.Servers()
	if len(result) != 2 {
		t.Errorf("expected 2 servers, got %d", len(result))
	}
}
