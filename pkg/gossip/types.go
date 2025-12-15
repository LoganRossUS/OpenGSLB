// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

package gossip

import "time"

// NodeRole identifies the role of a node in the gossip cluster.
type NodeRole string

const (
	// RoleAgent is an agent node that monitors backends.
	RoleAgent NodeRole = "agent"
	// RoleOverwatch is an overwatch node that serves DNS.
	RoleOverwatch NodeRole = "overwatch"
)

// NodeMeta is metadata attached to each memberlist node.
type NodeMeta struct {
	// Role is the node's role (agent or overwatch).
	Role NodeRole `json:"role"`

	// Region is the geographic region.
	Region string `json:"region,omitempty"`

	// Version is the OpenGSLB version.
	Version string `json:"version"`

	// Timestamp is when the metadata was last updated.
	Timestamp time.Time `json:"timestamp"`

	// AgentID is set for agent nodes.
	AgentID string `json:"agent_id,omitempty"`

	// NodeID is set for overwatch nodes.
	NodeID string `json:"node_id,omitempty"`
}

// ClusterStats provides statistics about the gossip cluster.
type ClusterStats struct {
	// NumMembers is the total number of cluster members.
	NumMembers int `json:"num_members"`

	// NumAgents is the number of agent nodes.
	NumAgents int `json:"num_agents"`

	// NumOverwatch is the number of overwatch nodes.
	NumOverwatch int `json:"num_overwatch"`

	// LocalNode is the name of the local node.
	LocalNode string `json:"local_node"`

	// LocalRole is the role of the local node.
	LocalRole NodeRole `json:"local_role"`

	// HealthScore is the cluster health score (0-100).
	HealthScore int `json:"health_score"`
}
