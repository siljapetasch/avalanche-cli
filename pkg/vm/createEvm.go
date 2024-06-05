// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package vm

import (
	"errors"
	"fmt"
	"github.com/ava-labs/avalanche-tooling-sdk-go/subnet"
	teleporterSDK "github.com/ava-labs/avalanche-tooling-sdk-go/teleporter"
	"math/big"
	"os"
	"time"

	"github.com/ava-labs/avalanche-cli/pkg/application"
	"github.com/ava-labs/avalanche-cli/pkg/binutils"
	"github.com/ava-labs/avalanche-cli/pkg/constants"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/statemachine"
	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/ava-labs/subnet-evm/core"
	"github.com/ava-labs/subnet-evm/params"
	"github.com/ava-labs/subnet-evm/utils"
	"github.com/ethereum/go-ethereum/common"
)

var versionComments = map[string]string{
	"v0.6.0-fuji": " (recommended for fuji durango)",
}

func CreateEvmSubnetConfig(
	app *application.Avalanche,
	subnetName string,
	genesisPath string,
	subnetEVMVersion string,
	getRPCVersionFromBinary bool,
	subnetEVMChainID uint64,
	subnetEVMTokenSymbol string,
	useSubnetEVMDefaults bool,
	useWarp bool,
	teleporterInfo *teleporterSDK.Info,
) ([]byte, *models.Sidecar, error) {
	var (
		genesisBytes []byte
		sc           *models.Sidecar
		err          error
		rpcVersion   int
	)

	if getRPCVersionFromBinary {
		_, vmBin, err := binutils.SetupSubnetEVM(app, subnetEVMVersion)
		if err != nil {
			return nil, &models.Sidecar{}, fmt.Errorf("failed to install subnet-evm: %w", err)
		}
		rpcVersion, err = GetVMBinaryProtocolVersion(vmBin)
		if err != nil {
			return nil, &models.Sidecar{}, fmt.Errorf("unable to get RPC version: %w", err)
		}
	} else {
		rpcVersion, err = GetRPCProtocolVersion(app, models.SubnetEvm, subnetEVMVersion)
		if err != nil {
			return nil, &models.Sidecar{}, err
		}
	}

	if genesisPath == "" {
		var genesisParams subnet.EVMGenesisParams
		genesisParams, sc, err = createEvmGenesisBytes(
			app,
			subnetName,
			subnetEVMVersion,
			rpcVersion,
			subnetEVMChainID,
			subnetEVMTokenSymbol,
			useSubnetEVMDefaults,
			useWarp,
			teleporterInfo,
		)
		if err != nil {
			return nil, &models.Sidecar{}, err
		}
		subnetParams := subnet.SubnetParams{
			SubnetEVM: &subnet.SubnetEVMParams{
				EnableWarp:    useWarp,
				GenesisParams: &genesisParams,
			},
			Name: subnetName,
		}
		newSubnet, err := subnet.New(&subnetParams)
		if err != nil {
			return nil, &models.Sidecar{}, err
		}
		genesisBytes = newSubnet.Genesis
	} else {
		ux.Logger.PrintToUser("importing genesis for subnet %s", subnetName)
		genesisBytes, err = os.ReadFile(genesisPath)
		if err != nil {
			return nil, &models.Sidecar{}, err
		}

		sc = &models.Sidecar{
			Name:       subnetName,
			VM:         models.SubnetEvm,
			VMVersion:  subnetEVMVersion,
			RPCVersion: rpcVersion,
			Subnet:     subnetName,
		}
	}

	return genesisBytes, sc, nil
}

func createEvmGenesisBytes(
	app *application.Avalanche,
	subnetName string,
	subnetEVMVersion string,
	rpcVersion int,
	subnetEVMChainID uint64,
	subnetEVMTokenSymbol string,
	useSubnetEVMDefaults bool,
	useWarp bool,
	teleporterInfo *teleporterSDK.Info,
) (subnet.EVMGenesisParams, *models.Sidecar, error) {
	ux.Logger.PrintToUser("creating genesis for subnet %s", subnetName)

	genesis := core.Genesis{}
	genesis.Timestamp = *utils.TimeToNewUint64(time.Now())

	conf := params.SubnetEVMDefaultChainConfig
	conf.NetworkUpgrades = params.NetworkUpgrades{}

	const (
		descriptorsState = "descriptors"
		feeState         = "fee"
		airdropState     = "airdrop"
		precompilesState = "precompiles"
	)

	var (
		chainID     *big.Int
		tokenSymbol string
		allocation  core.GenesisAlloc
		direction   statemachine.StateDirection
		err         error
	)

	subnetEvmState, err := statemachine.NewStateMachine(
		[]string{descriptorsState, feeState, airdropState, precompilesState},
	)
	if err != nil {
		return subnet.EVMGenesisParams{}, nil, err
	}
	for subnetEvmState.Running() {
		switch subnetEvmState.CurrentState() {
		case descriptorsState:
			chainID, tokenSymbol, direction, err = getDescriptors(
				app,
				subnetEVMChainID,
				subnetEVMTokenSymbol,
			)
		case feeState:
			*conf, direction, err = GetFeeConfig(*conf, app, useSubnetEVMDefaults)
		case airdropState:
			allocation, direction, err = getAllocation(
				app,
				subnetName,
				defaultEvmAirdropAmount,
				oneAvax,
				fmt.Sprintf("Amount to airdrop (in %s units)", tokenSymbol),
				useSubnetEVMDefaults,
			)
		case precompilesState:
			*conf, direction, err = getPrecompiles(*conf, app, &genesis.Timestamp, useSubnetEVMDefaults, useWarp)
		default:
			err = errors.New("invalid creation stage")
		}
		if err != nil {
			return subnet.EVMGenesisParams{}, nil, err
		}
		subnetEvmState.NextState(direction)
	}

	sc := &models.Sidecar{
		Name:        subnetName,
		VM:          models.SubnetEvm,
		VMVersion:   subnetEVMVersion,
		RPCVersion:  rpcVersion,
		Subnet:      subnetName,
		TokenSymbol: tokenSymbol,
		TokenName:   tokenSymbol + " Token",
	}

	genesisParams := subnet.EVMGenesisParams{
		ChainID:        chainID,
		FeeConfig:      conf.FeeConfig,
		Allocation:     allocation,
		Precompiles:    conf.GenesisPrecompiles,
		TeleporterInfo: teleporterInfo,
	}

	return genesisParams, sc, nil
}

func ensureAdminsHaveBalance(admins []common.Address, alloc core.GenesisAlloc) error {
	if len(admins) < 1 {
		return nil
	}

	for _, admin := range admins {
		// we can break at the first admin who has a non-zero balance
		if bal, ok := alloc[admin]; ok &&
			bal.Balance != nil &&
			bal.Balance.Uint64() > uint64(0) {
			return nil
		}
	}
	return errors.New(
		"none of the addresses in the transaction allow list precompile have any tokens allocated to them. Currently, no address can transact on the network. Airdrop some funds to one of the allow list addresses to continue",
	)
}

func GetVMVersion(
	app *application.Avalanche,
	vmName string,
	repoName string,
	vmVersion string,
) (string, error) {
	var err error
	switch vmVersion {
	case "latest":
		vmVersion, err = app.Downloader.GetLatestReleaseVersion(binutils.GetGithubLatestReleaseURL(
			constants.AvaLabsOrg,
			repoName,
		))
		if err != nil {
			return "", err
		}
	case "pre-release":
		vmVersion, err = app.Downloader.GetLatestPreReleaseVersion(
			constants.AvaLabsOrg,
			repoName,
		)
		if err != nil {
			return "", err
		}
	case "":
		vmVersion, err = askForVMVersion(app, vmName, repoName)
		if err != nil {
			return "", err
		}
	}
	return vmVersion, nil
}

func askForVMVersion(
	app *application.Avalanche,
	vmName string,
	repoName string,
) (string, error) {
	latestReleaseVersion, err := app.Downloader.GetLatestReleaseVersion(
		binutils.GetGithubLatestReleaseURL(
			constants.AvaLabsOrg,
			repoName,
		),
	)
	if err != nil {
		return "", err
	}
	latestPreReleaseVersion, err := app.Downloader.GetLatestPreReleaseVersion(
		constants.AvaLabsOrg,
		repoName,
	)
	if err != nil {
		return "", err
	}

	useCustom := "Specify custom version"
	useLatestRelease := "Use latest release version" + versionComments[latestReleaseVersion]
	useLatestPreRelease := "Use latest pre-release version" + versionComments[latestPreReleaseVersion]

	defaultPrompt := fmt.Sprintf("What version of %s would you like?", vmName)

	versionOptions := []string{useLatestRelease, useCustom}
	if latestPreReleaseVersion != latestReleaseVersion {
		versionOptions = []string{useLatestPreRelease, useLatestRelease, useCustom}
	}

	versionOption, err := app.Prompt.CaptureList(
		defaultPrompt,
		versionOptions,
	)
	if err != nil {
		return "", err
	}

	if versionOption == useLatestPreRelease {
		return latestPreReleaseVersion, err
	}

	if versionOption == useLatestRelease {
		return latestReleaseVersion, err
	}

	// prompt for version
	versions, err := app.Downloader.GetAllReleasesForRepo(
		constants.AvaLabsOrg,
		constants.SubnetEVMRepoName,
	)
	if err != nil {
		return "", err
	}
	version, err := app.Prompt.CaptureList("Pick the version for this VM", versions)
	if err != nil {
		return "", err
	}

	return version, nil
}
