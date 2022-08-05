// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package validatorcmd

import (
	"fmt"

	"github.com/ava-labs/avalanche-cli/pkg/application"
	"github.com/spf13/cobra"
)

var app *application.Avalanche

func NewCmd(injectedApp *application.Avalanche) *cobra.Command {
	app = injectedApp

	cmd := &cobra.Command{
		Use:   "validator",
		Short: "Create and manage testnet signing keys",
		Long: `The key command suite provides a collection of tools for creating signing
keys. You can use these keys to deploy subnets to the Fuji testnet.

To get started, use the key create command.`,
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println(err)
			}
		},
	}

	// avalanche key create
	cmd.AddCommand(newStartCmd())

	// avalanche key list
	cmd.AddCommand(newStopCmd())

	// avalanche key delete
	cmd.AddCommand(newStatusCmd())

	// avalanche key delete
	cmd.AddCommand(newInstallCmd())
	return cmd
}
