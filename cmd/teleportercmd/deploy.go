// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package teleportercmd

import (
	"encoding/hex"
	"fmt"

	"github.com/ava-labs/avalanche-cli/cmd/subnetcmd"
	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/key"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/subnet"
	"github.com/ava-labs/avalanche-cli/pkg/teleporter"
	"github.com/ava-labs/avalanchego/ids"

	"github.com/spf13/cobra"
)

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
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "use the given endpoint for network operations")
	cmd.Flags().BoolVarP(&useLocal, "local", "l", false, "operate on a local network")
	cmd.Flags().BoolVar(&useDevnet, "devnet", false, "operate on a devnet network")
	cmd.Flags().BoolVarP(&useFuji, "testnet", "t", false, "operate on testnet (alias to `fuji`)")
	cmd.Flags().BoolVarP(&useFuji, "fuji", "f", false, "operate on fuji (alias to `testnet`")
	cmd.Flags().BoolVarP(&useMainnet, "mainnet", "m", false, "operate on mainnet")
	return cmd
}

func deploy(_ *cobra.Command, args []string) error {
	network, err := subnetcmd.GetNetworkFromCmdLineFlags(
		useLocal,
		useDevnet,
		useFuji,
		useMainnet,
		"",
		false,
		[]models.NetworkKind{models.Local},
	)
	if err != nil {
		return err
	}
	subnetName := args[0]
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
	// get deploy key
	keyPath := app.GetKeyPath(sc.TeleporterKey)
	k, err := key.LoadSoft(network.ID, keyPath)
	if err != nil {
		return err
	}
	privKeyStr := hex.EncodeToString(k.Raw())
	// deploy to subnet
	td := teleporter.Deployer{}
	blockchainID := sc.Networks[network.Name()].BlockchainID
	endpoint := network.BlockchainEndpoint(blockchainID.String())
	alreadyDeployed, teleporterMessengerAddress, teleporterRegistryAddress, err := td.Deploy(
		app.GetTeleporterBinDir(),
		sc.TeleporterVersion,
		subnetName,
		endpoint,
		privKeyStr,
	)
	if err != nil {
		return err
	}
	// get relayer address to fund
	relayerAddress, _, err := teleporter.GetRelayerKeyInfo(app.GetKeyPath(constants.AWMRelayerKeyName))
	if err != nil {
		return err
	}
	if !alreadyDeployed {
		// fund relayer
		if err := teleporter.FundRelayer(
			endpoint,
			privKeyStr,
			relayerAddress,
		); err != nil {
			return err
		}
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
	if network.Kind == models.Local {
		k, err := key.LoadEwoq(network.ID)
		if err != nil {
			return err
		}
		privKeyStr := hex.EncodeToString(k.Raw())
		endpoint := network.CChainEndpoint()
		alreadyDeployed, cchainTeleporterMessengerAddress, cchainTeleporterRegistryAddress, err := td.Deploy(
			app.GetTeleporterBinDir(),
			sc.TeleporterVersion,
			"c-chain",
			endpoint,
			privKeyStr,
		)
		if err != nil {
			return err
		}
		if !alreadyDeployed {
			// fund relayer
			if err := teleporter.FundRelayer(
				endpoint,
				privKeyStr,
				relayerAddress,
			); err != nil {
				return err
			}
			if err := subnet.WriteExtraLocalNetworkData(app, cchainTeleporterMessengerAddress, cchainTeleporterRegistryAddress); err != nil {
				return err
			}
		}
	}
	return nil
}
