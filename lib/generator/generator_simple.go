package generator

import (
	"math/big"
)

// PrepareSimple funds the sender accounts and returns SenderInfo for on-the-fly
// transaction generation.
func (g *Generator) PrepareSimple() ([]SenderInfo, error) {
	err := g.prepareSenders()
	if err != nil {
		return nil, err
	}

	funded := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(senderFundingEth))

	senders := make([]SenderInfo, len(g.Senders))
	for i, sender := range g.Senders {
		senders[i] = SenderInfo{
			Account: sender,
			Balance: new(big.Int).Set(funded),
			ChainID: g.ChainID,
			EIP1559: g.EIP1559,
		}
	}
	return senders, nil
}
