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

package dns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Server is the DNS server that handles UDP and TCP queries.
type Server struct {
	udpServer *dns.Server
	tcpServer *dns.Server
	handler   *Handler
	address   string
	logger    *slog.Logger

	mu      sync.Mutex
	running bool
}

// ServerConfig contains configuration for the DNS server.
type ServerConfig struct {
	Address string
	Handler *Handler
	Logger  *slog.Logger
}

// NewServer creates a new DNS server.
func NewServer(cfg ServerConfig) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		handler: cfg.Handler,
		address: cfg.Address,
		logger:  logger,
	}
}

// Start begins listening for DNS queries on both UDP and TCP.
// This method blocks until the server is shutdown or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("server already running")
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("starting DNS server",
		"address", s.address,
	)

	// Create UDP server
	s.udpServer = &dns.Server{
		Addr:          s.address,
		Net:           "udp",
		Handler:       s.handler,
		MsgAcceptFunc: acceptMsg,
	}

	// Create TCP server
	s.tcpServer = &dns.Server{
		Addr:          s.address,
		Net:           "tcp",
		Handler:       s.handler,
		MsgAcceptFunc: acceptMsg,
	}

	errChan := make(chan error, 2)

	// Start UDP listener
	go func() {
		s.logger.Info("starting UDP listener", "address", s.address)
		if err := s.udpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("UDP server error: %w", err)
		}
	}()

	// Start TCP listener
	go func() {
		s.logger.Info("starting TCP listener", "address", s.address)
		if err := s.tcpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("TCP server error: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("shutdown signal received")
		return s.Shutdown(context.Background())
	case err := <-errChan:
		s.logger.Error("server error", "error", err)
		_ = s.Shutdown(context.Background())
		return err
	}
}

// acceptMsg decides from the header of an incoming message whether the
// handler gets to see it. It replaces the default of the DNS library, which
// answers every message without exactly one question (FORMERR) and every
// opcode other than QUERY and NOTIFY (NOTIMP) itself, with a bare header: no
// question section and, for a request with EDNS, no OPT record, which RFC
// 6891 section 6.1.1 requires in every response to such a request.
//
// Those requests now reach the handler, which gives the same response codes
// and builds the response like any other. Two protections of the default are
// kept: responses are never answered, and a message whose header announces
// more records than a query can have is rejected before it is parsed.
func acceptMsg(dh dns.Header) dns.MsgAcceptAction {
	const qrBit = 1 << 15
	if dh.Bits&qrBit != 0 {
		return dns.MsgIgnore
	}
	// At most one question; one record each in the answer section (NOTIFY
	// carries an SOA) and the authority section (IXFR carries an SOA); OPT
	// and TSIG in the additional section.
	if dh.Qdcount <= 1 && dh.Ancount <= 1 && dh.Nscount <= 1 && dh.Arcount <= 2 {
		return dns.MsgAccept
	}
	// Too many records to parse. A dynamic update normally looks like this.
	if opcode := int(dh.Bits>>11) & 0xF; opcode != dns.OpcodeQuery && opcode != dns.OpcodeNotify {
		return dns.MsgRejectNotImplemented
	}
	return dns.MsgReject
}

// Shutdown gracefully stops the DNS server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("shutting down DNS server")

	var errs []error

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Shutdown UDP server
	if s.udpServer != nil {
		if err := s.udpServer.ShutdownContext(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("UDP shutdown error: %w", err))
		}
	}

	// Shutdown TCP server
	if s.tcpServer != nil {
		if err := s.tcpServer.ShutdownContext(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("TCP shutdown error: %w", err))
		}
	}

	s.running = false
	s.logger.Info("DNS server stopped")

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// IsRunning returns whether the server is currently running.
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
