// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"math"
	"testing"
)

func TestWeightedRouter_EmptyPool(t *testing.T) {
	router := NewWeightedRouter()
	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestWeightedRouter_SingleServer(t *testing.T) {
	router := NewWeightedRouter()
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

func TestWeightedRouter_Distribution(t *testing.T) {
	router := NewWeightedRouter()
	servers := []*Server{
		{Address: "10.0.0.1", Port: 80, Weight: 100}, // 50%
		{Address: "10.0.0.2", Port: 80, Weight: 50},  // 25%
		{Address: "10.0.0.3", Port: 80, Weight: 50},  // 25%
	}
	pool := NewSimpleServerPool(servers)

	counts := make(map[string]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		selected, err := router.Route(context.Background(), pool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[selected.Address]++
	}

	// Check distribution is approximately correct (within 10%)
	tolerance := 0.10
	totalWeight := 200.0

	expected1 := float64(iterations) * (100.0 / totalWeight)
	expected2 := float64(iterations) * (50.0 / totalWeight)

	if math.Abs(float64(counts["10.0.0.1"])-expected1)/expected1 > tolerance {
		t.Errorf("10.0.0.1: expected ~%.0f, got %d", expected1, counts["10.0.0.1"])
	}
	if math.Abs(float64(counts["10.0.0.2"])-expected2)/expected2 > tolerance {
		t.Errorf("10.0.0.2: expected ~%.0f, got %d", expected2, counts["10.0.0.2"])
	}
	if math.Abs(float64(counts["10.0.0.3"])-expected2)/expected2 > tolerance {
		t.Errorf("10.0.0.3: expected ~%.0f, got %d", expected2, counts["10.0.0.3"])
	}
}

func TestWeightedRouter_ZeroWeight(t *testing.T) {
	router := NewWeightedRouter()
	servers := []*Server{
		{Address: "10.0.0.1", Port: 80, Weight: 0}, // Should default to 1
		{Address: "10.0.0.2", Port: 80, Weight: 0}, // Should default to 1
	}
	pool := NewSimpleServerPool(servers)

	// Should not error, should select from both
	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		selected, err := router.Route(context.Background(), pool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[selected.Address]++
	}

	// Both should be selected (roughly equal)
	if counts["10.0.0.1"] == 0 || counts["10.0.0.2"] == 0 {
		t.Error("expected both servers to be selected with zero weights")
	}
}

func TestWeightedRouter_Algorithm(t *testing.T) {
	router := NewWeightedRouter()
	if router.Algorithm() != AlgorithmWeighted {
		t.Errorf("expected %s, got %s", AlgorithmWeighted, router.Algorithm())
	}
}
