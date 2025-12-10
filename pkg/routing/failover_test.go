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

func TestFailoverRouter_EmptyPool(t *testing.T) {
	router := NewFailoverRouter()
	pool := NewSimpleServerPool([]*Server{})

	_, err := router.Route(context.Background(), pool)
	if err != ErrNoHealthyServers {
		t.Errorf("expected ErrNoHealthyServers, got %v", err)
	}
}

func TestFailoverRouter_SingleServer(t *testing.T) {
	router := NewFailoverRouter()
	server := &Server{Address: "10.0.0.1", Port: 80, Weight: 100}
	pool := NewSimpleServerPool([]*Server{server})

	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", selected.Address)
	}
}

func TestFailoverRouter_AlwaysSelectsFirst(t *testing.T) {
	router := NewFailoverRouter()
	servers := []*Server{
		{Address: "10.0.0.1", Port: 80}, // Primary
		{Address: "10.0.0.2", Port: 80}, // Secondary
		{Address: "10.0.0.3", Port: 80}, // Tertiary
	}
	pool := NewSimpleServerPool(servers)

	// Should always return the first server
	for i := 0; i < 100; i++ {
		selected, err := router.Route(context.Background(), pool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.Address != "10.0.0.1" {
			t.Errorf("expected 10.0.0.1 (primary), got %s", selected.Address)
		}
	}
}

func TestFailoverRouter_FailoverToSecondary(t *testing.T) {
	router := NewFailoverRouter()

	// Simulate primary is down - pool only contains secondary and tertiary
	servers := []*Server{
		{Address: "10.0.0.2", Port: 80}, // Secondary (now first)
		{Address: "10.0.0.3", Port: 80}, // Tertiary
	}
	pool := NewSimpleServerPool(servers)

	selected, err := router.Route(context.Background(), pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Address != "10.0.0.2" {
		t.Errorf("expected 10.0.0.2 (secondary), got %s", selected.Address)
	}
}

func TestFailoverRouter_Algorithm(t *testing.T) {
	router := NewFailoverRouter()
	if router.Algorithm() != AlgorithmFailover {
		t.Errorf("expected %s, got %s", AlgorithmFailover, router.Algorithm())
	}
}
