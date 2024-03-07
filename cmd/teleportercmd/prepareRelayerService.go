// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package teleportercmd

import (
	_ "embed"
	"fmt"
	"os/user"
	"os"
	"path/filepath"

	"github.com/ava-labs/avalanche-cli/pkg/teleporter"
	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/spf13/cobra"
)

//go:embed awm-relayer.service
var awmRelayerServiceTemplate []byte

// avalanche teleporter msg
func newPrepareRelayerServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "prepareService",
		Short:        "Installs AWM relayer as a service",
		Long:         `Installs AWM relayer as a service. Disabled by default.`,
		SilenceUsage: true,
		RunE:         prepareRelayerService,
		Args:         cobra.ExactArgs(0),
	}
	return cmd
}

func prepareRelayerService(_ *cobra.Command, args []string) error {
	relayerBin, err := teleporter.InstallRelayer(app.GetAWMRelayerBinDir())
        usr, err := user.Current()
        if err != nil {
		return err
        }
	awmRelayerServicesDir := app.GetAWMRelayerServiceDir()
	if err := os.MkdirAll(awmRelayerServicesDir, constants.DefaultPerms755); err != nil {
		return err
	}
	awmRelayerConfigPath := filepath.Join(awmRelayerServicesDir, constants.AWMRelayerConfigFilename)
	if err := os.WriteFile(awmRelayerConfigPath, []byte{}, constants.WriteReadReadPerms); err != nil {
		return err
	}
	awmRelayerServicePath := filepath.Join(awmRelayerServicesDir, "awm-relayer.service")
	awmRelayerServiceConf := fmt.Sprintf(string(awmRelayerServiceTemplate), usr.Username, usr.HomeDir, relayerBin, awmRelayerConfigPath)
	if err := os.WriteFile(awmRelayerServicePath, []byte(awmRelayerServiceConf), constants.WriteReadReadPerms); err != nil {
		return err
	}
	return err
}
