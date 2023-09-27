// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package nodecmd

import (
	"os"
	"fmt"
	"strings"

	"github.com/ava-labs/avalanche-cli/cmd/subnetcmd"
	"github.com/ava-labs/avalanche-cli/pkg/ansible"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/vms/platformvm/status"
	"github.com/spf13/cobra"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/exp/slices"
)

var subnetName string

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [clusterName]",
		Short: "(ALPHA Warning) Get node bootstrap status",
		Long: `(ALPHA Warning) This command is currently in experimental mode.

The node status command gets the bootstrap status of all nodes in a cluster with the Primary Network. 
To get the bootstrap status of a node with a Subnet, use --subnet flag`,
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE:         statusSubnet,
	}
	cmd.Flags().StringVar(&subnetName, "subnet", "", "specify the subnet the node is syncing with")

	return cmd
}

func statusSubnet(_ *cobra.Command, args []string) error {
	clusterName := args[0]
	if err := checkCluster(clusterName); err != nil {
		return err
	}
	if err := setupAnsible(clusterName); err != nil {
		return err
	}
	ansibleHostIDs, err := ansible.GetAnsibleHostsFromInventory(app.GetAnsibleInventoryDirPath(clusterName))
	if err != nil {
		return err
	}
	if subnetName != "" {
		// check subnet first
		if _, err := subnetcmd.ValidateSubnetNameAndGetChains([]string{subnetName}); err != nil {
			return err
		}
	}
	notBootstrappedNodes, err := checkClusterIsBootstrapped(clusterName)
	if err != nil {
		return err
	}
	if subnetName != "" {
		sc, err := app.LoadSidecar(subnetName)
		if err != nil {
			return err
		}
		blockchainID := sc.Networks[models.Fuji.String()].BlockchainID
		if blockchainID == ids.Empty {
			return ErrNoBlockchainID
		}
		notSyncedNodes := []string{}
		subnetSyncedNodes := []string{}
		subnetValidatingNodes := []string{}
		for _, host := range ansibleHostIDs {
			subnetSyncStatus, err := getNodeSubnetSyncStatus(blockchainID.String(), clusterName, host)
			if err != nil {
				return err
			}
			switch subnetSyncStatus {
			case status.Syncing.String():
				subnetSyncedNodes = append(subnetSyncedNodes, host)
			case status.Validating.String():
				subnetValidatingNodes = append(subnetValidatingNodes, host)
			default:
				notSyncedNodes = append(notSyncedNodes, host)
			}
		}
		printOutput(ansibleHostIDs, notBootstrappedNodes, notSyncedNodes, subnetSyncedNodes, subnetValidatingNodes, clusterName, subnetName)
		return nil
	}
	printOutput(ansibleHostIDs, notBootstrappedNodes, nil, nil, nil, clusterName, subnetName)
	return nil
}

func printOutput(hostAliases, notBootstrappedHosts, notSyncedHosts, subnetSyncedHosts, subnetValidatingHosts []string, clusterName, subnetName string) {
	if subnetName == "" && len(notBootstrappedHosts) == 0 {
		ux.Logger.PrintToUser("All nodes in cluster %s are bootstrapped to Primary Network!", clusterName)
	}
	if subnetName != "" && len(notSyncedHosts) == 0 {
		// all nodes are either synced to or validating subnet
		status := "synced to"
		if len(subnetSyncedHosts) == 0 {
			status = "validators of"
		}
		ux.Logger.PrintToUser("All nodes in cluster %s are %s Subnet %s", clusterName, status, subnetName)
	}

	ux.Logger.PrintToUser("")
	tit := fmt.Sprintf("STATUS FOR CLUSTER: %s", clusterName)
	ux.Logger.PrintToUser(tit)
	ux.Logger.PrintToUser(strings.Repeat("=", len(tit)))
	ux.Logger.PrintToUser("")
	header := []string{"Node", "Primary Network"}
	if subnetName != "" {
		header = append(header, "Subnet " + subnetName)
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader(header)
	table.SetRowLine(true)
	for _, host := range hostAliases {
		boostrappedStatus := logging.Green.Wrap("OK")
		if slices.Contains(notBootstrappedHosts, host) {
			boostrappedStatus = logging.Red.Wrap("ERR")
		}
		row := []string{
			host,
			boostrappedStatus,
		}
		if subnetName != "" {
			syncedStatus := logging.Red.Wrap("ERR")
			if slices.Contains(subnetSyncedHosts, host) {
				syncedStatus = logging.Green.Wrap("Synced")
			}
			if slices.Contains(subnetValidatingHosts, host) {
				syncedStatus = logging.Green.Wrap("Validating")
			}
			row = append(row, syncedStatus)
		}
		table.Append(row)
	}
	table.Render()
}

