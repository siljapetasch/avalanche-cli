// Copyright (C) 2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package docker

import (
	"os"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/remoteconfig"
)

func prepareAvalanchegoConfig(host *models.Host, networkID string) (string, string, error) {
	avagoConf := remoteconfig.PrepareAvalancheConfig(host.IP, networkID, nil)
	nodeConf, err := remoteconfig.RenderAvalancheNodeConfig(avagoConf)
	if err != nil {
		return "", "", err
	}
	nodeConfFile, err := os.CreateTemp("", "avalanchecli-node-*.yml")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(nodeConfFile.Name(), nodeConf, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", err
	}
	cChainConf, err := remoteconfig.RenderAvalancheCChainConfig(avagoConf)
	if err != nil {
		return "", "", err
	}
	cChainConfFile, err := os.CreateTemp("", "avalanchecli-cchain-*.yml")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(cChainConfFile.Name(), cChainConf, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", err
	}
	return nodeConfFile.Name(), cChainConfFile.Name(), nil
}

func prepareGrafanaConfig() (string, string, string, string, error) {
	grafanaDataSource, err := remoteconfig.RenderGrafanaLokiDataSourceConfig()
	if err != nil {
		return "", "", "", "", err
	}
	grafanaDataSourceFile, err := os.CreateTemp("", "avalanchecli-grafana-datasource-*.yml")
	if err != nil {
		return "", "", "", "", err
	}
	if err := os.WriteFile(grafanaDataSourceFile.Name(), grafanaDataSource, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", "", "", err
	}

	grafanaPromDataSource, err := remoteconfig.RenderGrafanaPrometheusDataSourceConfigg()
	if err != nil {
		return "", "", "", "", err
	}
	grafanaPromDataSourceFile, err := os.CreateTemp("", "avalanchecli-grafana-prom-datasource-*.yml")
	if err != nil {
		return "", "", "", "", err
	}
	if err := os.WriteFile(grafanaPromDataSourceFile.Name(), grafanaPromDataSource, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", "", "", err
	}

	grafanaDashboards, err := remoteconfig.RenderGrafanaDashboardConfig()
	if err != nil {
		return "", "", "", "", err
	}
	grafanaDashboardsFile, err := os.CreateTemp("", "avalanchecli-grafana-dashboards-*.yml")
	if err != nil {
		return "", "", "", "", err
	}
	if err := os.WriteFile(grafanaDashboardsFile.Name(), grafanaDashboards, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", "", "", err
	}

	grafanaConfig, err := remoteconfig.RenderGrafanaConfig()
	if err != nil {
		return "", "", "", "", err
	}
	grafanaConfigFile, err := os.CreateTemp("", "avalanchecli-grafana-config-*.ini")
	if err != nil {
		return "", "", "", "", err
	}
	if err := os.WriteFile(grafanaConfigFile.Name(), grafanaConfig, constants.WriteReadUserOnlyPerms); err != nil {
		return "", "", "", "", err
	}
	return grafanaConfigFile.Name(), grafanaDashboardsFile.Name(), grafanaDataSourceFile.Name(), grafanaPromDataSourceFile.Name(), nil
}
