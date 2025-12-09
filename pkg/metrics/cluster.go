// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Cluster/Raft metrics
var (
	// ClusterIsLeader indicates whether this node is the Raft leader.
	ClusterIsLeader = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_is_leader",
			Help:      "1 if this node is the Raft leader, 0 otherwise",
		},
	)

	// ClusterState indicates the current Raft state of this node.
	// Values: 0=follower, 1=candidate, 2=leader, 3=shutdown
	ClusterState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_state",
			Help:      "Current Raft state (0=follower, 1=candidate, 2=leader, 3=shutdown)",
		},
	)

	// ClusterPeers tracks the number of peers in the cluster.
	ClusterPeers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_peers",
			Help:      "Number of peers in the Raft cluster",
		},
	)

	// ClusterLeaderChangesTotal counts leadership transitions.
	ClusterLeaderChangesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cluster_leader_changes_total",
			Help:      "Total number of Raft leadership changes observed",
		},
	)

	// ClusterAppliedIndex tracks the last applied Raft log index.
	ClusterAppliedIndex = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_applied_index",
			Help:      "Last applied Raft log index",
		},
	)

	// ClusterCommitIndex tracks the committed Raft log index.
	ClusterCommitIndex = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_commit_index",
			Help:      "Committed Raft log index",
		},
	)

	// ClusterLastContactSeconds tracks time since last leader contact (for followers).
	ClusterLastContactSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_last_contact_seconds",
			Help:      "Seconds since last contact with leader (followers only)",
		},
	)

	// ClusterSnapshotIndex tracks the last snapshot index.
	ClusterSnapshotIndex = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_snapshot_index",
			Help:      "Last Raft snapshot index",
		},
	)

	// ClusterFSMApplyTotal counts FSM apply operations.
	ClusterFSMApplyTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cluster_fsm_apply_total",
			Help:      "Total number of FSM apply operations",
		},
	)

	// ClusterMode indicates the runtime mode (0=standalone, 1=cluster).
	ClusterMode = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_mode",
			Help:      "Runtime mode (0=standalone, 1=cluster)",
		},
	)

	// ClusterNodeInfo provides node identification as labels.
	ClusterNodeInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cluster_node_info",
			Help:      "Cluster node information",
		},
		[]string{"node_id", "address"},
	)
)

// SetClusterLeader updates the leadership metric.
func SetClusterLeader(isLeader bool) {
	if isLeader {
		ClusterIsLeader.Set(1)
	} else {
		ClusterIsLeader.Set(0)
	}
}

// SetClusterState updates the cluster state metric.
// state: "follower", "candidate", "leader", "shutdown"
func SetClusterState(state string) {
	var value float64
	switch state {
	case "follower":
		value = 0
	case "candidate":
		value = 1
	case "leader":
		value = 2
	case "shutdown":
		value = 3
	default:
		value = 0
	}
	ClusterState.Set(value)
}

// SetClusterPeers updates the peer count metric.
func SetClusterPeers(count int) {
	ClusterPeers.Set(float64(count))
}

// RecordLeaderChange increments the leadership change counter.
func RecordLeaderChange() {
	ClusterLeaderChangesTotal.Inc()
}

// SetClusterIndices updates the Raft index metrics.
func SetClusterIndices(applied, commit, snapshot uint64) {
	ClusterAppliedIndex.Set(float64(applied))
	ClusterCommitIndex.Set(float64(commit))
	ClusterSnapshotIndex.Set(float64(snapshot))
}

// SetClusterLastContact updates the last contact metric.
func SetClusterLastContact(seconds float64) {
	ClusterLastContactSeconds.Set(seconds)
}

// RecordFSMApply increments the FSM apply counter.
func RecordFSMApply() {
	ClusterFSMApplyTotal.Inc()
}

// SetClusterMode sets the runtime mode metric.
func SetClusterMode(isCluster bool) {
	if isCluster {
		ClusterMode.Set(1)
	} else {
		ClusterMode.Set(0)
	}
}

// SetClusterNodeInfo sets the node identification metric.
func SetClusterNodeInfo(nodeID, address string) {
	ClusterNodeInfo.WithLabelValues(nodeID, address).Set(1)
}

// UpdateRaftStats updates metrics from Raft stats map.
func UpdateRaftStats(stats map[string]string) {
	if applied, ok := stats["applied_index"]; ok {
		if v, err := parseUint64(applied); err == nil {
			ClusterAppliedIndex.Set(float64(v))
		}
	}
	if commit, ok := stats["commit_index"]; ok {
		if v, err := parseUint64(commit); err == nil {
			ClusterCommitIndex.Set(float64(v))
		}
	}
	if snapshot, ok := stats["snapshot_index"]; ok {
		if v, err := parseUint64(snapshot); err == nil {
			ClusterSnapshotIndex.Set(float64(v))
		}
	}
	if numPeers, ok := stats["num_peers"]; ok {
		if v, err := parseUint64(numPeers); err == nil {
			ClusterPeers.Set(float64(v))
		}
	}
	if state, ok := stats["state"]; ok {
		SetClusterState(state)
	}
}

// parseUint64 parses a string to uint64.
func parseUint64(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
