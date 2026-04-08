package generator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/yzhanginwa/evmbenchmark/lib/contract_meta_data/erc20"
)

func (g *Generator) prepareContractERC20() (common.Address, error) {
	return g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "My Token", "MYTOKEN")
}

// PrepareERC20 deploys the ERC20 contract, funds senders, and returns SenderInfo
// for on-the-fly transaction generation.
func (g *Generator) PrepareERC20() ([]SenderInfo, error) {
	contractAddress, err := g.prepareContractERC20()
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
			ContractAddress: contractAddress.Hex(),
			ChainID:         g.ChainID,
			EIP1559:         g.EIP1559,
		}
	}
	return senders, nil
}
