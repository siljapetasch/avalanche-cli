// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package teleportercmd

import (
	"github.com/ava-labs/avalanche-cli/cmd/subnetcmd"
	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/teleporter"

	"github.com/spf13/cobra"
)

type AddSubnetToRelayerServiceCmdFlags struct {
	Endpoint   string
	UseLocal   bool
	UseDevnet  bool
	UseFuji    bool
	UseMainnet bool
}

var addSubnetToRelayerServiceCmdFlags AddSubnetToRelayerServiceCmdFlags

// avalanche teleporter relayer addSubnetToService
func newAddSubnetToRelayerServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "addSubnetToService [subnetName]",
		Short:        "Adds a subnet to the AWM relayer service configuration",
		Long:         `Adds a subnet to the AWM relayer service configuration".`,
		SilenceUsage: true,
		RunE:         addSubnetToRelayerService,
		Args:         cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&addSubnetToRelayerServiceCmdFlags.Endpoint, "endpoint", "", "use the given endpoint for network operations")
	cmd.Flags().BoolVarP(&addSubnetToRelayerServiceCmdFlags.UseLocal, "local", "l", false, "operate on a local network")
	cmd.Flags().BoolVar(&addSubnetToRelayerServiceCmdFlags.UseDevnet, "devnet", false, "operate on a devnet network")
	cmd.Flags().BoolVarP(&addSubnetToRelayerServiceCmdFlags.UseFuji, "testnet", "t", false, "operate on testnet (alias to `fuji`)")
	cmd.Flags().BoolVarP(&addSubnetToRelayerServiceCmdFlags.UseFuji, "fuji", "f", false, "operate on fuji (alias to `testnet`")
	cmd.Flags().BoolVarP(&addSubnetToRelayerServiceCmdFlags.UseMainnet, "mainnet", "m", false, "operate on mainnet")
	return cmd
}

func addSubnetToRelayerService(_ *cobra.Command, args []string) error {
	return addSubnetToRelayerServiceWithLocalFlags(nil, args, addSubnetToRelayerServiceCmdFlags)
}

func addSubnetToRelayerServiceWithLocalFlags(_ *cobra.Command, args []string, flags AddSubnetToRelayerServiceCmdFlags) error {
	network, err := subnetcmd.GetNetworkFromCmdLineFlags(
		flags.UseLocal,
		flags.UseDevnet,
		flags.UseFuji,
		flags.UseMainnet,
		flags.Endpoint,
		false,
		"",
		[]models.NetworkKind{models.Local},
	)
	if err != nil {
		return err
	}

	subnetName := args[0]

	relayerAddress, relayerPrivateKey, err := teleporter.GetRelayerKeyInfo(app.GetKeyPath(constants.AWMRelayerKeyName))
	if err != nil {
		return err
	}

	subnetID, chainID, messengerAddress, registryAddress, _, err := getSubnetParams(network, "c-chain")
	if err != nil {
		return err
	}

	if err = teleporter.UpdateRelayerConfig(
		app.GetAWMRelayerServiceConfigPath(),
		app.GetAWMRelayerStorageDir(),
		relayerAddress,
		relayerPrivateKey,
		network,
		subnetID.String(),
		chainID.String(),
		messengerAddress,
		registryAddress,
	); err != nil {
		return err
	}

	subnetID, chainID, messengerAddress, registryAddress, _, err = getSubnetParams(network, subnetName)
	if err != nil {
		return err
	}

	if err = teleporter.UpdateRelayerConfig(
		app.GetAWMRelayerServiceConfigPath(),
		app.GetAWMRelayerStorageDir(),
		relayerAddress,
		relayerPrivateKey,
		network,
		subnetID.String(),
		chainID.String(),
		messengerAddress,
		registryAddress,
	); err != nil {
		return err
	}

	return nil
}
