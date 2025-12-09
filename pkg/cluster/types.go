// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package cluster

import "errors"

// Additional errors used by the cluster package.
var (
	ErrNotRunning = errors.New("raft node is not running")
	ErrNoLeader   = errors.New("no leader available")
)

// State represents the current state of a Raft node.
type State int

const (
	// StateFollower indicates the node is a follower.
	StateFollower State = iota
	// StateCandidate indicates the node is a candidate for leadership.
	StateCandidate
	// StateLeader indicates the node is the cluster leader.
	StateLeader
	// StateShutdown indicates the node has been shut down.
	StateShutdown
)

// String returns the string representation of a State.
func (s State) String() string {
	switch s {
	case StateFollower:
		return "Follower"
	case StateCandidate:
		return "Candidate"
	case StateLeader:
		return "Leader"
	case StateShutdown:
		return "Shutdown"
	default:
		return "Unknown"
	}
}

// LeaderInfo contains information about the cluster leader.
type LeaderInfo struct {
	// NodeID is the unique identifier of the leader node.
	NodeID string
	// Address is the Raft address of the leader.
	Address string
}

// NodeInfo contains information about a cluster node.
type NodeInfo struct {
	// ID is the unique identifier of the node.
	ID string
	// Address is the Raft address of the node.
	Address string
	// State is the current Raft state of the node.
	State State
	// IsVoter indicates whether this node can vote in elections.
	IsVoter bool
}

// LeaderObserver is a callback function invoked when leadership changes.
// The boolean parameter is true when this node becomes leader, false when
// it loses leadership.
type LeaderObserver func(isLeader bool)
