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

// TxBuilder builds a transaction for a given sender at a given nonce.
type TxBuilder interface {
	Build(si generator.SenderInfo, nonce uint64, gasPrice *big.Int, txIndex int) (*types.Transaction, error)
	GasLimit() uint64
	TransferValue() *big.Int
}

// --- Simple transfer builder ---

type simpleTxBuilder struct{}

func (b simpleTxBuilder) GasLimit() uint64         { return 21000 }
func (b simpleTxBuilder) TransferValue() *big.Int   { return big.NewInt(10000000000000) } // 1/100,000 ETH

func (b simpleTxBuilder) Build(si generator.SenderInfo, nonce uint64, gasPrice *big.Int, txIndex int) (*types.Transaction, error) {
	recipient, err := account.GenerateRandomAddress()
	if err != nil {
		return nil, err
	}
	return generator.GenerateSimpleTransferTx(
		si.Account.PrivateKey, recipient, nonce,
		si.ChainID, gasPrice, b.TransferValue(), si.EIP1559,
	)
}

// --- ERC20 transfer builder ---

type erc20TxBuilder struct{}

func (b erc20TxBuilder) GasLimit() uint64         { return 210000 }
func (b erc20TxBuilder) TransferValue() *big.Int   { return big.NewInt(0) }

func (b erc20TxBuilder) Build(si generator.SenderInfo, nonce uint64, gasPrice *big.Int, txIndex int) (*types.Transaction, error) {
	recipient, err := account.GenerateRandomAddress()
	if err != nil {
		return nil, err
	}
	return generator.GenerateContractCallingTx(
		si.Account.PrivateKey, si.ContractAddress, nonce,
		si.ChainID, gasPrice, b.GasLimit(), si.EIP1559,
		erc20.MyTokenABI, "transfer",
		common.HexToAddress(recipient), big.NewInt(1000),
	)
}

// --- Uniswap swap builder ---

type uniswapTxBuilder struct{}

func (b uniswapTxBuilder) GasLimit() uint64         { return 210000 }
func (b uniswapTxBuilder) TransferValue() *big.Int   { return big.NewInt(0) }

func (b uniswapTxBuilder) Build(si generator.SenderInfo, nonce uint64, gasPrice *big.Int, txIndex int) (*types.Transaction, error) {
	var amount0out, amount1out *big.Int
	if txIndex%2 == 1 {
		amount0out = big.NewInt(1000)
		amount1out = big.NewInt(0)
	} else {
		amount0out = big.NewInt(0)
		amount1out = big.NewInt(1000)
	}
	return generator.GenerateContractCallingTx(
		si.Account.PrivateKey, si.ContractAddress, nonce,
		si.ChainID, gasPrice, b.GasLimit(), si.EIP1559,
		uniswap.UniswapV2PairABI, "swap",
		amount0out, amount1out, si.Account.Address, []byte{},
	)
}

// NewTxBuilder returns a TxBuilder for the given transaction type.
func NewTxBuilder(txType string) TxBuilder {
	switch txType {
	case "simple":
		return simpleTxBuilder{}
	case "erc20":
		return erc20TxBuilder{}
	case "uniswap":
		return uniswapTxBuilder{}
	default:
		return nil
	}
}

// --- Transmitter ---

type Transmitter struct {
	rpcUrl  string
	limiter *limiterpkg.RateLimiter
}

func NewTransmitter(rpcUrl string, limiter *limiterpkg.RateLimiter) *Transmitter {
	return &Transmitter{rpcUrl: rpcUrl, limiter: limiter}
}

// freshGasPrice fetches the current gas price from the node using the same
// EIP-1559 logic as the generator, so it is always up-to-date at broadcast time.
func freshGasPrice(client *ethclient.Client, eip1559 bool) (*big.Int, error) {
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
func (t *Transmitter) Broadcast(ctx context.Context, senders []generator.SenderInfo, builder TxBuilder) error {
	if len(senders) == 0 {
		return nil
	}

	gasLimit := builder.GasLimit()
	transferValue := builder.TransferValue()

	ch := make(chan error, len(senders))

	for _, si := range senders {
		go func(si generator.SenderInfo) {
			client, err := ethclient.Dial(t.rpcUrl)
			if err != nil {
				ch <- err
				return
			}
			defer client.Close()

			balance := new(big.Int).Set(si.Balance)

			gasPrice, err := freshGasPrice(client, si.EIP1559)
			if err != nil {
				ch <- err
				return
			}
			cost := new(big.Int).Add(
				new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit)),
				transferValue,
			)

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

				if txIndex > 0 && txIndex%gasPriceRefreshEvery == 0 {
					if fresh, err := freshGasPrice(client, si.EIP1559); err == nil {
						gasPrice = fresh
						cost = new(big.Int).Add(
							new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit)),
							transferValue,
						)
					}
				}

				tx, err := builder.Build(si, si.Account.GetNonce(), gasPrice, txIndex)
				if err != nil {
					ch <- err
					return
				}

				t.limiter.Acquire()

				select {
				case <-ctx.Done():
					ch <- nil
					return
				default:
				}

				if err := client.SendTransaction(context.Background(), tx); err != nil {
					ch <- err
					return
				}

				balance.Sub(balance, cost)
				txIndex++
			}

			ch <- nil
		}(si)
	}

	for range senders {
		if err := <-ch; err != nil {
			return err
		}
	}

	return nil
}
