// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// opengslb-cli is the command-line tool for managing OpenGSLB Overwatch.
package main

import (
	"os"

	"github.com/loganrossus/OpenGSLB/cmd/opengslb-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
