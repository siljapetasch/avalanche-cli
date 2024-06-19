// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package models

import (
	"fmt"
	"slices"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
)

type NodeConfig struct {
	NodeID        string // instance id on cloud server
	Region        string // region where cloud server instance is deployed
	AMI           string // image id for cloud server dependent on its os (e.g. ubuntu )and region deployed (e.g. us-east-1)
	KeyPair       string // key pair name used on cloud server
	CertPath      string // where the cert is stored in user's local machine ssh directory
	SecurityGroup string // security group used on cloud server
	ElasticIP     string // public IP address of the cloud server
	CloudService  string // which cloud service node is hosted on (AWS / GCP)
	UseStaticIP   bool   // node has a static IP association
	IsMonitor     bool   // node has a monitoring dashboard (depricated)
	IsAWMRelayer  bool   // node has an AWM relayer service (depricated)
	IsLoadTest    bool   // node is used to host load test (deprecated)
	Roles         []NodeRole
}

type NodeRole int64

const (
	EmptyRole NodeRole = iota
	Validator
	Api
	Monitor
	AWMRelayer
	LoadTest
)

func (nr NodeRole) String() string {
	switch nr {
	case Validator:
		return constants.ValidatorRole
	case Api:
		return constants.APIRole
	case Monitor:
		return constants.MonitorRole
	case AWMRelayer:
		return constants.AWMRelayerRole
	case LoadTest:
		return constants.LoadTestRole
	default:
		return ""
	}
}

// AddRole adds a role to the node
func (nc *NodeConfig) AddRole(role NodeRole) {
	nc.Roles = append(nc.Roles, role)
}

// HasMonitor checks if the node has a monitor role
func (nc *NodeConfig) Monitor() bool {
	if slices.Contains(nc.Roles, Monitor) || nc.IsMonitor {
		return true
	}
	return false
}

// AWMRelayer checks if the node has an AWM relayer role
func (nc *NodeConfig) AWMRelayer() bool {
	if slices.Contains(nc.Roles, AWMRelayer) || nc.IsAWMRelayer {
		return true
	}
	return false
}

// LoadTest checks if the node has a load test role
func (nc *NodeConfig) LoadTest() bool {
	if slices.Contains(nc.Roles, LoadTest) || nc.IsLoadTest {
		return true
	}
	return false
}

// SyncRoles adds roles to the node
func (nc *NodeConfig) SyncRoles(roles []string) error {
	for _, role := range roles {
		switch role {
		case constants.ValidatorRole:
			nc.AddRole(Validator)
		case constants.APIRole:
			nc.AddRole(Api)
		case constants.MonitorRole:
			nc.AddRole(Monitor)
		case constants.AWMRelayerRole:
			nc.AddRole(AWMRelayer)
		case constants.LoadTestRole:
			nc.AddRole(LoadTest)
		default:
			return fmt.Errorf("role %s is not supported", role)
		}
	}
	return nil
}
