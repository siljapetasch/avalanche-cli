// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package teleportercmd

import (
	"fmt"

	"github.com/ava-labs/avalanche-cli/pkg/teleporter"
	"github.com/spf13/cobra"
)

// avalanche teleporter msg
func newRelayerInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Installs AWM relayer as a service",
		Long:         `Installs AWM relayer as a service. Disabled by default.`,
		SilenceUsage: true,
		RunE:         relayerInstall,
		Args:         cobra.ExactArgs(0),
	}
	return cmd
}

func relayerInstall(_ *cobra.Command, args []string) error {
	relayerBin, err := teleporter.InstallRelayer(app.GetAWMRelayerBinDir())
	fmt.Println(relayerBin)
	return err
}
