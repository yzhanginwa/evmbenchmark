package run

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/yzhanginwa/evmbenchmark/lib/account"
	"github.com/yzhanginwa/evmbenchmark/lib/contract_meta_data/erc20"
	"github.com/yzhanginwa/evmbenchmark/lib/contract_meta_data/uniswap"
	"github.com/yzhanginwa/evmbenchmark/lib/generator"
	limiterpkg "github.com/yzhanginwa/evmbenchmark/lib/limiter"
)

const (
	simpleTransferGasLimit = uint64(21000)
	contractCallGasLimit   = uint64(210000)
	simpleTransferValue    = int64(10000000000000) // 1/100,000 ETH in wei
)

type Transmitter struct {
	RpcUrl  string
	limiter *limiterpkg.RateLimiter
}

func NewTransmitter(rpcUrl string, limiter *limiterpkg.RateLimiter) (*Transmitter, error) {
	return &Transmitter{
		RpcUrl:  rpcUrl,
		limiter: limiter,
	}, nil
}

// Broadcast spawns one goroutine per sender. Each goroutine generates and signs
// transactions on-the-fly until the sender's ETH balance is exhausted.
func (t *Transmitter) Broadcast(senders []generator.SenderInfo, txType string, gasPrice *big.Int) error {
	ch := make(chan error, len(senders))

	for _, si := range senders {
		go func(si generator.SenderInfo) {
			client, err := ethclient.Dial(t.RpcUrl)
			if err != nil {
				ch <- err
				return
			}
			defer client.Close()

			balance := new(big.Int).Set(si.Balance)

			// Pre-compute per-tx cost (constant for a given txType and gasPrice).
			var gasLimit uint64
			var transferValue *big.Int
			switch txType {
			case "simple":
				gasLimit = simpleTransferGasLimit
				transferValue = big.NewInt(simpleTransferValue)
			default: // erc20, uniswap
				gasLimit = contractCallGasLimit
				transferValue = big.NewInt(0)
			}
			gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
			cost := new(big.Int).Add(gasCost, transferValue)

			txIndex := 0
			for {
				if balance.Cmp(cost) < 0 {
					break
				}

				nonce := si.Account.GetNonce()

				var tx *types.Transaction
				switch txType {
				case "simple":
					recipient, err := account.GenerateRandomAddress()
					if err != nil {
						ch <- err
						return
					}
					tx, err = generator.GenerateSimpleTransferTx(
						si.Account.PrivateKey, recipient, nonce,
						si.ChainID, gasPrice, transferValue, si.EIP1559,
					)
					if err != nil {
						ch <- err
						return
					}
				case "erc20":
					recipient, err := account.GenerateRandomAddress()
					if err != nil {
						ch <- err
						return
					}
					tx, err = generator.GenerateContractCallingTx(
						si.Account.PrivateKey,
						si.ContractAddress,
						nonce,
						si.ChainID,
						gasPrice,
						gasLimit,
						erc20.MyTokenABI,
						"transfer",
						common.HexToAddress(recipient),
						big.NewInt(1000),
					)
					if err != nil {
						ch <- err
						return
					}
				case "uniswap":
					var amount0out, amount1out *big.Int
					if txIndex%2 == 1 {
						amount0out = big.NewInt(1000)
						amount1out = big.NewInt(0)
					} else {
						amount0out = big.NewInt(0)
						amount1out = big.NewInt(1000)
					}
					tx, err = generator.GenerateContractCallingTx(
						si.Account.PrivateKey,
						si.ContractAddress,
						nonce,
						si.ChainID,
						gasPrice,
						gasLimit,
						uniswap.UniswapV2PairABI,
						"swap",
						amount0out,
						amount1out,
						si.Account.Address,
						[]byte{},
					)
					if err != nil {
						ch <- err
						return
					}
				}

				if t.limiter != nil {
					t.limiter.Acquire()
				}

				if err := broadcast(client, tx); err != nil {
					ch <- err
					return
				}

				balance.Sub(balance, cost)
				txIndex++
			}

			ch <- nil
		}(si)
	}

	for i := 0; i < len(senders); i++ {
		if err := <-ch; err != nil {
			return err
		}
	}

	return nil
}

// BroadcastTxsMap broadcasts a pre-generated map of transactions (used by the
// load command which replays transactions stored on disk by gentx).
func (t *Transmitter) BroadcastTxsMap(txsMap map[int]types.Transactions) error {
	ch := make(chan error, len(txsMap))

	for _, txs := range txsMap {
		go func(txs []*types.Transaction) {
			client, err := ethclient.Dial(t.RpcUrl)
			if err != nil {
				ch <- err
				return
			}
			defer client.Close()

			for _, tx := range txs {
				if t.limiter != nil {
					t.limiter.Acquire()
				}
				if err := broadcast(client, tx); err != nil {
					ch <- err
					return
				}
			}
			ch <- nil
		}(txs)
	}

	for i := 0; i < len(txsMap); i++ {
		if err := <-ch; err != nil {
			return err
		}
	}

	return nil
}

func broadcast(client *ethclient.Client, tx *types.Transaction) error {
	err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return err
	}
	return nil
}
