// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package teleportercmd

import (
	"fmt"

	"github.com/ava-labs/avalanche-cli/cmd/subnetcmd"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/subnet"
	"github.com/ava-labs/avalanche-cli/pkg/teleporter"
	"github.com/ava-labs/avalanchego/ids"

	"github.com/spf13/cobra"
)

type DeployCmdFlags struct {
	Endpoint   string
	UseLocal   bool
	UseDevnet  bool
	UseFuji    bool
	UseMainnet bool
}

var deployCmdflags DeployCmdFlags

// avalanche teleporter deploy
func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "deploy [subnetName]",
		Short:        "Deploys Teleporter into the given Subnet",
		Long:         `Deploys Teleporter into the given Subnet.`,
		SilenceUsage: true,
		RunE:         deploy,
		Args:         cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&deployCmdflags.Endpoint, "endpoint", "", "use the given endpoint for network operations")
	cmd.Flags().BoolVarP(&deployCmdflags.UseLocal, "local", "l", false, "operate on a local network")
	cmd.Flags().BoolVar(&deployCmdflags.UseDevnet, "devnet", false, "operate on a devnet network")
	cmd.Flags().BoolVarP(&deployCmdflags.UseFuji, "testnet", "t", false, "operate on testnet (alias to `fuji`)")
	cmd.Flags().BoolVarP(&deployCmdflags.UseFuji, "fuji", "f", false, "operate on fuji (alias to `testnet`")
	cmd.Flags().BoolVarP(&deployCmdflags.UseMainnet, "mainnet", "m", false, "operate on mainnet")
	return cmd
}

func deploy(_ *cobra.Command, args []string) error {
	return DeployWithLocalFlags(nil, args, deployCmdflags)
}

func DeployWithLocalFlags(_ *cobra.Command, args []string, flags DeployCmdFlags) error {
	subnetName := args[0]
	// fix endpoint if available
	if flags.UseDevnet && flags.Endpoint == "" {
		var err error
		flags.Endpoint, err = getDevnetEndpoint(subnetName)
		if err != nil {
			return err
		}
	}
	network, err := subnetcmd.GetNetworkFromCmdLineFlags(
		flags.UseLocal,
		flags.UseDevnet,
		flags.UseFuji,
		flags.UseMainnet,
		flags.Endpoint,
		true,
        "",
		[]models.NetworkKind{models.Local, models.Devnet},
	)
	if err != nil {
		return err
	}
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return fmt.Errorf("failed to load sidecar: %w", err)
	}
	// checks
	if !sc.TeleporterReady {
		return fmt.Errorf("subnet is not configured for teleporter")
	}
	if b, err := subnetcmd.HasSubnetEVMGenesis(subnetName); err != nil {
		return err
	} else if !b {
		return fmt.Errorf("only Subnet-EVM based vms can be used for teleporter")
	}
	if sc.Networks[network.Name()].BlockchainID == ids.Empty {
		return fmt.Errorf("subnet has not been deployed to %s", network.Name())
	}
	// deploy to subnet
	network.UpdateEndpoint(sc.Networks[network.Name()].Endpoint)
	blockchainID := sc.Networks[network.Name()].BlockchainID.String()
	alreadyDeployed, teleporterMessengerAddress, teleporterRegistryAddress, err := teleporter.DeployAndFundRelayer(
		app,
		sc.TeleporterVersion,
		network,
		subnetName,
		blockchainID,
		sc.TeleporterKey,
	)
	if err != nil {
		return err
	}
	if !alreadyDeployed {
		// update sidecar
		networkInfo := sc.Networks[network.Name()]
		networkInfo.TeleporterMessengerAddress = teleporterMessengerAddress
		networkInfo.TeleporterRegistryAddress = teleporterRegistryAddress
		sc.Networks[network.Name()] = networkInfo
		if err := app.UpdateSidecar(&sc); err != nil {
			return err
		}
	}
	// deploy to cchain for local
	if network.Kind == models.Local || network.Kind == models.Devnet {
		blockchainID := "C"
		alreadyDeployed, teleporterMessengerAddress, teleporterRegistryAddress, err = teleporter.DeployAndFundRelayer(
			app,
			sc.TeleporterVersion,
			network,
			"c-chain",
			blockchainID,
			"",
		)
		if err != nil {
			return err
		}
		if network.Kind == models.Local {
			if !alreadyDeployed {
				if err := subnet.WriteExtraLocalNetworkData(app, teleporterMessengerAddress, teleporterRegistryAddress); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
