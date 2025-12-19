//go:build !linux && !windows

// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package latency

// newPlatformCollector returns an error on unsupported platforms.
func newPlatformCollector(cfg CollectorConfig) (Collector, error) {
	return nil, ErrPlatformNotSupported
}
