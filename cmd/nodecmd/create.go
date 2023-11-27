// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package nodecmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sync"

	"github.com/ava-labs/avalanche-cli/cmd/subnetcmd"
	"github.com/ava-labs/avalanche-cli/pkg/ansible"
	awsAPI "github.com/ava-labs/avalanche-cli/pkg/aws"
	gcpAPI "github.com/ava-labs/avalanche-cli/pkg/gcp"
	"github.com/ava-labs/avalanche-cli/pkg/ssh"
	"github.com/ava-labs/avalanche-cli/pkg/terraform"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"github.com/ava-labs/avalanche-cli/pkg/vm"
	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/staking"
	"golang.org/x/exp/slices"

	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/models"

	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/spf13/cobra"
)

const (
	avalancheGoReferenceChoiceLatest = "latest"
	avalancheGoReferenceChoiceSubnet = "subnet"
	avalancheGoReferenceChoiceCustom = "custom"
)

var (
	createOnFuji                    bool
	createDevnet                    bool
	createOnMainnet                 bool
	useAWS                          bool
	useGCP                          bool
	cmdLineRegion                   []string
	authorizeAccess                 bool
	numNodes                        []int
	nodeType                        string
	useLatestAvalanchegoVersion     bool
	useAvalanchegoVersionFromSubnet string
	cmdLineGCPCredentialsPath       string
	cmdLineGCPProjectName           string
	cmdLineAlternativeKeyPairName   string
)

func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [clusterName]",
		Short: "(ALPHA Warning) Create a new validator on cloud server",
		Long: `(ALPHA Warning) This command is currently in experimental mode. 

The node create command sets up a validator on a cloud server of your choice. 
The validator will be validating the Avalanche Primary Network and Subnet 
of your choice. By default, the command runs an interactive wizard. It 
walks you through all the steps you need to set up a validator.
Once this command is completed, you will have to wait for the validator
to finish bootstrapping on the primary network before running further
commands on it, e.g. validating a Subnet. You can check the bootstrapping
status by running avalanche node status 

The created node will be part of group of validators called <clusterName> 
and users can call node commands with <clusterName> so that the command
will apply to all nodes in the cluster`,
		SilenceUsage: true,
		Args:         cobra.ExactArgs(1),
		RunE:         createNodes,
	}
	cmd.Flags().BoolVar(&useStaticIP, "use-static-ip", true, "attach static Public IP on cloud servers")
	cmd.Flags().BoolVar(&useAWS, "aws", false, "create node/s in AWS cloud")
	cmd.Flags().BoolVar(&useGCP, "gcp", false, "create node/s in GCP cloud")
	cmd.Flags().StringSliceVar(&cmdLineRegion, "region", []string{""}, "create node/s in given region")
	cmd.Flags().StringSliceVar(&cmdLineRegion, "regions", []string{""}, "alias to --region")
	cmd.Flags().BoolVar(&authorizeAccess, "authorize-access", false, "authorize CLI to create cloud resources")
	cmd.Flags().IntSliceVar(&numNodes, "num-nodes", []int{}, "number of nodes to create")
	cmd.Flags().StringVar(&nodeType, "node-type", "default", "cloud instance type")
	cmd.Flags().BoolVar(&useLatestAvalanchegoVersion, "latest-avalanchego-version", false, "install latest avalanchego version on node/s")
	cmd.Flags().StringVar(&useAvalanchegoVersionFromSubnet, "avalanchego-version-from-subnet", "", "install latest avalanchego version, that is compatible with the given subnet, on node/s")
	cmd.Flags().StringVar(&cmdLineGCPCredentialsPath, "gcp-credentials", "", "use given GCP credentials")
	cmd.Flags().StringVar(&cmdLineGCPProjectName, "gcp-project", "", "use given GCP project")
	cmd.Flags().StringVar(&cmdLineAlternativeKeyPairName, "alternative-key-pair-name", "", "key pair name to use if default one generates conflicts")
	cmd.Flags().StringVar(&awsProfile, "aws-profile", constants.AWSDefaultCredential, "aws profile to use")
	cmd.Flags().BoolVar(&createOnFuji, "fuji", false, "create node/s in Fuji Network")
	cmd.Flags().BoolVar(&createDevnet, "devnet", false, "create node/s into a new Devnet")
	return cmd
}

func preCreateChecks() error {
	if useLatestAvalanchegoVersion && useAvalanchegoVersionFromSubnet != "" {
		return fmt.Errorf("could not use both latest avalanchego version and avalanchego version based on given subnet")
	}
	if useAWS && useGCP {
		return fmt.Errorf("could not use both AWS and GCP cloud options")
	}
	if !useAWS && awsProfile != constants.AWSDefaultCredential {
		return fmt.Errorf("could not use AWS profile for non AWS cloud option")
	}
	if len(cmdLineRegion) != len(numNodes) {
		return fmt.Errorf("number of regions and number of nodes must be equal")
	}
	// set default instance type
	switch {
	case nodeType == "default" && useAWS:
		nodeType = "c5.2xlarge"
	case nodeType == "default" && useGCP:
		nodeType = "e2-standard-8"
	}
	return nil
}

func createNodes(_ *cobra.Command, args []string) error {
	if err := preCreateChecks(); err != nil {
		return err
	}
	clusterName := args[0]

	network, err := subnetcmd.GetNetworkFromCmdLineFlags(
		false,
		createDevnet,
		createOnFuji,
		createOnMainnet,
		"",
		false,
		[]models.NetworkKind{models.Fuji, models.Devnet},
	)
	if err != nil {
		return err
	}

	cloudService, err := setCloudService()
	if err != nil {
		return err
	}
	if cloudService != constants.GCPCloudService && cmdLineGCPCredentialsPath != "" {
		return fmt.Errorf("set to use GCP credentials but cloud option is not GCP")
	}
	if cloudService != constants.GCPCloudService && cmdLineGCPProjectName != "" {
		return fmt.Errorf("set to use GCP project but cloud option is not GCP")
	}
	if err := terraform.CheckIsInstalled(); err != nil {
		return err
	}
	err = terraform.RemoveDirectory(app.GetTerraformDir())
	if err != nil {
		return err
	}
	usr, err := user.Current()
	if err != nil {
		return err
	}
	cloudConfig := models.CloudConfigMap{}
	publicIPMap := map[string]string{}
	gcpProjectName := ""
	gcpCredentialFilepath := ""
	if cloudService == constants.AWSCloudService { // Get AWS Credential, region and AMI
		regions, ec2Svc, ami, err := getAWSCloudConfig(awsProfile, cmdLineRegion, authorizeAccess)
		if err != nil {
			return err
		}
		cloudConfig, err = createAWSInstances(ec2Svc, nodeType, numNodes, awsProfile, regions, ami, usr)
		if err != nil {
			return err
		}
		for _, region := range regions {
			if !useStaticIP {
				publicIPMap, err = awsAPI.GetInstancePublicIPs(ec2Svc[region], cloudConfig[region].InstanceIDs)
				if err != nil {
					return err
				}
			} else {
				for i, node := range cloudConfig[region].InstanceIDs {
					publicIPMap[node] = cloudConfig[region].PublicIPs[i]
				}
			}
		}
	} else {
		// Get GCP Credential, zone, Image ID, service account key file path, and GCP project name
		gcpClient, zones, imageID, credentialFilepath, projectName, err := getGCPConfig(cmdLineRegion)
		if err != nil {
			return err
		}
		cloudConfig, err = createGCPInstance(usr, gcpClient, nodeType, numNodes, zones, imageID, credentialFilepath, projectName, clusterName)
		if err != nil {
			return err
		}
		for _, zone := range zones {
			if !useStaticIP {
				publicIPMap, err = gcpAPI.GetInstancePublicIPs(gcpClient, projectName, zone, cloudConfig[zone].InstanceIDs)
				if err != nil {
					return err
				}
			} else {
				for i, node := range cloudConfig[zone].InstanceIDs {
					publicIPMap[node] = cloudConfig[zone].PublicIPs[i]
				}
			}
		}
		gcpProjectName = projectName
		gcpCredentialFilepath = credentialFilepath
	}

	if err = createClusterNodeConfig(network, cloudConfig, clusterName, cloudService); err != nil {
		return err
	}
	if cloudService == constants.GCPCloudService {
		if err = updateClustersConfigGCPKeyFilepath(gcpProjectName, gcpCredentialFilepath); err != nil {
			return err
		}
	}
	err = terraform.RemoveDirectory(app.GetTerraformDir())
	if err != nil {
		return err
	}
	inventoryPath := app.GetAnsibleInventoryDirPath(clusterName)
	avalancheGoVersion, err := getAvalancheGoVersion()
	if err != nil {
		return err
	}
	if err = ansible.CreateAnsibleHostInventory(inventoryPath, cloudConfig, cloudService, publicIPMap); err != nil {
		return err
	}
	if err := updateAnsiblePublicIPs(clusterName); err != nil {
		return err
	}
	allHosts, err := ansible.GetInventoryFromAnsibleInventoryFile(inventoryPath)
	if err != nil {
		return err
	}
	hosts := utils.Filter(allHosts, func(h *models.Host) bool { return slices.Contains(cloudConfig.GetInstanceIDs(""), h.GetCloudID()) })
	// waiting for all nodes to become accessible
	failedHosts := waitForHosts(hosts)
	if failedHosts.Len() > 0 {
		for _, result := range failedHosts.GetResults() {
			ux.Logger.PrintToUser("Instance %s failed to provision with error %s. Please check instance logs for more information", result.NodeID, result.Err)
		}
		return fmt.Errorf("failed to provision node(s) %s", failedHosts.GetNodeList())
	}

	ansibleHostIDs, err := utils.MapWithError(cloudConfig.GetInstanceIDs(""), func(s string) (string, error) { return models.HostCloudIDToAnsibleID(cloudService, s) })
	if err != nil {
		return err
	}

	defer disconnectHosts(hosts)

	ux.Logger.PrintToUser("Installing AvalancheGo and Avalanche-CLI and starting bootstrap process on the newly created Avalanche node(s) ...")
	wg := sync.WaitGroup{}
	wgResults := models.NodeResults{}
	for _, host := range hosts {
		wg.Add(1)
		go func(nodeResults *models.NodeResults, host *models.Host) {
			defer wg.Done()
			if err := host.Connect(); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
			if err := provideStakingCertAndKey(host); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
			if err := ssh.RunSSHSetupNode(host, app.Conf.GetConfigPath(), avalancheGoVersion, network.Kind == models.Devnet); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
			if err := ssh.RunSSHSetupBuildEnv(host); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
			if err := ssh.RunSSHSetupCLIFromSource(host, constants.SetupCLIFromSourceBranch); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
		}(&wgResults, host)
	}
	wg.Wait()
	ux.Logger.PrintToUser("======================================")
	ux.Logger.PrintToUser("AVALANCHE NODE(S) STATUS")
	ux.Logger.PrintToUser("======================================")
	ux.Logger.PrintToUser("")
	for _, node := range hosts {
		if wgResults.HasNodeIDWithError(node.NodeID) {
			ux.Logger.PrintToUser("Node %s is ERROR with error: %s", node.NodeID, wgResults.GetErrorHostMap()[node.NodeID])
		} else {
			ux.Logger.PrintToUser("Node %s is CREATED", node.NodeID)
		}
	}
	if network.Kind == models.Devnet {
		ux.Logger.PrintToUser("Setting up Devnet ...")
		if err := setupDevnet(clusterName, hosts); err != nil {
			return err
		}
	}

	if wgResults.HasErrors() {
		return fmt.Errorf("failed to deploy node(s) %s", wgResults.GetErrorHostMap())
	} else {
		printResults(cloudConfig, publicIPMap, ansibleHostIDs)
		ux.Logger.PrintToUser("AvalancheGo and Avalanche-CLI installed and node(s) are bootstrapping!")
	}
	return nil
}

// createClusterNodeConfig creates node config and save it in .avalanche-cli/nodes/{instanceID}
// also creates cluster config in .avalanche-cli/nodes storing various key pair and security group info for all clusters
// func createClusterNodeConfig(nodeIDs, publicIPs []string, region, ami, keyPairName, certPath, sg, clusterName string) error {
func createClusterNodeConfig(network models.Network, cloudConfigMap models.CloudConfigMap, clusterName, cloudService string) error {
	for _, cloudConfig := range cloudConfigMap {
		for i := range cloudConfig.InstanceIDs {
			publicIP := ""
			if len(cloudConfig.PublicIPs) > 0 {
				publicIP = cloudConfig.PublicIPs[i]
			}
			nodeConfig := models.NodeConfig{
				NodeID:        cloudConfig.InstanceIDs[i],
				Region:        cloudConfig.Region,
				AMI:           cloudConfig.ImageID,
				KeyPair:       cloudConfig.KeyPair,
				CertPath:      cloudConfig.CertFilePath,
				SecurityGroup: cloudConfig.SecurityGroup,
				ElasticIP:     publicIP,
				CloudService:  cloudService,
			}
			err := app.CreateNodeCloudConfigFile(cloudConfig.InstanceIDs[i], &nodeConfig)
			if err != nil {
				return err
			}
			if err = addNodeToClustersConfig(network, cloudConfig.InstanceIDs[i], clusterName); err != nil {
				return err
			}
			if err := updateKeyPairClustersConfig(cloudConfig); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateKeyPairClustersConfig(cloudConfig models.CloudConfig) error {
	clustersConfig := models.ClustersConfig{}
	var err error
	if app.ClustersConfigExists() {
		clustersConfig, err = app.LoadClustersConfig()
		if err != nil {
			return err
		}
	}
	if clustersConfig.KeyPair == nil {
		clustersConfig.KeyPair = make(map[string]string)
	}
	if _, ok := clustersConfig.KeyPair[cloudConfig.KeyPair]; !ok {
		clustersConfig.KeyPair[cloudConfig.KeyPair] = cloudConfig.CertFilePath
	}
	return app.WriteClustersConfigFile(&clustersConfig)
}

func addNodeToClustersConfig(network models.Network, nodeID, clusterName string) error {
	clustersConfig := models.ClustersConfig{}
	var err error
	if app.ClustersConfigExists() {
		clustersConfig, err = app.LoadClustersConfig()
		if err != nil {
			return err
		}
	}
	if clustersConfig.Clusters == nil {
		clustersConfig.Clusters = make(map[string]models.ClusterConfig)
	}
	if _, ok := clustersConfig.Clusters[clusterName]; !ok {
		clustersConfig.Clusters[clusterName] = models.ClusterConfig{
			Network: network,
			Nodes:   []string{},
		}
	}
	nodes := clustersConfig.Clusters[clusterName].Nodes
	clustersConfig.Clusters[clusterName] = models.ClusterConfig{
		Network: network,
		Nodes:   append(nodes, nodeID),
	}
	return app.WriteClustersConfigFile(&clustersConfig)
}

func getNodeID(nodeDir string) (ids.NodeID, error) {
	certBytes, err := os.ReadFile(filepath.Join(nodeDir, constants.StakerCertFileName))
	if err != nil {
		return ids.EmptyNodeID, err
	}
	keyBytes, err := os.ReadFile(filepath.Join(nodeDir, constants.StakerKeyFileName))
	if err != nil {
		return ids.EmptyNodeID, err
	}
	nodeID, err := utils.ToNodeID(certBytes, keyBytes)
	if err != nil {
		return ids.EmptyNodeID, err
	}
	return nodeID, nil
}

func generateNodeCertAndKeys(stakerCertFilePath, stakerKeyFilePath, blsKeyFilePath string) (ids.NodeID, error) {
	certBytes, keyBytes, err := staking.NewCertAndKeyBytes()
	if err != nil {
		return ids.EmptyNodeID, err
	}
	nodeID, err := utils.ToNodeID(certBytes, keyBytes)
	if err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.MkdirAll(filepath.Dir(stakerCertFilePath), constants.DefaultPerms755); err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.WriteFile(stakerCertFilePath, certBytes, constants.WriteReadUserOnlyPerms); err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.MkdirAll(filepath.Dir(stakerKeyFilePath), constants.DefaultPerms755); err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.WriteFile(stakerKeyFilePath, keyBytes, constants.WriteReadUserOnlyPerms); err != nil {
		return ids.EmptyNodeID, err
	}
	blsSignerKeyBytes, err := utils.NewBlsSecretKeyBytes()
	if err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.MkdirAll(filepath.Dir(blsKeyFilePath), constants.DefaultPerms755); err != nil {
		return ids.EmptyNodeID, err
	}
	if err := os.WriteFile(blsKeyFilePath, blsSignerKeyBytes, constants.WriteReadUserOnlyPerms); err != nil {
		return ids.EmptyNodeID, err
	}
	return nodeID, nil
}

func provideStakingCertAndKey(host *models.Host) error {
	instanceID := host.GetCloudID()
	keyPath := filepath.Join(app.GetNodesDir(), instanceID)
	nodeID, err := generateNodeCertAndKeys(
		filepath.Join(keyPath, constants.StakerCertFileName),
		filepath.Join(keyPath, constants.StakerKeyFileName),
		filepath.Join(keyPath, constants.BLSKeyFileName),
	)
	if err != nil {
		ux.Logger.PrintToUser("Failed to generate staking keys for host %s", instanceID)
		return err
	} else {
		ux.Logger.PrintToUser("Generated staking keys for host %s[%s] ", instanceID, nodeID.String())
	}
	return ssh.RunSSHUploadStakingFiles(host, keyPath)
}

func getIPAddress() (string, error) {
	resp, err := http.Get("https://api.ipify.org?format=json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("HTTP request failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	ipAddress, ok := result["ip"].(string)
	if ok {
		if net.ParseIP(ipAddress) == nil {
			return "", errors.New("invalid IP address")
		}
		return ipAddress, nil
	}

	return "", errors.New("no IP address found")
}

// getAvalancheGoVersion asks users whether they want to install the newest Avalanche Go version
// or if they want to use the newest Avalanche Go Version that is still compatible with Subnet EVM
// version of their choice
func getAvalancheGoVersion() (string, error) {
	version := ""
	subnet := ""
	if useLatestAvalanchegoVersion { //nolint: gocritic
		version = "latest"
	} else if useAvalanchegoVersionFromSubnet != "" {
		subnet = useAvalanchegoVersionFromSubnet
	} else {
		choice, subnetChoice, err := promptAvalancheGoReferenceChoice()
		if err != nil {
			return "", err
		}
		switch choice {
		case avalancheGoReferenceChoiceLatest:
			version = "latest"
		case avalancheGoReferenceChoiceCustom:
			customVersion, err := app.Prompt.CaptureVersion("Which version of AvalancheGo would you like to install? (Use format v1.10.13)")
			if err != nil {
				return "", err
			}
			version = customVersion
		case avalancheGoReferenceChoiceSubnet:
			subnet = subnetChoice
		}
	}
	if subnet != "" {
		sc, err := app.LoadSidecar(subnet)
		if err != nil {
			return "", err
		}
		version, err = GetLatestAvagoVersionForRPC(sc.RPCVersion)
		if err != nil {
			return "", err
		}
	}
	return version, nil
}

func GetLatestAvagoVersionForRPC(configuredRPCVersion int) (string, error) {
	desiredAvagoVersion, err := vm.GetLatestAvalancheGoByProtocolVersion(
		app, configuredRPCVersion, constants.AvalancheGoCompatibilityURL)
	if err != nil {
		return "", err
	}
	return desiredAvagoVersion, nil
}

// promptAvalancheGoReferenceChoice returns user's choice of either using the latest Avalanche Go
// version or using the latest Avalanche Go version that is still compatible with the subnet that user
// wants the cloud server to track
func promptAvalancheGoReferenceChoice() (string, string, error) {
	defaultVersion := "Use latest Avalanche Go Version"
	txt := "What version of Avalanche Go would you like to install in the node?"
	versionOptions := []string{defaultVersion, "Use the deployed Subnet's VM version that the node will be validating", "Custom"}
	versionOption, err := app.Prompt.CaptureList(txt, versionOptions)
	if err != nil {
		return "", "", err
	}

	switch versionOption {
	case defaultVersion:
		return avalancheGoReferenceChoiceLatest, "", nil
	case "Custom":
		return avalancheGoReferenceChoiceCustom, "", nil
	default:
		for {
			subnetName, err := app.Prompt.CaptureString("Which Subnet would you like to use to choose the avalanche go version?")
			if err != nil {
				return "", "", err
			}
			_, err = subnetcmd.ValidateSubnetNameAndGetChains([]string{subnetName})
			if err == nil {
				return avalancheGoReferenceChoiceSubnet, subnetName, nil
			}
			ux.Logger.PrintToUser(fmt.Sprintf("no subnet named %s found", subnetName))
		}
	}
}

func setCloudService() (string, error) {
	if useAWS {
		return constants.AWSCloudService, nil
	}
	if useGCP {
		return constants.GCPCloudService, nil
	}
	txt := "Which cloud service would you like to launch your Avalanche Node(s) in?"
	cloudOptions := []string{constants.AWSCloudService, constants.GCPCloudService}
	chosenCloudService, err := app.Prompt.CaptureList(txt, cloudOptions)
	if err != nil {
		return "", err
	}
	return chosenCloudService, nil
}

func printResults(cloudConfigMap models.CloudConfigMap, publicIPMap map[string]string, ansibleHostIDs []string) {
	ux.Logger.PrintToUser("======================================")
	ux.Logger.PrintToUser("AVALANCHE NODE(S) SUCCESSFULLY SET UP!")
	ux.Logger.PrintToUser("======================================")
	ux.Logger.PrintToUser("Please wait until the node(s) are successfully bootstrapped to run further commands on the node(s)")
	ux.Logger.PrintToUser("")
	ux.Logger.PrintToUser("Here are the details of the set up node(s): ")
	for _, cloudConfig := range cloudConfigMap {
		ux.Logger.PrintToUser(fmt.Sprintf("Don't delete or replace your ssh private key file at %s as you won't be able to access your cloud server without it", cloudConfig.CertFilePath))
		for i, instanceID := range cloudConfig.InstanceIDs {
			publicIP := ""
			publicIP = publicIPMap[instanceID]
			ux.Logger.PrintToUser("======================================")
			ux.Logger.PrintToUser(fmt.Sprintf("Node %s details: ", ansibleHostIDs[i]))
			ux.Logger.PrintToUser(fmt.Sprintf("Cloud Instance ID: %s", instanceID))
			ux.Logger.PrintToUser(fmt.Sprintf("Public IP: %s", publicIP))
			ux.Logger.PrintToUser(fmt.Sprintf("Cloud Region: %s", cloudConfig.Region))
			ux.Logger.PrintToUser("")
			ux.Logger.PrintToUser(fmt.Sprintf("staker.crt and staker.key are stored at %s. If anything happens to your node or the machine node runs on, these files can be used to fully recreate your node.", app.GetNodeInstanceDirPath(instanceID)))
			ux.Logger.PrintToUser("")
			ux.Logger.PrintToUser("To ssh to node, run: ")
			ux.Logger.PrintToUser("")
			ux.Logger.PrintToUser(utils.GetSSHConnectionString(publicIP, cloudConfig.CertFilePath))
			ux.Logger.PrintToUser("")
			ux.Logger.PrintToUser("======================================")
		}
	}
	ux.Logger.PrintToUser("")
}

// waitForHosts waits for all hosts to become available via SSH.
func waitForHosts(hosts []*models.Host) *models.NodeResults {
	hostErrors := models.NodeResults{}
	createdWaitGroup := sync.WaitGroup{}
	for _, host := range hosts {
		createdWaitGroup.Add(1)
		go func(nodeResults *models.NodeResults, host *models.Host) {
			defer createdWaitGroup.Done()
			if err := host.WaitForSSHShell(constants.SSHServerStartTimeout); err != nil {
				nodeResults.AddResult(host.NodeID, nil, err)
				return
			}
		}(&hostErrors, host)
	}
	createdWaitGroup.Wait()
	return &hostErrors
}
