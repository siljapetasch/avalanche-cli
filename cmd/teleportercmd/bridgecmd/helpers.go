// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package bridgecmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ava-labs/avalanche-cli/pkg/evm"
	"github.com/ava-labs/avalanche-cli/pkg/models"
	"github.com/ava-labs/avalanche-cli/pkg/utils"
	"github.com/ava-labs/avalanche-cli/pkg/ux"
	"github.com/ava-labs/avalanchego/ids"
	subnetevmabi "github.com/ava-labs/subnet-evm/accounts/abi"
	"github.com/ava-labs/subnet-evm/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

func getHubERC20Address(
	srcDir string,
	rpcURL string,
	address common.Address,
) (common.Address, error) {
	srcDir = utils.ExpandHome(srcDir)
	abiPath := filepath.Join(srcDir, "contracts/out/ERC20TokenHub.sol/ERC20TokenHub.abi.json")
	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return common.Address{}, err
	}
	metadata := &bind.MetaData{
		ABI: string(abiBytes),
	}
	abi, err := metadata.GetAbi()
	if err != nil {
		return common.Address{}, err
	}
	client, err := evm.GetClient(rpcURL)
	if err != nil {
		return common.Address{}, err
	}
	defer client.Close()
	contract := bind.NewBoundContract(address, *abi, client, client, client)
	var out []interface{}
	err = contract.Call(&bind.CallOpts{}, &out, "token")
	if err != nil {
		return common.Address{}, err
	}
	out0 := *subnetevmabi.ConvertType(out[0], new(common.Address)).(*common.Address)
	return out0, nil
}

func deployERC20Hub(
	srcDir string,
	rpcURL string,
	prefundedPrivateKey string,
	teleporterRegistryAddress common.Address,
	teleporterManagerAddress common.Address,
	erc20TokenAddress common.Address,
	erc20TokenDecimals uint8,
) (common.Address, error) {
	srcDir = utils.ExpandHome(srcDir)
	abiPath := filepath.Join(srcDir, "contracts/out/ERC20TokenHub.sol/ERC20TokenHub.abi.json")
	binPath := filepath.Join(srcDir, "contracts/out/ERC20TokenHub.sol/ERC20TokenHub.bin")
	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return common.Address{}, err
	}
	binBytes, err := os.ReadFile(binPath)
	if err != nil {
		return common.Address{}, err
	}
	metadata := &bind.MetaData{
		ABI: string(abiBytes),
		Bin: string(binBytes),
	}
	abi, err := metadata.GetAbi()
	if err != nil {
		return common.Address{}, err
	}
	bin := common.FromHex(metadata.Bin)
	client, err := evm.GetClient(rpcURL)
	if err != nil {
		return common.Address{}, err
	}
	defer client.Close()
	txOpts, err := evm.GetTxOptsWithSigner(client, prefundedPrivateKey)
	if err != nil {
		return common.Address{}, err
	}
	address, tx, _, err := bind.DeployContract(
		txOpts,
		*abi,
		bin,
		client,
		teleporterRegistryAddress,
		teleporterManagerAddress,
		erc20TokenAddress,
		erc20TokenDecimals,
	)
	if err != nil {
		return common.Address{}, err
	}
	if _, success, err := evm.WaitForTransaction(client, tx); err != nil {
		return common.Address{}, err
	} else if !success {
		return common.Address{}, fmt.Errorf("failed receipt status deploying contract")
	}
	return address, nil
}

func deployNativeHub(
	srcDir string,
	rpcURL string,
	prefundedPrivateKey string,
	teleporterRegistryAddress common.Address,
	teleporterManagerAddress common.Address,
	wrappedNativeTokenAddress common.Address,
) (common.Address, error) {
	srcDir = utils.ExpandHome(srcDir)
	abiPath := filepath.Join(srcDir, "contracts/out/NativeTokenSource.sol/NativeTokenSource.abi.json")
	binPath := filepath.Join(srcDir, "contracts/out/NativeTokenSource.sol/NativeTokenSource.bin")
	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return common.Address{}, err
	}
	binBytes, err := os.ReadFile(binPath)
	if err != nil {
		return common.Address{}, err
	}
	metadata := &bind.MetaData{
		ABI: string(abiBytes),
		Bin: string(binBytes),
	}
	abi, err := metadata.GetAbi()
	if err != nil {
		return common.Address{}, err
	}
	bin := common.FromHex(metadata.Bin)
	client, err := evm.GetClient(rpcURL)
	if err != nil {
		return common.Address{}, err
	}
	defer client.Close()
	txOpts, err := evm.GetTxOptsWithSigner(client, prefundedPrivateKey)
	if err != nil {
		return common.Address{}, err
	}
	address, tx, _, err := bind.DeployContract(txOpts, *abi, bin, client, teleporterRegistryAddress, teleporterManagerAddress, wrappedNativeTokenAddress)
	if err != nil {
		return common.Address{}, err
	}
	if _, success, err := evm.WaitForTransaction(client, tx); err != nil {
		return common.Address{}, err
	} else if !success {
		return common.Address{}, fmt.Errorf("failed receipt status deploying contract")
	}
	return address, nil
}

func deployWrappedNativeToken(
	srcDir string,
	rpcURL string,
	prefundedPrivateKey string,
	tokenName string,
) (common.Address, error) {
	srcDir = utils.ExpandHome(srcDir)
	abiPath := filepath.Join(srcDir, "contracts/out/WrappedNativeToken.sol/WrappedNativeToken.abi.json")
	binPath := filepath.Join(srcDir, "contracts/out/WrappedNativeToken.sol/WrappedNativeToken.bin")
	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return common.Address{}, err
	}
	binBytes, err := os.ReadFile(binPath)
	if err != nil {
		return common.Address{}, err
	}
	metadata := &bind.MetaData{
		ABI: string(abiBytes),
		Bin: string(binBytes),
	}
	abi, err := metadata.GetAbi()
	if err != nil {
		return common.Address{}, err
	}
	bin := common.FromHex(metadata.Bin)
	client, err := evm.GetClient(rpcURL)
	if err != nil {
		return common.Address{}, err
	}
	defer client.Close()
	txOpts, err := evm.GetTxOptsWithSigner(client, prefundedPrivateKey)
	if err != nil {
		return common.Address{}, err
	}
	address, tx, _, err := bind.DeployContract(txOpts, *abi, bin, client, tokenName)
	if err != nil {
		return common.Address{}, err
	}
	if _, success, err := evm.WaitForTransaction(client, tx); err != nil {
		return common.Address{}, err
	} else if !success {
		return common.Address{}, fmt.Errorf("failed receipt status deploying contract")
	}
	return address, nil
}

func filterSubnetsByNetwork(network models.Network, subnetNames []string) ([]string, error) {
	filtered := []string{}
	for _, subnetName := range subnetNames {
		sc, err := app.LoadSidecar(subnetName)
		if err != nil {
			return nil, err
		}
		if sc.Networks[network.Name()].BlockchainID != ids.Empty {
			filtered = append(filtered, subnetName)
		}
	}
	return filtered, nil
}

func validateSubnet(network models.Network, subnetName string) error {
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}
	if sc.Networks[network.Name()].BlockchainID == ids.Empty {
		return fmt.Errorf("subnet %s not deployed into %s", subnetName, network.Name())
	}
	return nil
}

func promptChain(
	prompt string,
	network models.Network,
	avoidCChain bool,
	avoidSubnet string,
	chainFlags *ChainFlags,
) (bool, error) {
	subnetNames, err := app.GetSubnetNames()
	if err != nil {
		return false, err
	}
	subnetNames, err = filterSubnetsByNetwork(network, subnetNames)
	if err != nil {
		return false, err
	}
	subnetNames = utils.RemoveFromSlice(subnetNames, avoidSubnet)
	cChainOption := "C-Chain"
	notListedOption := "My blockchain isn't listed"
	subnetOptions := []string{}
	if !avoidCChain {
		subnetOptions = append(subnetOptions, cChainOption)
	}
	subnetOptions = append(subnetOptions, utils.Map(subnetNames, func(s string) string { return "Subnet " + s })...)
	subnetOptions = append(subnetOptions, notListedOption)
	subnetOption, err := app.Prompt.CaptureListWithSize(
		prompt,
		subnetOptions,
		11,
	)
	if err != nil {
		return false, err
	}
	if subnetOption == notListedOption {
		ux.Logger.PrintToUser("Please import the subnet first, using the `avalanche subnet import` command suite")
		return true, nil
	}
	if subnetOption == cChainOption {
		chainFlags.CChain = true
	} else {
		chainFlags.SubnetName = strings.TrimPrefix(subnetOption, "Subnet ")
	}
	return false, nil
}
