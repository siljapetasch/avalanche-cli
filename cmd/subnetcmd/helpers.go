// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"fmt"
	"strings"

	"github.com/ava-labs/avalanche-cli/cmd/flags"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"golang.org/x/exp/slices"
)

func askForNetworkEndpoint(networkKind models.NetworkKind) (string, error) {
	return app.Prompt.CaptureString(fmt.Sprintf("%s Network Endpoint", networkKind.String()))
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
	supportedNetworkKinds []models.NetworkKind,
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
        var err error
        network, err = app.GetClusterNetwork(clusterName)
        if err != nil {
			return models.UndefinedNetwork, err
        }
        network.Kind = models.Cluster
        network.ClusterName = clusterName
        return network, nil
	}
	if endpoint != "" {
		network.Endpoint = endpoint
	}

	// no flag was set, prompt user
	if network.Kind == models.Undefined {
		networkKindStr, err := app.Prompt.CaptureList(
			"Choose a network for the operation",
			utils.Map(supportedNetworkKinds, func(n models.NetworkKind) string { return n.String() }),
		)
		if err != nil {
			return models.UndefinedNetwork, err
		}
		network = models.StandardNetworkFromString(networkKindStr)
		if network == models.UndefinedNetwork {
			networkKind := models.NetworkKindFromString(networkKindStr)
			switch networkKind {
			case models.Devnet:
				endpoint, err := askForNetworkEndpoint(networkKind)
				if err != nil {
					return models.UndefinedNetwork, err
				}
				return models.NewStandardDevnetNetworkWithEndpoint(endpoint), nil
			}
			return models.Network{}, fmt.Errorf("PEPE")
		}


		if askForDevnetEndpoint {
			if err := fillNetworkEndpoint(&network); err != nil {
				return models.UndefinedNetwork, err
			}
		}
		return network, nil
	}

	// for err messages
	networkFlags := map[models.NetworkKind]string{
		models.Local:   "--local",
		models.Devnet:  "--devnet",
		models.Fuji:    "--fuji/--testnet",
		models.Mainnet: "--mainnet",
	}
	supportedNetworksFlags := strings.Join(utils.Map(supportedNetworkKinds, func(n models.NetworkKind) string { return networkFlags[n] }), ", ")

	// unsupported network
	if !slices.Contains(supportedNetworkKinds, network.Kind) {
		return models.UndefinedNetwork, fmt.Errorf("network flag %s is not supported. use one of %s", networkFlags[network.Kind], supportedNetworksFlags)
	}

	// not mutually exclusive flag selection
	if !flags.EnsureMutuallyExclusive([]bool{useLocal, useDevnet, useFuji, useMainnet}) {
		return models.UndefinedNetwork, fmt.Errorf("network flags %s are mutually exclusive", supportedNetworksFlags)
	}
	if askForDevnetEndpoint {
		if err := fillNetworkEndpoint(&network); err != nil {
			return models.UndefinedNetwork, err
		}
	}

	return network, nil
}
