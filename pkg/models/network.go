// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package models

import (
	"fmt"
	"os"
	"strings"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanchego/genesis"
	avagoconstants "github.com/ava-labs/avalanchego/utils/constants"
)

type NetworkKind int64

const (
	Undefined NetworkKind = iota
	Mainnet
	Fuji
	Local
	Devnet
	Cluster
)

func (nk NetworkKind) String() string {
	switch nk {
	case Mainnet:
		return "Mainnet"
	case Fuji:
		return "Fuji"
	case Local:
		return "Local Network"
	case Devnet:
		return "Devnet"
	case Cluster:
		return "Cluster"
	}
	return "invalid network"
}

type Network struct {
	Kind        NetworkKind
	ID          uint32
	Endpoint    string
	ClusterName string
}

var (
	UndefinedNetwork = NewNetwork(Undefined, 0, "")
	LocalNetwork     = NewNetwork(Local, constants.LocalNetworkID, constants.LocalAPIEndpoint)
	DevnetNetwork    = NewNetwork(Devnet, constants.DevnetNetworkID, constants.DevnetAPIEndpoint)
	FujiNetwork      = NewNetwork(Fuji, avagoconstants.FujiID, constants.FujiAPIEndpoint)
	MainnetNetwork   = NewNetwork(Mainnet, avagoconstants.MainnetID, constants.MainnetAPIEndpoint)
)

func NetworkKindFromString(s string) NetworkKind {
	switch s {
	case Mainnet.String():
		return Mainnet
	case Fuji.String():
		return Fuji
	case Local.String():
		return Local
	case Cluster.String():
		return Cluster
	case Devnet.String():
		return Devnet
	}
	return Undefined
}

func NewNetwork(kind NetworkKind, id uint32, endpoint string) Network {
	return Network{
		Kind:     kind,
		ID:       id,
		Endpoint: endpoint,
	}
}

func NewDevnetNetwork(ip string, port int) Network {
	endpoint := fmt.Sprintf("http://%s:%d", ip, port)
	return NewNetwork(Devnet, constants.DevnetNetworkID, endpoint)
}

func NewStandardDevnetNetworkWithEndpoint(endpoint string) Network {
	return NewNetwork(Devnet, constants.DevnetNetworkID, endpoint)
}

func StandardNetworkFromString(s string) Network {
	switch s {
	case Mainnet.String():
		return MainnetNetwork
	case Fuji.String():
		return FujiNetwork
	case Local.String():
		return LocalNetwork
	}
	return UndefinedNetwork
}

// TODO: remove this. If Devnet or cluster, needs more info
func NetworkFromString(s string) Network {
	switch s {
	case Mainnet.String():
		return MainnetNetwork
	case Fuji.String():
		return FujiNetwork
	case Local.String():
		return LocalNetwork
	case Devnet.String():
		return DevnetNetwork
	}
	return UndefinedNetwork
}

func NetworkFromNetworkID(networkID uint32) Network {
	switch networkID {
	case avagoconstants.MainnetID:
		return MainnetNetwork
	case avagoconstants.FujiID:
		return FujiNetwork
	case constants.LocalNetworkID:
		return LocalNetwork
	case constants.DevnetNetworkID:
		return DevnetNetwork
	}
	return UndefinedNetwork
}

func (n Network) Name() string {
	if n.ClusterName != "" {
		return "Cluster " + n.ClusterName
	}
	name := n.Kind.String()
	if n.Kind == Devnet {
		name += " " + n.Endpoint
	}
	return name
}

func (n Network) CChainEndpoint() string {
	return n.BlockchainEndpoint("C")
}

func (n Network) CChainWSEndpoint() string {
	return n.BlockchainWSEndpoint("C")
}

func (n Network) BlockchainEndpoint(blockchainID string) string {
	return fmt.Sprintf("%s/ext/bc/%s/rpc", n.Endpoint, blockchainID)
}

func (n Network) BlockchainWSEndpoint(blockchainID string) string {
	trimmedURI := n.Endpoint
	trimmedURI = strings.TrimPrefix(trimmedURI, "http://")
	trimmedURI = strings.TrimPrefix(trimmedURI, "https://")
	return fmt.Sprintf("ws://%s/ext/bc/%s/ws", trimmedURI, blockchainID)
}

func (n Network) NetworkIDFlagValue() string {
	switch n.Kind {
	case Local:
		return fmt.Sprintf("network-%d", n.ID)
	case Devnet:
		return fmt.Sprintf("network-%d", n.ID)
	case Fuji:
		return "fuji"
	case Mainnet:
		return "mainnet"
	}
	return "invalid-network"
}

func (n Network) GenesisParams() *genesis.Params {
	switch n.Kind {
	case Local:
		return &genesis.LocalParams
	case Devnet:
		return &genesis.LocalParams
	case Fuji:
		return &genesis.FujiParams
	case Mainnet:
		return &genesis.MainnetParams
	}
	return nil
}

func (n *Network) HandlePublicNetworkSimulation() {
	// used in E2E to simulate public network execution paths on a local network
	if os.Getenv(constants.SimulatePublicNetwork) != "" {
		n.Kind = Local
		n.ID = constants.LocalNetworkID
		n.Endpoint = constants.LocalAPIEndpoint
	}
}
