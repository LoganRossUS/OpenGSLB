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

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/loganrossus/OpenGSLB/pkg/overwatch"
	"github.com/loganrossus/OpenGSLB/pkg/store"
)

// defaultDataDir is where the overwatch keeps its store when
// overwatch.data_dir is not set.
const defaultDataDir = "/var/lib/opengslb"

// overwatchDataDir returns the directory of the overwatch store.
func (a *Application) overwatchDataDir() string {
	if dir := a.config.Overwatch.DataDir; dir != "" {
		return dir
	}
	return defaultDataDir
}

// openOverwatchStore opens the bbolt store of the overwatch and migrates
// records of earlier layouts.
//
// There are two ways this can go wrong, and they are handled differently
// (docs/ARCHITECTURE_DECISIONS.md, "Running without the persistent store"):
//
//   - No store has ever existed at the path and none can be created (the
//     directory cannot be made or written). Nothing that was stored can be
//     missing, so the overwatch starts, serves the domains of the
//     configuration file and refuses API writes with 503. This is logged at
//     WARN.
//   - A store file exists, or its existence cannot be ruled out, and it cannot
//     be opened. Starting without it would silently drop the domains and
//     servers that were created through the API and answer NXDOMAIN for them,
//     so the overwatch does not start.
func (a *Application) openOverwatchStore() error {
	dataDir := a.overwatchDataDir()
	storePath := filepath.Join(dataDir, "overwatch.db")

	bboltStore, err := openStoreAt(dataDir, storePath)
	if err != nil {
		if !storeCannotExist(storePath) {
			return fmt.Errorf("the overwatch store %s exists or may exist, and cannot be opened: %w; "+
				"refusing to start without the domains and servers stored in it "+
				"(fix the permissions or the lock, or point overwatch.data_dir at another directory)", storePath, err)
		}
		a.storeUnavailable = fmt.Sprintf("the data directory %q is not usable (%v)", dataDir, err)
		a.logger.Warn("RUNNING WITHOUT PERSISTENCE: the overwatch store could not be created. "+
			"API requests that create, change or delete domains, servers or regions are refused with 503. "+
			"Agent registrations, overrides and pinned agent certificates are kept in memory only and are lost on restart. "+
			"Set overwatch.data_dir to a directory this process can write and restart.",
			"data_dir", dataDir,
			"store", storePath,
			"error", err,
		)
		return nil
	}
	a.overwatchStore = bboltStore

	migrated, err := overwatch.MigrateLegacyDomainBackends(context.Background(), bboltStore, a.logger)
	if err != nil {
		// The records stay where they are and are tried again at the next
		// start. The servers in them are not served until then.
		a.logger.Error("failed to migrate domain backends stored by an earlier version", "error", err)
	} else if migrated > 0 {
		a.logger.Info("migrated domain backends stored by an earlier version to servers", "count", migrated)
	}
	return nil
}

// openStoreAt creates the data directory if needed and opens the store in it.
func openStoreAt(dataDir, storePath string) (*store.BboltStore, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	return store.NewBboltStore(storePath)
}

// storeCannotExist reports whether it is certain that there is no store file
// at path: the file, or a directory on the way to it, does not exist.
// Any other answer (it exists, or it cannot be looked at) counts as "may exist".
func storeCannotExist(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// syncBackendToDNS brings the DNS entry of a backend's domain in line with
// the backend: registered with its current weight and region, or absent when
// the backend was removed or is disabled. It is called for every change the
// backend registry reports.
//
// NOTE: it may run while the registry holds its write lock. Do NOT call
// methods that lock the registry from here.
func (a *Application) syncBackendToDNS(backend *overwatch.Backend, removed bool) {
	if a.dnsRegistry == nil {
		return
	}
	if removed || backend.Disabled {
		if err := a.dnsRegistry.DeregisterServer(backend.Service, backend.Address, backend.Port); err != nil {
			// Not being in DNS is the wanted state: a disabled backend that was
			// never registered, or one whose domain is gone, is not an error.
			a.logger.Debug("backend not removed from DNS",
				"service", backend.Service,
				"address", backend.Address,
				"port", backend.Port,
				"error", err)
		}
		return
	}
	if err := a.dnsRegistry.RegisterServer(backend.Service, backend.Address, backend.Port, backend.Weight, backend.Region); err != nil {
		a.logger.Error("failed to register backend in DNS",
			"service", backend.Service,
			"address", backend.Address,
			"port", backend.Port,
			"error", err)
	}
}

// syncDynamicBackendsToDNS registers every backend that does not come from
// the configuration file (agent and API backends) in the DNS registry, and
// leaves disabled ones out. Backends of the configuration file are placed by
// dns.BuildRegistry, which also applies the regions of each domain.
//
// Startup calls it after the stored domains are loaded. Anything that
// rebuilds the DNS registry (a configuration reload) must call it afterwards,
// or the API and agent backends are missing from DNS until the next restart.
func (a *Application) syncDynamicBackendsToDNS() {
	if a.dnsRegistry == nil || a.backendRegistry == nil {
		return
	}
	for _, backend := range a.backendRegistry.GetAllBackends() {
		if backend.Source == overwatch.SourceStatic || backend.Disabled {
			continue
		}
		if err := a.dnsRegistry.RegisterServer(backend.Service, backend.Address, backend.Port, backend.Weight, backend.Region); err != nil {
			a.logger.Debug("backend not registered in DNS",
				"service", backend.Service,
				"address", backend.Address,
				"port", backend.Port,
				"error", err)
		}
	}
}
