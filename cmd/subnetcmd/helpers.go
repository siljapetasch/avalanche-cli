// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"fmt"
	"strings"

	"github.com/ava-labs/avalanche-cli/cmd/flags"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
//	"golang.org/x/exp/slices"
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


func askForNetworkEndpoint(networkOption NetworkOption) (string, error) {
	return app.Prompt.CaptureString(fmt.Sprintf("%s Network Endpoint", networkOption.String()))
}

func fillNetworkEndpoint(network *models.Network) error {
	if network.Endpoint == "" {
		endpoint, err := app.Prompt.CaptureURL(fmt.Sprintf("%s Network Endpoint", network.Kind.String()))
		if err != nil {
			return err
		}
		network.Endpoint = endpoint
	}
	return nil
}

func GetNetworkFromCmdLineFlags(
	useLocal bool,
	useDevnet bool,
	useFuji bool,
	useMainnet bool,
	endpoint string,
	askForDevnetEndpoint bool,
	clusterName string,
	supportedNetworkOptions []NetworkOption,
) (models.Network, error) {
	// get network from flags
	network := models.UndefinedNetwork
	switch {
	case useLocal:
		network = models.LocalNetwork
	case useDevnet:
		network = models.DevnetNetwork
	case useFuji:
		network = models.FujiNetwork
	case useMainnet:
		network = models.MainnetNetwork
	case clusterName != "":
		return app.GetClusterNetwork(clusterName)
	}
	if endpoint != "" {
		network.Endpoint = endpoint
	}

	// no flag was set, prompt user
	if network.Kind == models.Undefined {
		networkOptionStr, err := app.Prompt.CaptureList(
			"Choose a network for the operation",
			utils.Map(supportedNetworkOptions, func(n NetworkOption) string { return n.String() }),
		)
		if err != nil {
			return models.UndefinedNetwork, err
		}
		networkOption := networkOptionFromString(networkOptionStr)
		switch networkOption {
		case Local:
			return models.LocalNetwork, nil
		case Fuji:
			return models.FujiNetwork, nil
		case Mainnet:
			return models.MainnetNetwork, nil
		case Cluster:
			clusterNames, err := app.ListClusterNames()
			if err != nil {
				return models.UndefinedNetwork, err
			}
			if len(clusterNames) == 0 {
				return models.UndefinedNetwork, fmt.Errorf("there are no clusters defined")
			}
			clusterName, err := app.Prompt.CaptureList(
				"Choose a cluster",
				clusterNames,
			)
			if err != nil {
				return models.UndefinedNetwork, err
			}
			return app.GetClusterNetwork(clusterName)
		case Devnet:
			endpoint, err := askForNetworkEndpoint(Devnet)
			if err != nil {
				return models.UndefinedNetwork, err
			}
			return models.NewStandardDevnetNetworkWithEndpoint(endpoint), nil
		}
		return models.Network{}, fmt.Errorf("PEPE")

		if askForDevnetEndpoint {
			if err := fillNetworkEndpoint(&network); err != nil {
				return models.UndefinedNetwork, err
			}
		}
		return network, nil
	}

	// for err messages
	networkFlags := map[NetworkOption]string{
		Local:   "--local",
		Devnet:  "--devnet",
		Fuji:    "--fuji/--testnet",
		Mainnet: "--mainnet",
		Cluster: "--cluster",
	}
	supportedNetworksFlags := strings.Join(utils.Map(supportedNetworkOptions, func(n NetworkOption) string { return networkFlags[n] }), ", ")

	/*
	// unsupported network
	if !slices.Contains(supportedNetworkOptions, network.Kind) {
		return models.UndefinedNetwork, fmt.Errorf("network flag %s is not supported. use one of %s", networkFlags[network.Kind], supportedNetworksFlags)
	}
	*/

	// not mutually exclusive flag selection
	if !flags.EnsureMutuallyExclusive([]bool{useLocal, useDevnet, useFuji, useMainnet, useDevnet, clusterName != ""}) {
		return models.UndefinedNetwork, fmt.Errorf("network flags %s are mutually exclusive", supportedNetworksFlags)
	}
	if askForDevnetEndpoint {
		if err := fillNetworkEndpoint(&network); err != nil {
			return models.UndefinedNetwork, err
		}
	}

	return network, nil
}
