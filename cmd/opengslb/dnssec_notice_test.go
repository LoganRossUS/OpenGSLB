// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/loganrossus/OpenGSLB/pkg/config"
)

func dnssecWarnings(logs string) int {
	n := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "level=WARN") && strings.Contains(line, "DNSSEC signing is not active") {
			n++
		}
	}
	return n
}

func TestWarnDNSSECNotActive(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		want    int
	}{{true, 1}, {false, 0}} {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		cfg := &config.Config{}
		cfg.Overwatch.DNSSEC.Enabled = tc.enabled
		warnDNSSECNotActive(logger, cfg)
		if got := dnssecWarnings(buf.String()); got != tc.want {
			t.Errorf("enabled=%v: %d DNSSEC warnings, want %d\n%s", tc.enabled, got, tc.want, buf.String())
		}
	}
}

// An overwatch that starts with overwatch.dnssec.enabled: true must say, once
// and at WARN, that its responses are not signed.
func TestInitialize_WarnsThatDNSSECIsNotActive(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			cfg := loadTestConfig(t, fmt.Sprintf(contractConfig,
				contractFreePort(t), contractFreePort(t), contractFreePort(t), 1, 1))
			cfg.Overwatch.DataDir = t.TempDir()
			cfg.Overwatch.DNSSEC.Enabled = enabled

			var buf bytes.Buffer
			app := NewApplication(cfg, slog.New(slog.NewTextHandler(&buf, nil)))
			if err := app.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			t.Cleanup(func() {
				if app.overwatchStore != nil {
					_ = app.overwatchStore.Close()
				}
			})

			want := 0
			if enabled {
				want = 1
			}
			if got := dnssecWarnings(buf.String()); got != want {
				t.Errorf("%d DNSSEC warnings at startup, want %d", got, want)
			}
		})
	}
}
