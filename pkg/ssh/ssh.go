// Copyright (C) 2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package ssh

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/ava-labs/avalanche-cli/pkg/application"
	"github.com/ava-labs/avalanche-cli/pkg/binutils"
	"github.com/ava-labs/avalanche-cli/pkg/docker"
	"github.com/ava-labs/avalanche-cli/pkg/monitoring"
	"github.com/ava-labs/avalanche-cli/pkg/remoteconfig"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/ava-labs/avalanchego/ids"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/models"
)

type scriptInputs struct {
	AvalancheGoVersion      string
	SubnetExportFileName    string
	SubnetName              string
	ClusterName             string
	GoVersion               string
	CliBranch               string
	IsDevNet                bool
	IsE2E                   bool
	NetworkFlag             string
	VMBinaryPath            string
	SubnetEVMReleaseURL     string
	SubnetEVMArchive        string
	MonitoringDashboardPath string
	LoadTestRepoDir         string
	LoadTestRepo            string
	LoadTestPath            string
	LoadTestCommand         string
	LoadTestBranch          string
	LoadTestGitCommit       string
	CheckoutCommit          bool
	LoadTestResultFile      string
	GrafanaPkg              string
	CustomVMRepoDir         string
	CustomVMRepoURL         string
	CustomVMBranch          string
	CustomVMBuildScript     string
}

//go:embed shell/*.sh
var script embed.FS

// RunOverSSH runs provided script path over ssh.
// This script can be template as it will be rendered using scriptInputs vars
func RunOverSSH(
	scriptDesc string,
	host *models.Host,
	timeout time.Duration,
	scriptPath string,
	templateVars scriptInputs,
) error {
	startTime := time.Now()
	shellScript, err := script.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	var script bytes.Buffer
	t, err := template.New(scriptDesc).Parse(string(shellScript))
	if err != nil {
		return err
	}
	err = t.Execute(&script, templateVars)
	if err != nil {
		return err
	}

	if output, err := host.Command(script.String(), nil, timeout); err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	executionTime := time.Since(startTime)
	ux.Logger.Info("RunOverSSH[%s]%s took %s with err: %v", host.NodeID, scriptDesc, executionTime, err)
	return nil
}

func PostOverSSH(host *models.Host, path string, requestBody string) ([]byte, error) {
	if path == "" {
		path = "/ext/info"
	}
	localhost, err := url.Parse(constants.LocalAPIEndpoint)
	if err != nil {
		return nil, err
	}
	requestHeaders := fmt.Sprintf("POST %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Content-Length: %d\r\n"+
		"Content-Type: application/json\r\n\r\n", path, localhost.Host, len(requestBody))
	httpRequest := requestHeaders + requestBody
	return host.Forward(httpRequest, constants.SSHPOSTTimeout)
}

// RunSSHSetupNode runs script to setup node
func RunSSHSetupNode(host *models.Host, configPath string) error {
	if err := RunOverSSH(
		"Setup Node",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupNode.sh",
		scriptInputs{IsE2E: utils.IsE2E()},
	); err != nil {
		return err
	}
	// name: copy metrics config to cloud server
	ux.Logger.Info("Uploading config %s to server %s: %s", configPath, host.NodeID, filepath.Join(constants.CloudNodeCLIConfigBasePath, filepath.Base(configPath)))
	if err := host.Upload(
		configPath,
		filepath.Join(constants.CloudNodeCLIConfigBasePath, filepath.Base(configPath)),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return nil
}

// RunSSHSetupDockerService runs script to setup docker compose service for CLI
func RunSSHSetupDockerService(host *models.Host) error {
	if host.IsSystemD() {
		return RunOverSSH(
			"Setup Docker Service",
			host,
			constants.SSHLongRunningScriptTimeout,
			"shell/setupDockerService.sh",
			scriptInputs{},
		)
	} else {
		// no need to setup docker service
		return nil
	}
}

// RunSSHRestartNode runs script to restart avalanchego
func RunSSHRestartNode(host *models.Host) error {
	remoteComposeFile := utils.GetRemoteComposeFile()
	avagoService := "avalanchego"
	if utils.IsE2E() {
		avagoService += utils.E2ESuffix(host.IP)
	}
	return docker.RestartDockerComposeService(host, remoteComposeFile, avagoService, constants.SSHLongRunningScriptTimeout)
}

// ComposeSSHSetupAWMRelayer used docker compose to setup AWM Relayer
func ComposeSSHSetupAWMRelayer(host *models.Host) error {
	if err := docker.ComposeSSHSetupAWMRelayer(host); err != nil {
		return err
	}
	return docker.StartDockerComposeService(host, utils.GetRemoteComposeFile(), "awm-relayer", constants.SSHLongRunningScriptTimeout)
}

// RunSSHStartAWMRelayerService runs script to start an AWM Relayer Service
func RunSSHStartAWMRelayerService(host *models.Host) error {
	return docker.StartDockerComposeService(host, utils.GetRemoteComposeFile(), "awm-relayer", constants.SSHLongRunningScriptTimeout)
}

// RunSSHStopAWMRelayerService runs script to start an AWM Relayer Service
func RunSSHStopAWMRelayerService(host *models.Host) error {
	return docker.StopDockerComposeService(host, utils.GetRemoteComposeFile(), "awm-relayer", constants.SSHLongRunningScriptTimeout)
}

// RunSSHUpgradeAvalanchego runs script to upgrade avalanchego
func RunSSHUpgradeAvalanchego(host *models.Host, network models.Network, avalancheGoVersion string) error {
	withMonitoring, err := docker.WasNodeSetupWithMonitoring(host)
	if err != nil {
		return err
	}

	if err := docker.ComposeSSHSetupNode(host, network, avalancheGoVersion, withMonitoring); err != nil {
		return err
	}
	return docker.RestartDockerCompose(host, constants.SSHLongRunningScriptTimeout)
}

// RunSSHStartNode runs script to start avalanchego
func RunSSHStartNode(host *models.Host) error {
	if utils.IsE2E() && utils.E2EDocker() {
		return RunOverSSH(
			"E2E Start Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_startNode.sh",
			scriptInputs{},
		)
	}
	return docker.StartDockerComposeService(host, utils.GetRemoteComposeFile(), "avalanchego", constants.SSHLongRunningScriptTimeout)
}

// RunSSHStopNode runs script to stop avalanchego
func RunSSHStopNode(host *models.Host) error {
	if utils.IsE2E() && utils.E2EDocker() {
		return RunOverSSH(
			"E2E Stop Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_stopNode.sh",
			scriptInputs{},
		)
	}
	return docker.StopDockerComposeService(host, utils.GetRemoteComposeFile(), "avalanchego", constants.SSHLongRunningScriptTimeout)
}

// RunSSHUpgradeSubnetEVM runs script to upgrade subnet evm
func RunSSHUpgradeSubnetEVM(host *models.Host, subnetEVMBinaryPath string) error {
	return RunOverSSH(
		"Upgrade Subnet EVM",
		host,
		constants.SSHScriptTimeout,
		"shell/upgradeSubnetEVM.sh",
		scriptInputs{VMBinaryPath: subnetEVMBinaryPath},
	)
}

func replaceCustomVarDashboardValues(customGrafanaDashboardFileName, chainID string) error {
	content, err := os.ReadFile(customGrafanaDashboardFileName)
	if err != nil {
		return err
	}
	replacements := []struct {
		old string
		new string
	}{
		{"\"text\": \"CHAIN_ID_VAL\"", fmt.Sprintf("\"text\": \"%v\"", chainID)},
		{"\"value\": \"CHAIN_ID_VAL\"", fmt.Sprintf("\"value\": \"%v\"", chainID)},
		{"\"query\": \"CHAIN_ID_VAL\"", fmt.Sprintf("\"query\": \"%v\"", chainID)},
	}
	for _, r := range replacements {
		content = []byte(strings.ReplaceAll(string(content), r.old, r.new))
	}
	err = os.WriteFile(customGrafanaDashboardFileName, content, constants.WriteReadUserOnlyPerms)
	if err != nil {
		return err
	}
	return nil
}

func RunSSHUpdateMonitoringDashboards(host *models.Host, monitoringDashboardPath, customGrafanaDashboardPath, chainID string) error {
	remoteDashboardsPath := utils.GetRemoteComposeServicePath("grafana", "dashboards")
	if !utils.DirectoryExists(monitoringDashboardPath) {
		return fmt.Errorf("%s does not exist", monitoringDashboardPath)
	}
	if customGrafanaDashboardPath != "" && utils.FileExists(utils.ExpandHome(customGrafanaDashboardPath)) {
		if err := utils.FileCopy(utils.ExpandHome(customGrafanaDashboardPath), filepath.Join(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON)); err != nil {
			return err
		}
		if err := replaceCustomVarDashboardValues(filepath.Join(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON), chainID); err != nil {
			return err
		}
	}
	if err := host.MkdirAll(remoteDashboardsPath, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON),
		filepath.Join(remoteDashboardsPath, constants.CustomGrafanaDashboardJSON),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return docker.RestartDockerComposeService(host, utils.GetRemoteComposeFile(), "grafana", constants.SSHLongRunningScriptTimeout)
}

func RunSSHSetupMonitoringFolders(host *models.Host) error {
	for _, folder := range remoteconfig.RemoteFoldersToCreateMonitoring() {
		if err := host.MkdirAll(folder, constants.SSHDirOpsTimeout); err != nil {
			return err
		}
	}
	return nil
}

func RunSSHCopyMonitoringDashboards(host *models.Host, monitoringDashboardPath string) error {
	// TODO: download dashboards from github instead
	remoteDashboardsPath := utils.GetRemoteComposeServicePath("grafana", "dashboards")
	if !utils.DirectoryExists(monitoringDashboardPath) {
		return fmt.Errorf("%s does not exist", monitoringDashboardPath)
	}
	if err := host.MkdirAll(remoteDashboardsPath, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	dashboards, err := os.ReadDir(monitoringDashboardPath)
	if err != nil {
		return err
	}
	for _, dashboard := range dashboards {
		if err := host.Upload(
			filepath.Join(monitoringDashboardPath, dashboard.Name()),
			filepath.Join(remoteDashboardsPath, dashboard.Name()),
			constants.SSHFileOpsTimeout,
		); err != nil {
			return err
		}
	}
	if composeFileExists(host) {
		return docker.RestartDockerComposeService(host, utils.GetRemoteComposeFile(), "grafana", constants.SSHLongRunningScriptTimeout)
	} else {
		return nil
	}
}

func RunSSHCopyYAMLFile(host *models.Host, yamlFilePath string) error {
	if err := host.Upload(
		yamlFilePath,
		fmt.Sprintf("/home/ubuntu/%s", filepath.Base(yamlFilePath)),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return nil
}

func RunSSHSetupPrometheusConfig(host *models.Host, avalancheGoPorts, machinePorts, loadTestPorts []string) error {
	for _, folder := range remoteconfig.PrometheusFoldersToCreate() {
		if err := host.MkdirAll(folder, constants.SSHDirOpsTimeout); err != nil {
			return err
		}
	}
	cloudNodePrometheusConfigTemp := utils.GetRemoteComposeServicePath("prometheus", "prometheus.yml")
	promConfig, err := os.CreateTemp("", "prometheus")
	if err != nil {
		return err
	}
	defer os.Remove(promConfig.Name())
	if err := monitoring.WritePrometheusConfig(promConfig.Name(), avalancheGoPorts, machinePorts, loadTestPorts); err != nil {
		return err
	}

	return host.Upload(
		promConfig.Name(),
		cloudNodePrometheusConfigTemp,
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHSetupLokiConfig(host *models.Host, port int) error {
	for _, folder := range remoteconfig.LokiFoldersToCreate() {
		if err := host.MkdirAll(folder, constants.SSHDirOpsTimeout); err != nil {
			return err
		}
	}
	cloudNodeLokiConfigTemp := utils.GetRemoteComposeServicePath("loki", "loki.yml")
	lokiConfig, err := os.CreateTemp("", "loki")
	if err != nil {
		return err
	}
	defer os.Remove(lokiConfig.Name())
	if err := monitoring.WriteLokiConfig(lokiConfig.Name(), strconv.Itoa(port)); err != nil {
		return err
	}
	return host.Upload(
		lokiConfig.Name(),
		cloudNodeLokiConfigTemp,
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHSetupPromtailConfig(host *models.Host, lokiIP string, lokiPort int, cloudID string, nodeID string, chainID string) error {
	for _, folder := range remoteconfig.PromtailFoldersToCreate() {
		if err := host.MkdirAll(folder, constants.SSHDirOpsTimeout); err != nil {
			return err
		}
	}
	cloudNodePromtailConfigTemp := utils.GetRemoteComposeServicePath("promtail", "promtail.yml")
	promtailConfig, err := os.CreateTemp("", "promtail")
	if err != nil {
		return err
	}
	defer os.Remove(promtailConfig.Name())

	if err := monitoring.WritePromtailConfig(promtailConfig.Name(), lokiIP, strconv.Itoa(lokiPort), cloudID, nodeID, chainID); err != nil {
		return err
	}
	return host.Upload(
		promtailConfig.Name(),
		cloudNodePromtailConfigTemp,
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHDownloadNodePrometheusConfig(host *models.Host, nodeInstanceDirPath string) error {
	return host.Download(
		constants.CloudNodePrometheusConfigPath,
		filepath.Join(nodeInstanceDirPath, constants.NodePrometheusConfigFileName),
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHUploadNodeAWMRelayerConfig(host *models.Host, nodeInstanceDirPath string) error {
	cloudAWMRelayerConfigDir := filepath.Join(constants.CloudNodeCLIConfigBasePath, constants.ServicesDir, constants.AWMRelayerInstallDir)
	if err := host.MkdirAll(cloudAWMRelayerConfigDir, constants.SSHDirOpsTimeout); err != nil {
		return err
	}
	return host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.ServicesDir, constants.AWMRelayerInstallDir, constants.AWMRelayerConfigFilename),
		filepath.Join(cloudAWMRelayerConfigDir, constants.AWMRelayerConfigFilename),
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHGetNewSubnetEVMRelease runs script to download new subnet evm
func RunSSHGetNewSubnetEVMRelease(host *models.Host, subnetEVMReleaseURL, subnetEVMArchive string) error {
	return RunOverSSH(
		"Get Subnet EVM Release",
		host,
		constants.SSHScriptTimeout,
		"shell/getNewSubnetEVMRelease.sh",
		scriptInputs{SubnetEVMReleaseURL: subnetEVMReleaseURL, SubnetEVMArchive: subnetEVMArchive},
	)
}

// RunSSHSetupDevNet runs script to setup devnet
func RunSSHSetupDevNet(host *models.Host, nodeInstanceDirPath string) error {
	if err := host.MkdirAll(
		constants.CloudNodeConfigPath,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.GenesisFileName),
		filepath.Join(constants.CloudNodeConfigPath, constants.GenesisFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.NodeFileName),
		filepath.Join(constants.CloudNodeConfigPath, constants.NodeFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	if err := docker.StopDockerCompose(host, constants.SSHLongRunningScriptTimeout); err != nil {
		return err
	}
	if err := host.Remove("/home/ubuntu/.avalanchego/db", true); err != nil {
		return err
	}
	if err := host.MkdirAll("/home/ubuntu/.avalanchego/db", constants.SSHDirOpsTimeout); err != nil {
		return err
	}
	if err := host.Remove("/home/ubuntu/.avalanchego/logs", true); err != nil {
		return err
	}
	if err := host.MkdirAll("/home/ubuntu/.avalanchego/logs", constants.SSHDirOpsTimeout); err != nil {
		return err
	}
	return docker.StartDockerCompose(host, constants.SSHLongRunningScriptTimeout)
}

// RunSSHUploadStakingFiles uploads staking files to a remote host via SSH.
func RunSSHUploadStakingFiles(host *models.Host, nodeInstanceDirPath string) error {
	if err := host.MkdirAll(
		constants.CloudNodeStakingPath,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.StakerCertFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.StakerCertFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.StakerKeyFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.StakerKeyFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.BLSKeyFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.BLSKeyFileName),
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHRenderAvalancheNodeConfig renders avalanche node config to a remote host via SSH.
func RunSSHRenderAvalancheNodeConfig(app *application.Avalanche, host *models.Host, network models.Network, trackSubnets []string) error {
	// get subnet ids
	subnetIDs, err := utils.MapWithError(trackSubnets, func(subnetName string) (string, error) {
		sc, err := app.LoadSidecar(subnetName)
		if err != nil {
			return "", err
		} else {
			return sc.Networks[network.Name()].SubnetID.String(), nil
		}
	})
	if err != nil {
		return err
	}

	nodeConfFile, err := os.CreateTemp("", "avalanchecli-node-*.yml")
	if err != nil {
		return err
	}
	defer os.Remove(nodeConfFile.Name())

	avagoConf := remoteconfig.PrepareAvalancheConfig(host.IP, network.NetworkIDFlagValue(), subnetIDs)
	// make sure that genesis and bootstrap data is preserved
	if genesisFileExists(host) {
		avagoConf.GenesisPath = filepath.Join(constants.DockerNodeConfigPath, constants.GenesisFileName)
	}
	remoteAvagoConfFile, err := getAvalancheGoConfigData(host)
	if err != nil {
		return err
	}
	bootstrapIDs, _ := utils.GetValueString(remoteAvagoConfFile, "bootstrap-ids")
	bootstrapIPs, _ := utils.GetValueString(remoteAvagoConfFile, "bootstrap-ips")
	avagoConf.BootstrapIDs = bootstrapIDs
	avagoConf.BootstrapIPs = bootstrapIPs
	// ready to render node config
	nodeConf, err := remoteconfig.RenderAvalancheNodeConfig(avagoConf)
	if err != nil {
		return err
	}
	if err := os.WriteFile(nodeConfFile.Name(), nodeConf, constants.WriteReadUserOnlyPerms); err != nil {
		return err
	}
	return host.Upload(nodeConfFile.Name(), remoteconfig.GetRemoteAvalancheNodeConfig(), constants.SSHFileOpsTimeout)
}

// RunSSHCreatePlugin runs script to create plugin
func RunSSHCreatePlugin(host *models.Host, sc models.Sidecar) error {
	vmID, err := sc.GetVMID()
	if err != nil {
		return err
	}
	subnetVMBinaryPath := fmt.Sprintf(constants.CloudNodeSubnetEvmBinaryPath, vmID)
	hostInstaller := NewHostInstaller(host)
	tmpDir, err := host.CreateTempDir()
	if err != nil {
		return err
	}
	defer func(h *models.Host) {
		_ = h.Remove(tmpDir, true)
	}(host)
	switch {
	case sc.VM == models.CustomVM:
		if err := RunOverSSH(
			"Build CustomVM",
			host,
			constants.SSHLongRunningScriptTimeout,
			"shell/buildCustomVM.sh",
			scriptInputs{
				CustomVMRepoDir:     tmpDir,
				CustomVMRepoURL:     sc.CustomVMRepoURL,
				CustomVMBranch:      sc.CustomVMBranch,
				CustomVMBuildScript: sc.CustomVMBuildScript,
				VMBinaryPath:        subnetVMBinaryPath,
			},
		); err != nil {
			return err
		}

	case sc.VM == models.SubnetEvm:
		dl := binutils.NewSubnetEVMDownloader()
		installURL, _, err := dl.GetDownloadURL(sc.VMVersion, hostInstaller) // extension is tar.gz
		if err != nil {
			return err
		}

		archiveName := "subnet-evm.tar.gz"
		archiveFullPath := filepath.Join(tmpDir, archiveName)

		// download and install subnet evm
		if _, err := host.Command(fmt.Sprintf("%s %s -O %s", "busybox wget", installURL, archiveFullPath), nil, constants.SSHLongRunningScriptTimeout); err != nil {
			return err
		}
		if _, err := host.Command(fmt.Sprintf("tar -xzf %s -C %s", archiveFullPath, tmpDir), nil, constants.SSHLongRunningScriptTimeout); err != nil {
			return err
		}

		if _, err := host.Command(fmt.Sprintf("mv -f %s/subnet-evm %s", tmpDir, subnetVMBinaryPath), nil, constants.SSHLongRunningScriptTimeout); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected error: unsupported VM type: %s", sc.VM)
	}

	return nil
}

// RunSSHMergeSubnetNodeConfig merges subnet node config to the node config on the remote host
func mergeSubnetNodeConfig(host *models.Host, subnetNodeConfigPath string) error {
	if subnetNodeConfigPath == "" {
		return fmt.Errorf("subnet node config path is empty")
	}
	tmpFile, err := os.CreateTemp("", "avalanchecli-subnet-node-*.yml")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	if err := host.Download(remoteconfig.GetRemoteAvalancheNodeConfig(), tmpFile.Name(), constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	remoteNodeConfigBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("error reading remote node config: %w", err)
	}
	var remoteNodeConfig map[string]interface{}
	if err := json.Unmarshal(remoteNodeConfigBytes, &remoteNodeConfig); err != nil {
		return fmt.Errorf("error unmarshalling remote node config: %w", err)
	}
	subnetNodeConfigBytes, err := os.ReadFile(subnetNodeConfigPath)
	if err != nil {
		return fmt.Errorf("error reading subnet node config: %w", err)
	}
	var subnetNodeConfig map[string]interface{}
	if err := json.Unmarshal(subnetNodeConfigBytes, &subnetNodeConfig); err != nil {
		return fmt.Errorf("error unmarshalling subnet node config: %w", err)
	}
	mergedNodeConfig := utils.MergeJSONMaps(remoteNodeConfig, subnetNodeConfig)
	mergedNodeConfigBytes, err := json.Marshal(mergedNodeConfig)
	if err != nil {
		return fmt.Errorf("error creating merged node config: %w", err)
	}
	if err := os.WriteFile(tmpFile.Name(), mergedNodeConfigBytes, constants.WriteReadUserOnlyPerms); err != nil {
		return err
	}
	return host.Upload(tmpFile.Name(), remoteconfig.GetRemoteAvalancheNodeConfig(), constants.SSHFileOpsTimeout)
}

// RunSSHSyncSubnetData syncs subnet data required
func RunSSHSyncSubnetData(app *application.Avalanche, host *models.Host, network models.Network, subnetName string) error {
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}
	subnetID := sc.Networks[network.Name()].SubnetID
	if subnetID == ids.Empty {
		return errors.New("subnet id is empty")
	}
	subnetIDStr := subnetID.String()
	blockchainID := sc.Networks[network.Name()].BlockchainID
	// genesis config
	genesisFilename := filepath.Join(app.GetNodesDir(), host.GetCloudID(), constants.GenesisFileName)
	if err := host.Upload(genesisFilename, remoteconfig.GetRemoteAvalancheGenesis(), constants.SSHFileOpsTimeout); err != nil {
		return fmt.Errorf("error uploading genesis config to %s: %w", remoteconfig.GetRemoteAvalancheGenesis(), err)
	}
	// end genesis config
	// subnet node config
	subnetNodeConfigPath := app.GetAvagoNodeConfigPath(subnetName)
	if utils.FileExists(subnetNodeConfigPath) {
		if err := mergeSubnetNodeConfig(host, subnetNodeConfigPath); err != nil {
			return err
		}
	}
	// subnet config
	if app.AvagoSubnetConfigExists(subnetName) {
		subnetConfig, err := app.LoadRawAvagoSubnetConfig(subnetName)
		if err != nil {
			return fmt.Errorf("error loading subnet config: %w", err)
		}
		subnetConfigFile, err := os.CreateTemp("", "avalanchecli-subnet-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(subnetConfigFile.Name())
		if err := os.WriteFile(subnetConfigFile.Name(), subnetConfig, constants.WriteReadUserOnlyPerms); err != nil {
			return err
		}
		subnetConfigPath := filepath.Join(constants.CloudNodeConfigPath, "subnets", subnetIDStr+".json")
		if err := host.MkdirAll(filepath.Dir(subnetConfigPath), constants.SSHDirOpsTimeout); err != nil {
			return err
		}
		if err := host.Upload(subnetConfigFile.Name(), subnetConfigPath, constants.SSHFileOpsTimeout); err != nil {
			return fmt.Errorf("error uploading subnet config to %s: %w", subnetConfigPath, err)
		}
	}
	// end subnet config

	// chain config
	if blockchainID != ids.Empty && app.ChainConfigExists(subnetName) {
		chainConfigFile, err := os.CreateTemp("", "avalanchecli-chain-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(chainConfigFile.Name())
		chainConfig, err := app.LoadRawChainConfig(subnetName)
		if err != nil {
			return fmt.Errorf("error loading chain config: %w", err)
		}
		if err := os.WriteFile(chainConfigFile.Name(), chainConfig, constants.WriteReadUserOnlyPerms); err != nil {
			return err
		}
		chainConfigPath := filepath.Join(constants.CloudNodeConfigPath, "chains", blockchainID.String(), "config.json")
		if err := host.MkdirAll(filepath.Dir(chainConfigPath), constants.SSHDirOpsTimeout); err != nil {
			return err
		}
		if err := host.Upload(chainConfigFile.Name(), chainConfigPath, constants.SSHFileOpsTimeout); err != nil {
			return fmt.Errorf("error uploading chain config to %s: %w", chainConfigPath, err)
		}
	}
	// end chain config

	// network upgrade
	if app.NetworkUpgradeExists(subnetName) {
		networkUpgradesFile, err := os.CreateTemp("", "avalanchecli-network-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(networkUpgradesFile.Name())
		networkUpgrades, err := app.LoadRawNetworkUpgrades(subnetName)
		if err != nil {
			return fmt.Errorf("error loading network upgrades: %w", err)
		}
		if err := os.WriteFile(networkUpgradesFile.Name(), networkUpgrades, constants.WriteReadUserOnlyPerms); err != nil {
			return err
		}
		networkUpgradesPath := filepath.Join(constants.CloudNodeConfigPath, "subnets", "chains", blockchainID.String(), "upgrade.json")
		if err := host.MkdirAll(filepath.Dir(networkUpgradesPath), constants.SSHDirOpsTimeout); err != nil {
			return err
		}
		if err := host.Upload(networkUpgradesFile.Name(), networkUpgradesPath, constants.SSHFileOpsTimeout); err != nil {
			return fmt.Errorf("error uploading network upgrades to %s: %w", networkUpgradesPath, err)
		}
	}
	// end network upgrade

	return nil
}

func RunSSHBuildLoadTestCode(host *models.Host, loadTestRepo, loadTestPath, loadTestGitCommit, repoDirName, loadTestBranch string, checkoutCommit bool) error {
	return StreamOverSSH(
		"Build Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/buildLoadTest.sh",
		scriptInputs{
			LoadTestRepoDir: repoDirName,
			LoadTestRepo:    loadTestRepo, LoadTestPath: loadTestPath, LoadTestGitCommit: loadTestGitCommit,
			CheckoutCommit: checkoutCommit, LoadTestBranch: loadTestBranch,
		},
	)
}

func RunSSHBuildLoadTestDependencies(host *models.Host) error {
	return RunOverSSH(
		"Build Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/buildLoadTestDeps.sh",
		scriptInputs{GoVersion: constants.BuildEnvGolangVersion},
	)
}

func RunSSHRunLoadTest(host *models.Host, loadTestCommand, loadTestName string) error {
	return RunOverSSH(
		"Run Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/runLoadTest.sh",
		scriptInputs{
			GoVersion:          constants.BuildEnvGolangVersion,
			LoadTestCommand:    loadTestCommand,
			LoadTestResultFile: fmt.Sprintf("/home/ubuntu/.avalanchego/logs/loadtest_%s.txt", loadTestName),
		},
	)
}

// RunSSHSetupCLIFromSource installs any CLI branch from source
func RunSSHSetupCLIFromSource(host *models.Host, cliBranch string) error {
	if !constants.EnableSetupCLIFromSource {
		return nil
	}
	timeout := constants.SSHLongRunningScriptTimeout
	if utils.IsE2E() && utils.E2EDocker() {
		timeout = 10 * time.Minute
	}
	return RunOverSSH(
		"Setup CLI From Source",
		host,
		timeout,
		"shell/setupCLIFromSource.sh",
		scriptInputs{CliBranch: cliBranch},
	)
}

// RunSSHCheckAvalancheGoVersion checks node avalanchego version
func RunSSHCheckAvalancheGoVersion(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.getNodeVersion\"}"
	return PostOverSSH(host, "", requestBody)
}

// RunSSHCheckBootstrapped checks if node is bootstrapped to primary network
func RunSSHCheckBootstrapped(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.isBootstrapped\", \"params\": {\"chain\":\"X\"}}"
	return PostOverSSH(host, "", requestBody)
}

// RunSSHCheckHealthy checks if node is healthy
func RunSSHCheckHealthy(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\":\"health.health\",\"params\": {\"tags\": [\"P\"]}}"
	return PostOverSSH(host, "/ext/health", requestBody)
}

// RunSSHGetNodeID reads nodeID from avalanchego
func RunSSHGetNodeID(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.getNodeID\"}"
	return PostOverSSH(host, "", requestBody)
}

// SubnetSyncStatus checks if node is synced to subnet
func RunSSHSubnetSyncStatus(host *models.Host, blockchainID string) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := fmt.Sprintf("{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"platform.getBlockchainStatus\", \"params\": {\"blockchainID\":\"%s\"}}", blockchainID)
	return PostOverSSH(host, "/ext/bc/P", requestBody)
}

// StreamOverSSH runs provided script path over ssh.
// This script can be template as it will be rendered using scriptInputs vars
func StreamOverSSH(
	scriptDesc string,
	host *models.Host,
	timeout time.Duration,
	scriptPath string,
	templateVars scriptInputs,
) error {
	shellScript, err := script.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	var script bytes.Buffer
	t, err := template.New(scriptDesc).Parse(string(shellScript))
	if err != nil {
		return err
	}
	err = t.Execute(&script, templateVars)
	if err != nil {
		return err
	}

	if err := host.StreamSSHCommand(script.String(), nil, timeout); err != nil {
		return err
	}
	return nil
}

// RunSSHWhitelistPubKey downloads the authorized_keys file from the specified host, appends the provided sshPubKey to it, and uploads the file back to the host.
func RunSSHWhitelistPubKey(host *models.Host, sshPubKey string) error {
	const sshAuthFile = "/home/ubuntu/.ssh/authorized_keys"
	tmpName := filepath.Join(os.TempDir(), utils.RandomString(10))
	defer os.Remove(tmpName)
	if err := host.Download(sshAuthFile, tmpName, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	// write ssh public key
	tmpFile, err := os.OpenFile(tmpName, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := tmpFile.WriteString(sshPubKey + "\n"); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return host.Upload(tmpFile.Name(), sshAuthFile, constants.SSHFileOpsTimeout)
}

// RunSSHDownloadFile downloads specified file from the specified host
func RunSSHDownloadFile(host *models.Host, filePath string, localFilePath string) error {
	return host.Download(filePath, localFilePath, constants.SSHFileOpsTimeout)
}

func RunSSHUpsizeRootDisk(host *models.Host) error {
	return RunOverSSH(
		"Upsize Disk",
		host,
		constants.SSHScriptTimeout,
		"shell/upsizeRootDisk.sh",
		scriptInputs{},
	)
}

// composeFileExists checks if the docker-compose file exists on the host
func composeFileExists(host *models.Host) bool {
	composeFileExists, _ := host.FileExists(utils.GetRemoteComposeFile())
	return composeFileExists
}

func genesisFileExists(host *models.Host) bool {
	genesisFileExists, _ := host.FileExists(filepath.Join(constants.CloudNodeConfigPath, constants.GenesisFileName))
	return genesisFileExists
}

func getAvalancheGoConfigData(host *models.Host) (map[string]interface{}, error) {
	// get remote node.json file
	nodeJSONPath := filepath.Join(constants.CloudNodeConfigPath, constants.NodeFileName)
	tmpFile, err := os.CreateTemp("", "avalanchecli-node-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	if err := host.Download(nodeJSONPath, tmpFile.Name(), constants.SSHFileOpsTimeout); err != nil {
		return nil, err
	}
	// parse node.json file
	nodeJSON, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	var avagoConfig map[string]interface{}
	if err := json.Unmarshal(nodeJSON, &avagoConfig); err != nil {
		return nil, err
	}
	return avagoConfig, nil
}
