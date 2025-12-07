#!/bin/bash

# Script to add copyright headers to all .go files

HEADER='// Copyright (C) 2025 Logan Ross
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
'

# Find all .go files and add header if not already present
find . -name "*.go" -type f | while read -r file; do
    # Check if file already has the copyright header
    if ! grep -q "Copyright (C) 2025 Logan Ross" "$file"; then
        echo "Adding header to: $file"
        # Create temp file with header + original content
        echo "$HEADER" | cat - "$file" > "$file.tmp"
        mv "$file.tmp" "$file"
    else
        echo "Skipping (already has header): $file"
    fi
done

echo "Done! Headers added to all .go files."
