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

// Command apidocs generates docs/api.md from api/openapi.yaml.
//
// The OpenAPI document is the single source of truth for the REST API; the
// Markdown reference is derived from it and must not be edited by hand.
// Regenerate it with "make api-docs" (or "go generate ./tools/apidocs").
// The test in this package fails when the committed file is out of date.
package main

//go:generate go run . -spec ../../api/openapi.yaml -out ../../docs/api.md

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	specPath := flag.String("spec", "api/openapi.yaml", "path to the OpenAPI document")
	outPath := flag.String("out", "docs/api.md", "path of the Markdown file to write")
	flag.Parse()

	if err := run(*specPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "apidocs:", err)
		os.Exit(1)
	}
}

func run(specPath, outPath string) error {
	spec, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}
	res, err := Generate(spec)
	if err != nil {
		return fmt.Errorf("%s: %w", specPath, err)
	}
	if err := os.WriteFile(outPath, res.Markdown, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d operations, %d schemas\n", outPath, res.Operations, res.Schemas)
	return nil
}
