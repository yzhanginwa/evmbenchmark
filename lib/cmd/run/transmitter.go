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

// freshGasPrice fetches the current gas price from the node using the same
// EIP-1559 logic as the generator, so it is always up-to-date at broadcast time.
func (t *Transmitter) freshGasPrice(eip1559 bool) (*big.Int, error) {
	client, err := ethclient.Dial(t.RpcUrl)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if eip1559 {
		header, err := client.HeaderByNumber(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		tipCap, tipErr := client.SuggestGasTipCap(context.Background())
		if tipErr != nil || tipCap.Sign() == 0 {
			tipCap = big.NewInt(1_000_000_000) // 1 Gwei default
		}
		return new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tipCap), nil
	}

	return client.SuggestGasPrice(context.Background())
}

// gasPriceRefreshEvery controls how often each sender goroutine re-fetches the
// current gas price to stay ahead of rising base fees during long runs.
const gasPriceRefreshEvery = 50

// Broadcast spawns one goroutine per sender. Each goroutine generates and signs
// transactions on-the-fly until the sender's ETH balance is exhausted.
func (t *Transmitter) Broadcast(ctx context.Context, senders []generator.SenderInfo, txType string) error {
	if len(senders) == 0 {
		return nil
	}

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

			// Fetch initial gas price and compute per-tx cost.
			// cost is recomputed whenever gasPrice is refreshed.
			gasPrice, err := t.freshGasPrice(si.EIP1559)
			if err != nil {
				ch <- err
				return
			}
			gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
			cost := new(big.Int).Add(gasCost, transferValue)

			txIndex := 0
			for {
				select {
				case <-ctx.Done():
					ch <- nil
					return
				default:
				}

				if balance.Cmp(cost) < 0 {
					break
				}

				// Periodically refresh gas price to stay ahead of rising base fees.
				if txIndex > 0 && txIndex%gasPriceRefreshEvery == 0 {
					if fresh, err := t.freshGasPrice(si.EIP1559); err == nil {
						gasPrice = fresh
						gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
						cost = new(big.Int).Add(gasCost, transferValue)
					}
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
						si.EIP1559,
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
						si.EIP1559,
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

				select {
				case <-ctx.Done():
					ch <- nil
					return
				default:
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

func broadcast(client *ethclient.Client, tx *types.Transaction) error {
	err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return err
	}
	return nil
}
