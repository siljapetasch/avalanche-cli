// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	cmdflags "github.com/ava-labs/avalanche-cli/cmd/flags"
	"github.com/ava-labs/avalanche-cli/pkg/cobrautils"
	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/metrics"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/teleporter"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/ava-labs/avalanche-cli/pkg/vm"
	"github.com/ava-labs/subnet-evm/params"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const (
	forceFlag  = "force"
	latest     = "latest"
	preRelease = "pre-release"
)

type CreateFlags struct {
	GenesisFile                    string
	UseSubnetEvm                   bool
	EvmVersion                     string
	EvmChainID                     uint64
	EvmToken                       string
	EvmDefaults                    bool
	UseCustom                      bool
	UseLatestReleasedEvmVersion    bool
	UseLatestPreReleasedEvmVersion bool
	ForceCreate                    bool
	vmFile                         string
	useRepo                        bool
	useTeleporter                  bool
	useWarp                        bool
	CustomVMRepoURL                string
	CustomVMBranch                 string
	CustomVMBuildScript            string
}

var (
	createFlags CreateFlags

	errIllegalNameCharacter = errors.New(
		"illegal name character: only letters, no special characters allowed")
	errMutuallyExlusiveVersionOptions = errors.New("version flags --latest,--pre-release,vm-version are mutually exclusive")
	errMutuallyVMConfigOptions        = errors.New("specifying --genesis flag disables SubnetEVM config flags --evm-chain-id,--evm-token,--evm-defaults")
)

// avalanche subnet create
func newCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [subnetName]",
		Short: "Create a new subnet configuration",
		Long: `The subnet create command builds a new genesis file to configure your Subnet.
By default, the command runs an interactive wizard. It walks you through
all the steps you need to create your first Subnet.

The tool supports deploying Subnet-EVM, and custom VMs. You
can create a custom, user-generated genesis with a custom VM by providing
the path to your genesis and VM binaries with the --genesis and --vm flags.

By default, running the command with a subnetName that already exists
causes the command to fail. If you'd like to overwrite an existing
configuration, pass the -f flag.`,
		Args:              cobrautils.ExactArgs(1),
		RunE:              createSubnetConfig,
		PersistentPostRun: handlePostRun,
	}
	cmd.Flags().StringVar(&createFlags.GenesisFile, "genesis", "", "file path of genesis to use")
	cmd.Flags().BoolVar(&createFlags.UseSubnetEvm, "evm", false, "use the Subnet-EVM as the base template")
	cmd.Flags().StringVar(&createFlags.EvmVersion, "vm-version", "", "version of Subnet-EVM template to use")
	cmd.Flags().Uint64Var(&createFlags.EvmChainID, "evm-chain-id", 0, "chain ID to use with Subnet-EVM")
	cmd.Flags().StringVar(&createFlags.EvmToken, "evm-token", "", "token symbol to use with Subnet-EVM")
	cmd.Flags().BoolVar(&createFlags.EvmDefaults, "evm-defaults", false, "use default settings for fees/airdrop/precompiles/teleporter with Subnet-EVM")
	cmd.Flags().BoolVar(&createFlags.UseCustom, "custom", false, "use a custom VM template")
	cmd.Flags().BoolVar(&createFlags.UseLatestPreReleasedEvmVersion, preRelease, false, "use latest Subnet-EVM pre-released version, takes precedence over --vm-version")
	cmd.Flags().BoolVar(&createFlags.UseLatestReleasedEvmVersion, latest, false, "use latest Subnet-EVM released version, takes precedence over --vm-version")
	cmd.Flags().BoolVarP(&createFlags.ForceCreate, forceFlag, "f", false, "overwrite the existing configuration if one exists")
	cmd.Flags().StringVar(&createFlags.vmFile, "vm", "", "file path of custom vm to use. alias to custom-vm-path")
	cmd.Flags().StringVar(&createFlags.vmFile, "custom-vm-path", "", "file path of custom vm to use")
	cmd.Flags().StringVar(&createFlags.CustomVMRepoURL, "custom-vm-repo-url", "", "custom vm repository url")
	cmd.Flags().StringVar(&createFlags.CustomVMBranch, "custom-vm-branch", "", "custom vm branch or commit")
	cmd.Flags().StringVar(&createFlags.CustomVMBuildScript, "custom-vm-build-script", "", "custom vm build-script")
	cmd.Flags().BoolVar(&createFlags.useRepo, "from-github-repo", false, "generate custom VM binary from github repository")
	cmd.Flags().BoolVar(&createFlags.useWarp, "warp", true, "generate a vm with warp support (needed for teleporter)")
	cmd.Flags().BoolVar(&createFlags.useTeleporter, "teleporter", false, "interoperate with other blockchains using teleporter")
	return cmd
}

func createSubnetConfig(cmd *cobra.Command, args []string) error {
	return CallCreate(cmd, args[0], createFlags)
}

func CallCreate(
	cmd *cobra.Command,
	subnetName string,
	flags CreateFlags,
) error {
	if app.GenesisExists(subnetName) && !flags.ForceCreate {
		return errors.New("configuration already exists. Use --" + forceFlag + " parameter to overwrite")
	}

	if err := checkInvalidSubnetNames(subnetName); err != nil {
		return fmt.Errorf("subnet name %q is invalid: %w", subnetName, err)
	}

	if flags.CustomVMRepoURL != "" || flags.CustomVMBranch != "" || flags.CustomVMBuildScript != "" {
		// assumes custom vm
		flags.UseCustom = true
	}

	if !cmdflags.EnsureMutuallyExclusive([]bool{flags.UseSubnetEvm, flags.UseCustom}) {
		return errors.New("too many VMs selected. Provide at most one VM selection flag")
	}

	if !cmdflags.EnsureMutuallyExclusive([]bool{
		flags.UseLatestReleasedEvmVersion,
		flags.UseLatestPreReleasedEvmVersion,
		flags.EvmVersion != "",
	}) {
		return errMutuallyExlusiveVersionOptions
	}

	if flags.GenesisFile != "" && (flags.EvmChainID != 0 || flags.EvmToken != "" || flags.EvmDefaults) {
		return errMutuallyVMConfigOptions
	}

	subnetType := getVMFromFlag(flags.UseSubnetEvm, flags.UseCustom)

	if subnetType == "" {
		subnetEvmOption := "Subnet-EVM"
		customVMOption := "Custom VM"
		explainOption := "Explain the difference"
		options := []string{subnetEvmOption, customVMOption, explainOption}
		var subnetTypeStr string
		for {
			option, err := app.Prompt.CaptureList(
				"VM",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case subnetEvmOption:
				subnetTypeStr = models.SubnetEvm
			case customVMOption:
				subnetTypeStr = models.CustomVM
			case explainOption:
				ux.Logger.PrintToUser("Virtual machines are the blueprint the defines the application-level logic of a blockchain. It determines the language and rules for writing and executing smart contracts, as well as other blockchain logic.")
				ux.Logger.PrintToUser("Subnet-EVM is a EVM-compatible virtual machine that supports smart contract development in Solidity. This VM is an out-of-box solution for Subnet deployers who want a dApp development experience that is nearly identical to Ethereum, without having to manage or create a custom virtual machine. For more information, please visit: https://github.com/ava-labs/subnet-evm")
				ux.Logger.PrintToUser("Custom VMs created with the HyperSDK or writen from scratch in golang or rust can be deployed on Avalanche using the second option. More information can be found in the docs at https://docs.avax.network/learn/avalanche/virtual-machines.")
				continue
			}
			break
		}
		subnetType = models.VMTypeFromString(subnetTypeStr)
	}

	var (
		genesisBytes []byte
		sc           *models.Sidecar
		err          error
	)

	if flags.UseLatestReleasedEvmVersion {
		flags.EvmVersion = latest
	}

	if flags.UseLatestPreReleasedEvmVersion {
		flags.EvmVersion = preRelease
	}

	if flags.EvmVersion != latest && flags.EvmVersion != preRelease && flags.EvmVersion != "" && !semver.IsValid(flags.EvmVersion) {
		return fmt.Errorf("invalid version string, should be semantic version (ex: v1.1.1): %s", flags.EvmVersion)
	}

	genesisFileIsEVM := false
	if flags.GenesisFile != "" {
		genesisFileIsEVM, err = utils.PathIsSubnetEVMGenesis(flags.GenesisFile)
		if err != nil {
			return err
		}
	}

	if subnetType == models.SubnetEvm && flags.GenesisFile != "" && !genesisFileIsEVM {
		return fmt.Errorf("provided genesis file has no proper Subnet-EVM format")
	}

	if subnetType == models.SubnetEvm {
		flags.EvmVersion, err = vm.GetVMVersion(app, "Subnet-EVM", constants.SubnetEVMRepoName, flags.EvmVersion)
		if err != nil {
			return err
		}
	}

	if subnetType == models.SubnetEvm && flags.GenesisFile == "" {
		if flags.EvmChainID == 0 {
			flags.EvmChainID, err = app.Prompt.CaptureUint64("Chain ID")
			if err != nil {
				return err
			}
		}
	}

	// Gas token
	externalGasToken := false
	if subnetType == models.SubnetEvm && flags.GenesisFile == "" {
		nativeTokenOption := "It's own Native Token"
		externalTokenOption := "A token from another blockchain"
		explainOption := "Explain the difference"
		options := []string{nativeTokenOption, externalTokenOption, explainOption}
		for {
			option, err := app.Prompt.CaptureList(
				"What kind of gas token should your blockchain use?",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case nativeTokenOption:
				flags.EvmToken, err = app.Prompt.CaptureString("Token Symbol")
				if err != nil {
					return err
				}
				allocateToNewKeyOption := "1m to new key (+x amount to relayer)"
				allocateToEwoqOption := "1m to new ewoq (not recommended for production, +x amount to relayer)"
				customAllocationOption := "Custom allocation (configure exact amount to relayer)"
				options := []string{allocateToNewKeyOption, allocateToEwoqOption, customAllocationOption}
				option, err := app.Prompt.CaptureList(
					"Initial Token Allocation",
					options,
				)
				if err != nil {
					return err
				}
				if option == customAllocationOption {
					_, err := app.Prompt.CaptureAddress("Address to allocate to")
					if err != nil {
						return err
					}
					_, err = app.Prompt.CaptureUint64(fmt.Sprintf("Amount to airdrop (in %s units)", flags.EvmToken))
					if err != nil {
						return err
					}
				}
				fixedSupplyOption := "I want to have a fixed supply of tokens on my blockchain. (Native Minter Precompile OFF)"
				dynamicSupplyOption := "Yes, I want to be able to mint additional tokens on my blockchain. (Native Minter Precompile ON)"
				options = []string{fixedSupplyOption, dynamicSupplyOption}
				option, err = app.Prompt.CaptureList(
					"Allow minting new native Tokens? (Native Minter Precompile)",
					options,
				)
				if err != nil {
					return err
				}
				if option == dynamicSupplyOption {
					_, _, _, _, err := vm.GenerateAllowList(app, "mint native tokens", flags.EvmVersion)
					if err != nil {
						return err
					}
				}
			case externalTokenOption:
				externalGasToken = true
			case explainOption:
				ux.Logger.PrintToUser("Gas tokens exist because blockchains have limited resources. Native tokens the default gas token, and are non-programmable unless wrapped by an ERC-20 token contract.")
				ux.Logger.PrintToUser("If desired, ERC-20 tokens can be deployed on other blockchains and used as the gas token enabled by a bridge. When a transaction is initiated, the ERC-20 amount will be locked on the source chain, a message will be relayed to the Subnet, and then the token will be minted to the sender's address using the Native Minter precompile. This means users with a balance of that ERC-20 on a separate chain can use it to pay for gas on the Subnet.")
				continue
			}
			break
		}
	}

	// Transaction / Gas Fees
	if subnetType == models.SubnetEvm && flags.GenesisFile == "" {
		customizeOption := "Customize fee config"
		explainOption := "Explain the difference"
		lowOption := "Low disk use    / Low Throughput    1.5 mil gas/s (C-Chain's setting)"
		mediumOption := "Medium disk use / Medium Throughput 2 mil   gas/s"
		highOption := "High disk use   / High Throughput   5 mil   gas/s"
		options := []string{lowOption, mediumOption, highOption, customizeOption, explainOption}
		for {
			option, err := app.Prompt.CaptureList(
				"How should the gas fees be configured on your Blockchain?",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case customizeOption:
				config := params.ChainConfig{}
				_, _, err = vm.CustomizeFeeConfig(config, app)
				if err != nil {
					return err
				}
			case explainOption:
				ux.Logger.PrintToUser("The two gas fee variables that have the largest impact on performance are the gas limit, the maximum amount of gas that fits in a block, and the gas target, the expected amount of gas consumed in a rolling ten-second period.")
				ux.Logger.PrintToUser("By increasing the gas limit, you can fit more transactions into a single block which in turn increases your max throughput. Increasing the gas target has the same effect; if the targeted amount of gas is not consumed, the dynamic fee algorithm will decrease the base fee until it reaches the minimum.")
				ux.Logger.PrintToUser("There is a long-term risk of increasing your gas parameters. By allowing more transactions to occur on your network, the network state will increase at a faster rate, meaning infrastructure costs and requirements will increase.")
				continue
			}
			break
		}
		dontChangeFeeSettingsOption := "I am fine with the gas fee configuration set in the genesis (Fee Manager Precompile OFF)"
		changeFeeSettingsOption := "I want to be able to adjust gas pricing if necessary - recommended for production (Fee Manager Precompile ON)"
		options = []string{dontChangeFeeSettingsOption, changeFeeSettingsOption, explainOption}
		for {
			option, err := app.Prompt.CaptureList(
				"Should these fees be changeable on the fly? (Fee Manager Precompile)",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case changeFeeSettingsOption:
				_, _, _, _, err := vm.GenerateAllowList(app, "adjust the gas fees", flags.EvmVersion)
				if err != nil {
					return err
				}
				//missing case for dontChangeFeeSettingsOption
			}
			break
		}
		burnFees := "I am fine with gas fees being burned (Reward Manager Precompile OFF)"
		distributeFees := "I want to customize accumulated gas fees distribution (Reward Manager Precompile ON)"
		explainOption = "Explain the difference"
		options = []string{burnFees, distributeFees, explainOption}
		for {
			option, err := app.Prompt.CaptureList(
				"By default, all fees on Avalanche are burned (sent to a blackhole address). (Reward Manager Precompile)",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case distributeFees:
				_, _, _, _, err := vm.GenerateAllowList(app, "customize gas fees distribution", flags.EvmVersion)
				if err != nil {
					return err
				}
			case explainOption:
				ux.Logger.PrintToUser("The fee reward mechanism can be configured with a stateful precompile contract called the RewardManager. The configuration can include burning fees, sending fees to a predefined address, or enabling fees to be collected by block producers. For more info, please visit: https://docs.avax.network/build/subnet/upgrade/customize-a-subnet#changing-fee-reward-mechanisms")
				continue
			}
			break
		}
	}

	// Interoperability
	var teleporterInfo *teleporter.Info
	if subnetType == models.SubnetEvm || genesisFileIsEVM {
		if externalGasToken {
			flags.useTeleporter = true
		}
		if flags.EvmDefaults {
			flags.useTeleporter = true
		}
		flagName := "teleporter"
		if flag := cmd.Flags().Lookup(flagName); flag == nil {
			return fmt.Errorf("flag configuration %q not found for cmd %q", flagName, cmd.Use)
		} else if !flag.Changed && !externalGasToken {
			interoperatingBlockchainOption := "Yes, I want my blockchain to be able to interoperate with other blockchains and the C-Chain"
			isolatedBlockchainOption := "No, I want to run my blockchain isolated"
			explainOption := "Explain the difference"
			options := []string{interoperatingBlockchainOption, isolatedBlockchainOption, explainOption}
			for {
				option, err := app.Prompt.CaptureList(
					"Do you want to connect your blockchain with other blockchains or the C-Chain? (Deploy Teleporter along with its Registry)",
					options,
				)
				if err != nil {
					return err
				}
				switch option {
				case interoperatingBlockchainOption:
					flags.useTeleporter = true
				case isolatedBlockchainOption:
					flags.useTeleporter = false
				case explainOption:
					ux.Logger.PrintToUser("Avalanche already enables native interoperability between blockchains with the VM-agnostic Avalanche Warp Messaging protocol (AWM). Teleporter is a messaging protocol built on top of AWM that provides a developer-friendly interface for sending and receiving cross-chain messages to and from EVM-compatible blockchains.")
					continue
				}
				break
			}
		}
		if flags.useTeleporter && !flags.useWarp {
			return fmt.Errorf("warp should be enabled for teleporter to work")
		}
		if flags.useTeleporter {
			teleporterInfo, err = teleporter.GetInfo(app)
			if err != nil {
				return err
			}
		}
	}

	// Permissioning
	if subnetType == models.SubnetEvm && flags.GenesisFile == "" {
		noOption := "No"
		yesOption := "Yes"
		explainOption := "Explain the difference"
		options := []string{noOption, yesOption, explainOption}
		for {
			option, err := app.Prompt.CaptureList(
				"You can optionally add permissioning on different levels to your blockchain. Do you want to make your blockchain permissioned?",
				options,
			)
			if err != nil {
				return err
			}
			switch option {
			case yesOption:
				anyoneCanSubmitTransactionsOption := "I want anyone to be able to submit transactions on my blockchain. (Transaction Allow List OFF)"
				approvedCanSubmitTransactionsOption := "I want only approved addresses to submit transactions on my blockchain. (Transaction Allow List ON)"
				options := []string{anyoneCanSubmitTransactionsOption, approvedCanSubmitTransactionsOption, explainOption}
				for {
					option, err := app.Prompt.CaptureList(
						"Do you want to allow only certain user addresses to interact with your blockchain? (Transaction Allow List Precompile)",
						options,
					)
					if err != nil {
						return err
					}
					switch option {
					case approvedCanSubmitTransactionsOption:
						_, _, _, _, err := vm.GenerateAllowList(app, "issue transactions", flags.EvmVersion)
						if err != nil {
							return err
						}
					case explainOption:
						ux.Logger.PrintToUser("The Transaction Allow List is a precompile contract that allows you to specify a list of addresses that are allowed to submit transactions to your blockchain. This is useful for permissioning your blockchain, similar to a whitelist, and can be used to prevent spam and unwanted transactions on your chain. This prevents any unauthorized users from sending transactions or deploying smart contracts on your blockchain. For more information, please visit: https://docs.avax.network/build/subnet/upgrade/customize-a-subnet#restricting-who-can-submit-transactions.")
						continue
					}
					break
				}
				anyoneCanDeployContractsOption := "I want anyone to be able to deploy smart contracts on my blockchain. (Smart Contract Deployer Allow List OFF)"
				approvedCanDeployContractsOption := "I want only approved addresses to deploy smart contracts on my blockchain. (Smart Contract Deployer Allow List ON)"
				options = []string{anyoneCanDeployContractsOption, approvedCanDeployContractsOption, explainOption}
				for {
					option, err := app.Prompt.CaptureList(
						"Do you want to allow only certain addresses to deploy smart contracts on your blockchain? (Contract Deployer Allow List)",
						options,
					)
					if err != nil {
						return err
					}
					switch option {
					case approvedCanDeployContractsOption:
						_, _, _, _, err := vm.GenerateAllowList(app, "deploy smart contracts", flags.EvmVersion)
						if err != nil {
							return err
						}
					case explainOption:
						ux.Logger.PrintToUser("While you may wish to allow anyone to submit transactions to your blockchain, you may want to restrict who can deploy smart contracts and create dApps on your chain. The Smart Contract Deployer Allow List is a precompile contract that allows you to specify a list of addresses that are allowed to deploy smart contracts on your blockchain. For more information, please visit: https://docs.avax.network/build/subnet/upgrade/customize-a-subnet#restricting-smart-contract-deployers.")
						continue
					}
					break
				}
			case explainOption:
				ux.Logger.PrintToUser("You can permission your chain at different levels of interaction with EVM-Precompiles. These precompiles act as whitelists, preventing unapproved users from deploying smart contracts, sending transactions, or interacting with your blockchain. You may choose to apply as many or as little of these rules as you see fit.")
				continue
			}
			break
		}
	}

	return nil

	switch subnetType {
	case models.SubnetEvm:
		genesisBytes, sc, err = vm.CreateEvmSubnetConfig(
			app,
			subnetName,
			flags.GenesisFile,
			flags.EvmVersion,
			true,
			flags.EvmChainID,
			flags.EvmToken,
			flags.EvmDefaults,
			flags.useWarp,
			teleporterInfo,
		)
		if err != nil {
			return err
		}
	case models.CustomVM:
		genesisBytes, sc, err = vm.CreateCustomSubnetConfig(
			app,
			subnetName,
			flags.GenesisFile,
			flags.useRepo,
			flags.CustomVMRepoURL,
			flags.CustomVMBranch,
			flags.CustomVMBuildScript,
			flags.vmFile,
		)
		if err != nil {
			return err
		}
	default:
		return errors.New("not implemented")
	}

	if flags.useTeleporter {
		sc.TeleporterReady = flags.useTeleporter
		sc.TeleporterKey = constants.TeleporterKeyName
		sc.TeleporterVersion = teleporterInfo.Version
		if flags.GenesisFile != "" && genesisFileIsEVM {
			// evm genesis file was given. make appropriate checks and customizations for teleporter
			genesisBytes, err = addSubnetEVMGenesisPrefundedAddress(genesisBytes, teleporterInfo.FundedAddress, teleporterInfo.FundedBalance.String())
			if err != nil {
				return err
			}
		}
	}

	if err = app.WriteGenesisFile(subnetName, genesisBytes); err != nil {
		return err
	}

	sc.ImportedFromAPM = false
	if err = app.CreateSidecar(sc); err != nil {
		return err
	}
	if subnetType == models.SubnetEvm {
		err = sendMetrics(cmd, subnetType.RepoName(), subnetName)
		if err != nil {
			return err
		}
	}
	ux.Logger.GreenCheckmarkToUser("Successfully created subnet configuration")
	return nil
}

func addSubnetEVMGenesisPrefundedAddress(genesisBytes []byte, address string, balance string) ([]byte, error) {
	var genesisMap map[string]interface{}
	if err := json.Unmarshal(genesisBytes, &genesisMap); err != nil {
		return nil, err
	}
	allocI, ok := genesisMap["alloc"]
	if !ok {
		return nil, fmt.Errorf("alloc field not found on genesis")
	}
	alloc, ok := allocI.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected genesis alloc field to be map[string]interface, found %T", allocI)
	}
	trimmedAddress := strings.TrimPrefix(address, "0x")
	alloc[trimmedAddress] = map[string]interface{}{
		"balance": balance,
	}
	genesisMap["alloc"] = alloc
	return json.MarshalIndent(genesisMap, "", "  ")
}

func sendMetrics(cmd *cobra.Command, repoName, subnetName string) error {
	flags := make(map[string]string)
	flags[constants.SubnetType] = repoName
	genesis, err := app.LoadEvmGenesis(subnetName)
	if err != nil {
		return err
	}
	conf := genesis.Config.GenesisPrecompiles
	precompiles := make([]string, 6)
	for precompileName := range conf {
		precompileTag := "precompile-" + precompileName
		flags[precompileTag] = precompileName
		precompiles = append(precompiles, precompileName)
	}
	numAirdropAddresses := len(genesis.Alloc)
	for address := range genesis.Alloc {
		if address.String() != vm.PrefundedEwoqAddress.String() {
			precompileTag := "precompile-" + constants.CustomAirdrop
			flags[precompileTag] = constants.CustomAirdrop
			precompiles = append(precompiles, constants.CustomAirdrop)
			break
		}
	}
	sort.Strings(precompiles)
	precompilesJoined := strings.Join(precompiles, ",")
	flags[constants.PrecompileType] = precompilesJoined
	flags[constants.NumberOfAirdrops] = strconv.Itoa(numAirdropAddresses)
	metrics.HandleTracking(cmd, constants.MetricsSubnetCreateCommand, app, flags)
	return nil
}

func checkInvalidSubnetNames(name string) error {
	// this is currently exactly the same code as in avalanchego/vms/platformvm/create_chain_tx.go
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ') {
			return errIllegalNameCharacter
		}
	}
	return nil
}

func getVMFromFlag(useSubnetEvm bool, useCustom bool) models.VMType {
	if useSubnetEvm {
		return models.SubnetEvm
	}
	if useCustom {
		return models.CustomVM
	}
	return ""
}

// override postrun function from root.go, so that we don't double send metrics for the same command
func handlePostRun(_ *cobra.Command, _ []string) {}
