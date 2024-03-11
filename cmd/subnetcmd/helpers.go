// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"fmt"
	"strings"

	"github.com/ava-labs/avalanche-cli/cmd/flags"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"github.com/ava-labs/avalanchego/api/info"
	"golang.org/x/exp/slices"
)

type NetworkOption int64

const (
	Undefined NetworkOption = iota
	Mainnet
	Fuji
	Local
	Devnet
	Cluster
)

func (n NetworkOption) String() string {
	switch n {
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

func networkOptionFromString(s string) NetworkOption {
	switch s {
	case "Mainnet":
		return Mainnet
	case "Fuji":
		return Fuji
	case "Local Network":
		return Local
	case "Devnet":
		return Devnet
	case "Cluster":
		return Cluster
	}
	return Undefined
}

func GetNetworkFromCmdLineFlags(
	useLocal bool,
	useDevnet bool,
	useFuji bool,
	useMainnet bool,
	endpoint string,
	requireDevnetEndpointSpecification bool,
	clusterName string,
	supportedNetworkOptions []NetworkOption,
) (models.Network, error) {
	var err error
	// supported flags
	networkFlags := map[NetworkOption]string{
		Local:   "--local",
		Devnet:  "--devnet",
		Fuji:    "--fuji/--testnet",
		Mainnet: "--mainnet",
		Cluster: "--cluster",
	}
	supportedNetworksFlags := strings.Join(utils.Map(supportedNetworkOptions, func(n NetworkOption) string { return networkFlags[n] }), ", ")
	// received option
	networkOption := Undefined
	switch {
	case useLocal:
		networkOption = Local
	case useDevnet:
		networkOption = Devnet
	case useFuji:
		networkOption = Fuji
	case useMainnet:
		networkOption = Mainnet
	case clusterName != "":
		networkOption = Cluster
	}
	// unsupported option
	if networkOption != Undefined && !slices.Contains(supportedNetworkOptions, networkOption) {
		return models.UndefinedNetwork, fmt.Errorf("network flag %s is not supported. use one of %s", networkFlags[networkOption], supportedNetworksFlags)
	}
	// mutual exclusion
	if !flags.EnsureMutuallyExclusive([]bool{useLocal, useDevnet, useFuji, useMainnet, clusterName != ""}) {
		return models.UndefinedNetwork, fmt.Errorf("network flags %s are mutually exclusive", supportedNetworksFlags)
	}

	if networkOption == Undefined {
		// undefined, so prompt
		clusterNames, err := app.ListClusterNames()
		if err != nil {
			return models.UndefinedNetwork, err
		}
		if len(clusterNames) == 0 {
			if index, err := utils.GetIndexInSlice(supportedNetworkOptions, Cluster); err == nil {
				supportedNetworkOptions = append(supportedNetworkOptions[:index], supportedNetworkOptions[index+1:]...)
			}
		}
		networkOptionStr, err := app.Prompt.CaptureList(
			"Choose a network for the operation",
			utils.Map(supportedNetworkOptions, func(n NetworkOption) string { return n.String() }),
		)
		if err != nil {
			return models.UndefinedNetwork, err
		}
		networkOption = networkOptionFromString(networkOptionStr)
		switch networkOption {
		case Cluster:
			clusterName, err = app.Prompt.CaptureList(
				"Choose a cluster",
				clusterNames,
			)
			if err != nil {
				return models.UndefinedNetwork, err
			}
		}
	}

	if networkOption == Devnet && endpoint == "" && requireDevnetEndpointSpecification {
		endpoint, err = app.Prompt.CaptureURL(fmt.Sprintf("%s Network Endpoint", networkOption.String()), false)
		if err != nil {
			return models.UndefinedNetwork, err
		}
	}

	network := models.UndefinedNetwork
	switch networkOption {
	case Local:
		network = models.NewLocalNetwork()
	case Devnet:
		networkID := uint32(0)
		if endpoint != "" {
			infoClient := info.NewClient(endpoint)
			ctx, cancel := utils.GetAPIContext()
			defer cancel()
			networkID, err = infoClient.GetNetworkID(ctx)
			if err != nil {
				return models.UndefinedNetwork, err
			}
		}
		network = models.NewDevnetNetwork(endpoint, networkID)
	case Fuji:
		network = models.NewFujiNetwork()
	case Mainnet:
		network = models.NewMainnetNetwork()
	case Cluster:
		network, err = app.GetClusterNetwork(clusterName)
		if err != nil {
			return models.UndefinedNetwork, err
		}
	}
	// on all cases, enable user setting specific endpoint
	if endpoint != "" {
		network.Endpoint = endpoint
	}

	return network, nil
}
