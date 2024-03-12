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
	"github.com/spf13/cobra"
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

type NetworkFlags struct {
	UseLocal   bool
	UseDevnet  bool
	UseFuji    bool
	UseMainnet bool
	Endpoint   string
	ClusterName string
}

var (
	globalNetworkFlags NetworkFlags
)

func AddNetworkFlagsToCmd(cmd *cobra.Command, networkFlags *NetworkFlags, alwaysAddEndpoint bool, supportedNetworkOptions []NetworkOption) {
	addEndpoint := alwaysAddEndpoint
	for _, networkOption := range supportedNetworkOptions {
		switch networkOption {
		case Local:
			cmd.Flags().BoolVarP(&networkFlags.UseLocal, "local", "l", false, "opeate on a local network")
		case Devnet:
			cmd.Flags().BoolVar(&networkFlags.UseDevnet, "devnet", false, "operate on a devnet network")
			addEndpoint = true
		case Fuji:
			cmd.Flags().BoolVarP(&networkFlags.UseFuji, "testnet", "t", false, "operate on testnet (alias to `fuji`)")
			cmd.Flags().BoolVarP(&networkFlags.UseFuji, "fuji", "f", false, "operate on fuji (alias to `testnet`")
		case Mainnet:
			cmd.Flags().BoolVarP(&networkFlags.UseMainnet, "mainnet", "m", false, "operate on mainnet")
		case Cluster:
			cmd.Flags().StringVar(&networkFlags.ClusterName, "cluster", "", "operate on the given cluster")
		}
	}
	if addEndpoint {
		cmd.Flags().StringVar(&networkFlags.Endpoint, "endpoint", "", "use the given endpoint for network operations")
	}
}

func GetNetworkFromCmdLineFlags(
	networkFlags NetworkFlags,
	requireDevnetEndpointSpecification bool,
	supportedNetworkOptions []NetworkOption,
	subnetName string,
) (models.Network, error) {
	if subnetName != "" {
		// get supported networks from sidecar
		sc, err := app.LoadSidecar(subnetName)
		if err != nil {
			return models.UndefinedNetwork, err
		}
		for networkName, _ := range sc.Networks {
			fmt.Println(networkName)
		}
	}
	return models.UndefinedNetwork, fmt.Errorf("PEPE")

	var err error
	// supported flags
	networkFlagsMap := map[NetworkOption]string{
		Local:   "--local",
		Devnet:  "--devnet",
		Fuji:    "--fuji/--testnet",
		Mainnet: "--mainnet",
		Cluster: "--cluster",
	}
	supportedNetworksFlags := strings.Join(utils.Map(supportedNetworkOptions, func(n NetworkOption) string { return networkFlagsMap[n] }), ", ")
	// received option
	networkOption := Undefined
	switch {
	case networkFlags.UseLocal:
		networkOption = Local
	case networkFlags.UseDevnet:
		networkOption = Devnet
	case networkFlags.UseFuji:
		networkOption = Fuji
	case networkFlags.UseMainnet:
		networkOption = Mainnet
	case networkFlags.ClusterName != "":
		networkOption = Cluster
	}
	// unsupported option
	if networkOption != Undefined && !slices.Contains(supportedNetworkOptions, networkOption) {
		return models.UndefinedNetwork, fmt.Errorf("network flag %s is not supported. use one of %s", networkFlagsMap[networkOption], supportedNetworksFlags)
	}
	// mutual exclusion
	if !flags.EnsureMutuallyExclusive([]bool{networkFlags.UseLocal, networkFlags.UseDevnet, networkFlags.UseFuji, networkFlags.UseMainnet, networkFlags.ClusterName != ""}) {
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
			networkFlags.ClusterName, err = app.Prompt.CaptureList(
				"Choose a cluster",
				clusterNames,
			)
			if err != nil {
				return models.UndefinedNetwork, err
			}
		}
	}

	if networkOption == Devnet && networkFlags.Endpoint == "" && requireDevnetEndpointSpecification {
		networkFlags.Endpoint, err = app.Prompt.CaptureURL(fmt.Sprintf("%s Network Endpoint", networkOption.String()), false)
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
		if networkFlags.Endpoint != "" {
			infoClient := info.NewClient(networkFlags.Endpoint)
			ctx, cancel := utils.GetAPIContext()
			defer cancel()
			networkID, err = infoClient.GetNetworkID(ctx)
			if err != nil {
				return models.UndefinedNetwork, err
			}
		}
		network = models.NewDevnetNetwork(networkFlags.Endpoint, networkID)
	case Fuji:
		network = models.NewFujiNetwork()
	case Mainnet:
		network = models.NewMainnetNetwork()
	case Cluster:
		network, err = app.GetClusterNetwork(networkFlags.ClusterName)
		if err != nil {
			return models.UndefinedNetwork, err
		}
	}
	// on all cases, enable user setting specific endpoint
	if networkFlags.Endpoint != "" {
		network.Endpoint = networkFlags.Endpoint
	}

	return network, nil
}

func CreateSubnetFirst(cmd *cobra.Command, args []string, subnetName string, skipPrompt bool) error {
	if !app.SubnetConfigExists(subnetName) {
		if !skipPrompt {
			yes, err := app.Prompt.CaptureNoYes(fmt.Sprintf("Subnet %s is not created yet. Do you want to create it first?", subnetName))
			if err != nil {
				return err
			}
			if !yes {
				return fmt.Errorf("subnet not available and not being created first")
			}
		}
		return createSubnetConfig(cmd, args)
	}
	return nil
}

func DeploySubnetFirst(cmd *cobra.Command, args []string, subnetName string, skipPrompt bool) error {
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}
	if len(sc.Networks) == 0 {
		if !skipPrompt {
			yes, err := app.Prompt.CaptureNoYes(fmt.Sprintf("Subnet %s is not deployed yet. Do you want to deploy it first?", subnetName))
			if err != nil {
				return err
			}
			if !yes {
				return fmt.Errorf("subnet not deployed and not being deployed first")
			}
		}
		return runDeploy(cmd, args)
	}
	return nil
}
