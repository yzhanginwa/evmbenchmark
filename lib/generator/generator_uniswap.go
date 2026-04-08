package generator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/yzhanginwa/evmbenchmark/lib/contract_meta_data/erc20"
	"github.com/yzhanginwa/evmbenchmark/lib/contract_meta_data/uniswap"
)

func (g *Generator) prepareContractUniswap() (common.Address, error) {
	tokenA, err := g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "Token A", "TOKENA")
	if err != nil {
		return common.Address{}, err
	}

	tokenB, err := g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "Token B", "TOKENB")
	if err != nil {
		return common.Address{}, err
	}

	uniswapContract, err := g.deployContract(uniswapContractGasLimit, uniswap.UniswapV2FactoryBin, uniswap.UniswapV2FactoryABI, g.FaucetAccount.Address)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(uniswapCreatePairGasLimit, uniswapContract, uniswap.UniswapV2FactoryABI, "createPair", tokenA, tokenB)
	if err != nil {
		return common.Address{}, err
	}

	data, err := g.callContractView(uniswapContract, uniswap.UniswapV2FactoryABI, "getPair", tokenA, tokenB)
	if err != nil {
		return common.Address{}, err
	}

	// we know the first returned value is the address of the pair
	pairContract := data[0].(common.Address)

	amount := big.NewInt(1000000000000000000)
	err = g.executeContractFunction(erc20TransferGasLimit, tokenA, erc20.MyTokenABI, "transfer", pairContract, amount)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(erc20TransferGasLimit, tokenB, erc20.MyTokenABI, "transfer", pairContract, amount)
	if err != nil {
		return common.Address{}, err
	}

	err = g.executeContractFunction(uniswapMintGasLimit, pairContract, uniswap.UniswapV2PairABI, "mint", g.FaucetAccount.Address)
	if err != nil {
		return common.Address{}, err
	}

	return pairContract, nil
}

// PrepareUniswap sets up the Uniswap contracts, funds senders, and returns SenderInfo
// for on-the-fly transaction generation.
func (g *Generator) PrepareUniswap() ([]SenderInfo, error) {
	pairContract, err := g.prepareContractUniswap()
	if err != nil {
		return nil, err
	}

	err = g.prepareSenders()
	if err != nil {
		return nil, err
	}

	funded := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(senderFundingEth))

	senders := make([]SenderInfo, len(g.Senders))
	for i, sender := range g.Senders {
		senders[i] = SenderInfo{
			Account:         sender,
			Balance:         new(big.Int).Set(funded),
			ContractAddress: pairContract.Hex(),
			ChainID:         g.ChainID,
			EIP1559:         g.EIP1559,
		}
	}
	return senders, nil
}
