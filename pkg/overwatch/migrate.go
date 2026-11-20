// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package overwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// legacyDomainBackend is a record under store.PrefixDomainBackends as version
// 1.1.10 and earlier wrote it for POST /api/v1/domains/{name}/backends.
type legacyDomainBackend struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Weight   int    `json:"weight"`
	Priority int    `json:"priority"`
	Region   string `json:"region"`
	Enabled  bool   `json:"enabled"`
}

// MigrateLegacyDomainBackends rewrites the domain backends that version 1.1.10
// and earlier kept under "domain_backends/{domain}/{id}" into backend records
// under "backends/{domain}:{address}:{port}", the layout of the backend
// registry. It must run before the registry loads its store.
//
// Up to 1.1.10 a backend added to a domain and a server were two kinds of
// object in two places. The domain backends were put into the DNS registry
// directly and were unknown to the backend registry, so they were never
// health checked and could not be read, changed or deleted as servers. From
// now on the backend registry is the one place a server lives.
//
// For each legacy record:
//   - if the backend registry has no record for the same service, address and
//     port, one is written with source "api" and the stored weight, region and
//     priority. The server is enabled whatever the legacy "enabled" flag says:
//     that flag never had an effect, and it was stored as false whenever a
//     request left it out (as the example in the API guide did), so it does
//     not say what the operator wanted;
//   - if it has one already, that record wins and the legacy one is dropped;
//   - the legacy record is deleted once the new one is safely written.
//
// A record that cannot be read is left in place and logged. The function is
// idempotent: a second run finds nothing to do. It returns the number of
// records migrated.
func MigrateLegacyDomainBackends(ctx context.Context, st store.Store, logger *slog.Logger) (int, error) {
	if st == nil {
		return 0, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

	pairs, err := st.List(ctx, store.PrefixDomainBackends)
	if err != nil {
		return 0, fmt.Errorf("failed to list legacy domain backends: %w", err)
	}

	migrated := 0
	for _, pair := range pairs {
		// Key: domain_backends/{domain}/{id}. A domain name holds no "/".
		rest := strings.TrimPrefix(pair.Key, store.PrefixDomainBackends)
		slash := strings.Index(rest, "/")
		if slash <= 0 {
			logger.Warn("skipping legacy domain backend with an unexpected key", "key", pair.Key)
			continue
		}
		domain := rest[:slash]

		var legacy legacyDomainBackend
		if err := json.Unmarshal(pair.Value, &legacy); err != nil {
			logger.Warn("skipping legacy domain backend that cannot be read", "key", pair.Key, "error", err)
			continue
		}
		if net.ParseIP(legacy.Address) == nil || legacy.Port < 1 || legacy.Port > 65535 {
			logger.Warn("skipping legacy domain backend without a valid address and port",
				"key", pair.Key, "address", legacy.Address, "port", legacy.Port)
			continue
		}

		key := storeKey(backendKey(domain, legacy.Address, legacy.Port))
		_, err := st.Get(ctx, key)
		switch {
		case err == nil:
			logger.Info("legacy domain backend is a server already, dropping the legacy record",
				"domain", domain, "address", legacy.Address, "port", legacy.Port)
		case errors.Is(err, store.ErrKeyNotFound):
			weight := legacy.Weight
			if weight == 0 {
				weight = 1
			}
			now := time.Now()
			backend := Backend{
				Service:         domain,
				Address:         legacy.Address,
				Port:            legacy.Port,
				Weight:          weight,
				Region:          legacy.Region,
				Priority:        legacy.Priority,
				Source:          SourceAPI,
				CreatedAt:       now,
				UpdatedAt:       now,
				AgentLastSeen:   now,
				EffectiveStatus: StatusHealthy,
				Authority:       AuthorityStatic,
			}
			data, err := json.Marshal(backend)
			if err != nil {
				return migrated, fmt.Errorf("failed to marshal migrated backend %s: %w", key, err)
			}
			if err := st.Set(ctx, key, data); err != nil {
				return migrated, fmt.Errorf("failed to store migrated backend %s: %w", key, err)
			}
			logger.Info("migrated legacy domain backend to a server",
				"domain", domain, "address", legacy.Address, "port", legacy.Port,
				"weight", weight, "region", legacy.Region, "legacy_enabled_flag_ignored", legacy.Enabled)
			migrated++
		default:
			return migrated, fmt.Errorf("failed to read backend %s: %w", key, err)
		}

		if err := st.Delete(ctx, pair.Key); err != nil {
			return migrated, fmt.Errorf("failed to delete legacy domain backend %s: %w", pair.Key, err)
		}
	}

	return migrated, nil
}
