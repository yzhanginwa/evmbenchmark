package generator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/yzhanginwa/evmbenchmark/lib/contracts/erc20"
	"github.com/yzhanginwa/evmbenchmark/lib/contracts/uniswap"
)

// PrepareUniswap deploys Uniswap contracts, funds senders, and returns SenderInfo.
func (g *Generator) PrepareUniswap() ([]SenderInfo, error) {
	pairContract, err := g.deployUniswapPair()
	if err != nil {
		return nil, err
	}
	return g.prepare(pairContract.Hex())
}

func (g *Generator) deployUniswapPair() (common.Address, error) {
	tokenA, err := g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "Token A", "TOKENA")
	if err != nil {
		return common.Address{}, err
	}

	tokenB, err := g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "Token B", "TOKENB")
	if err != nil {
		return common.Address{}, err
	}

	factory, err := g.deployContract(uniswapContractGasLimit, uniswap.UniswapV2FactoryBin, uniswap.UniswapV2FactoryABI, g.FaucetAccount.Address)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(uniswapCreatePairGasLimit, factory, uniswap.UniswapV2FactoryABI, "createPair", tokenA, tokenB)
	if err != nil {
		return common.Address{}, err
	}

	data, err := g.callContractView(factory, uniswap.UniswapV2FactoryABI, "getPair", tokenA, tokenB)
	if err != nil {
		return common.Address{}, err
	}
	pair := data[0].(common.Address)

	amount := big.NewInt(1000000000000000000)
	err = g.executeContractFunction(erc20TransferGasLimit, tokenA, erc20.MyTokenABI, "transfer", pair, amount)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(erc20TransferGasLimit, tokenB, erc20.MyTokenABI, "transfer", pair, amount)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(uniswapMintGasLimit, pair, uniswap.UniswapV2PairABI, "mint", g.FaucetAccount.Address)
	if err != nil {
		return common.Address{}, err
	}

	return pair, nil
}
