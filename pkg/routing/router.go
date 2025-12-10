// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package routing

import (
	"context"
	"errors"
)

// ErrNoHealthyServers is returned when no healthy servers are available.
var ErrNoHealthyServers = errors.New("no healthy servers available")

// Server represents a backend server for routing decisions.
type Server struct {
	Address string
	Port    int
	Weight  int
	Region  string
}

// ServerPool provides access to servers for routing decisions.
type ServerPool interface {
	// Servers returns all servers in the pool.
	Servers() []*Server
}

// Router selects a server from a pool based on a routing algorithm.
// Note: "Router" refers to DNS response routing (selecting which IP to return),
// not network traffic routing. See ADR-011.
type Router interface {
	// Route selects a server from the pool.
	// Returns ErrNoHealthyServers if the pool is empty.
	Route(ctx context.Context, pool ServerPool) (*Server, error)

	// Algorithm returns the name of the routing algorithm.
	Algorithm() string
}

// SimpleServerPool is a basic implementation of ServerPool.
type SimpleServerPool struct {
	servers []*Server
}

// NewSimpleServerPool creates a new SimpleServerPool with the given servers.
func NewSimpleServerPool(servers []*Server) *SimpleServerPool {
	return &SimpleServerPool{servers: servers}
}

// Servers returns all servers in the pool.
func (p *SimpleServerPool) Servers() []*Server {
	return p.servers
}
