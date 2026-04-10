package generator

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/yzhanginwa/evmbenchmark/lib/account"
	"github.com/yzhanginwa/evmbenchmark/lib/contracts/erc20"
)

// senderFundingEth is the amount of ETH sent to each sender during preparation.
const senderFundingEth = int64(10000)

// SenderInfo holds all info needed for on-the-fly transaction generation.
type SenderInfo struct {
	Account         *account.Account
	Balance         *big.Int
	ContractAddress string // empty for simple transfers
	ChainID         *big.Int
	EIP1559         bool
}

type Generator struct {
	FaucetAccount *account.Account
	Senders       []*account.Account
	client        *ethclient.Client
	ChainID       *big.Int
	GasPrice      *big.Int
	EIP1559       bool
}

func NewGenerator(rpcUrl, faucetPrivateKey string, senderCount int) (*Generator, error) {
	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		return &Generator{}, err
	}

	eip1559 := false
	header, err := client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		return &Generator{}, err
	}
	if header.BaseFee != nil {
		eip1559 = true
	}

	fmt.Println("EIP-1559:", eip1559)

	var gasPrice *big.Int
	if eip1559 {
		tipCap, tipErr := client.SuggestGasTipCap(context.Background())
		if tipErr != nil || tipCap.Sign() == 0 {
			tipCap = big.NewInt(1000000000) // default 1 Gwei tip
		}
		// GasFeeCap = baseFee * 2 + tipCap to stay above fluctuating base fee
		gasPrice = new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tipCap)
	} else {
		gasPrice, err = client.SuggestGasPrice(context.Background())
		if err != nil {
			return &Generator{}, err
		}
	}

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return &Generator{}, err
	}

	faucetAccount, err := account.CreateFaucetAccount(client, faucetPrivateKey)
	if err != nil {
		return &Generator{}, err
	}

	senders := make([]*account.Account, senderCount)
	for i := 0; i < senderCount; i++ {
		s, err := account.NewAccount(client)
		if err != nil {
			return &Generator{}, err
		}
		senders[i] = s
	}

	return &Generator{
		FaucetAccount: faucetAccount,
		Senders:       senders,
		client:        client,
		ChainID:       chainID,
		GasPrice:      gasPrice,
		EIP1559:       eip1559,
	}, nil
}

func (g *Generator) Close() {
	g.client.Close()
}

// prepare funds all senders and returns SenderInfo with the given contract address
// (empty string for simple transfers).
func (g *Generator) prepare(contractAddress string) ([]SenderInfo, error) {
	err := g.prepareSenders()
	if err != nil {
		return nil, err
	}

	funded := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(senderFundingEth))

	senders := make([]SenderInfo, len(g.Senders))
	for i, sender := range g.Senders {
		senders[i] = SenderInfo{
			Account:         sender,
			Balance:         new(big.Int).Set(funded),
			ContractAddress: contractAddress,
			ChainID:         g.ChainID,
			EIP1559:         g.EIP1559,
		}
	}
	return senders, nil
}

// PrepareSimple funds sender accounts for simple ETH transfers.
func (g *Generator) PrepareSimple() ([]SenderInfo, error) {
	return g.prepare("")
}

// PrepareERC20 deploys an ERC20 contract, funds senders, and returns SenderInfo.
func (g *Generator) PrepareERC20() ([]SenderInfo, error) {
	contractAddress, err := g.deployContract(erc20ContractGasLimit, erc20.MyTokenBin, erc20.MyTokenABI, "My Token", "MYTOKEN")
	if err != nil {
		return nil, err
	}
	return g.prepare(contractAddress.Hex())
}

func (g *Generator) prepareSenders() error {
	value := new(big.Int)
	value.Mul(big.NewInt(1e18), big.NewInt(senderFundingEth))

	txs := types.Transactions{}

	for _, recipient := range g.Senders {
		signedTx, err := GenerateSimpleTransferTx(g.FaucetAccount.PrivateKey, recipient.Address.Hex(), g.FaucetAccount.GetNonce(), g.ChainID, g.GasPrice, value, g.EIP1559)
		if err != nil {
			return err
		}

		err = g.client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			return err
		}

		txs = append(txs, signedTx)
	}

	return waitForReceipts(g.client, txs, 20*time.Second)
}

func (g *Generator) deployContract(gasLimit uint64, contractBin, contractABI string, args ...interface{}) (common.Address, error) {
	tx, err := GenerateContractCreationTx(
		g.FaucetAccount.PrivateKey,
		g.FaucetAccount.GetNonce(),
		g.ChainID,
		g.GasPrice,
		gasLimit,
		contractBin,
		contractABI,
		args...,
	)
	if err != nil {
		return common.Address{}, err
	}

	err = g.client.SendTransaction(context.Background(), tx)
	if err != nil {
		return common.Address{}, err
	}

	return bind.WaitDeployed(context.Background(), g.client, tx)
}

func (g *Generator) executeContractFunction(gasLimit uint64, contractAddress common.Address, contractABI, methodName string, args ...interface{}) error {
	tx, err := GenerateContractCallingTx(
		g.FaucetAccount.PrivateKey,
		contractAddress.Hex(),
		g.FaucetAccount.GetNonce(),
		g.ChainID,
		g.GasPrice,
		gasLimit,
		g.EIP1559,
		contractABI,
		methodName,
		args...,
	)
	if err != nil {
		return err
	}

	err = g.client.SendTransaction(context.Background(), tx)
	if err != nil {
		return err
	}

	_, err = bind.WaitMined(context.Background(), g.client, tx)
	return err
}

func (g *Generator) callContractView(contractAddress common.Address, contractABI, methodName string, args ...interface{}) ([]interface{}, error) {
	parsedABI, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack(methodName, args...)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &contractAddress,
		Data: data,
	}

	result, err := g.client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, err
	}

	return parsedABI.Unpack(methodName, result)
}

func waitForReceipts(client *ethclient.Client, txs types.Transactions, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, tx := range txs {
		txHash := tx.Hash()
		for {
			_, err := client.TransactionReceipt(context.Background(), txHash)
			if err == nil {
				break
			}
			if err != ethereum.NotFound {
				return err
			}
			select {
			case <-ctx.Done():
				return errors.New("timeout waiting for transaction receipts")
			default:
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
	return nil
}
