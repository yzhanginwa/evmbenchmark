package run

import (
	"log"

	"github.com/yzhanginwa/evmbenchmark/lib/generator"
	limiterpkg "github.com/yzhanginwa/evmbenchmark/lib/limiter"
)

func Run(httpRpc, wsRpc, faucetPrivateKey string, senderCount int, txType string, mempool int, autoTune, verbose bool) {
	gen, err := generator.NewGenerator(httpRpc, faucetPrivateKey, senderCount, 0, false, "")
	if err != nil {
		log.Fatalf("Failed to create generator: %v", err)
	}

	var senders []generator.SenderInfo

	switch txType {
	case "simple":
		senders, err = gen.PrepareSimple()
	case "erc20":
		senders, err = gen.PrepareERC20()
	case "uniswap":
		senders, err = gen.PrepareUniswap()
	default:
		log.Fatalf("Transaction type \"%v\" is not valid", txType)
	}
	if err != nil {
		log.Fatalf("Failed to prepare: %v", err)
	}

	limiter := limiterpkg.NewRateLimiter(mempool)

	ethListener := NewEthereumListener(wsRpc, limiter, autoTune, verbose)
	err = ethListener.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	err = ethListener.SubscribeNewHeads()
	if err != nil {
		log.Fatalf("Failed to subscribe to new heads: %v", err)
	}

	transmitter, err := NewTransmitter(httpRpc, limiter)
	if err != nil {
		log.Fatalf("Failed to create transmitter: %v", err)
	}

	err = transmitter.Broadcast(senders, txType, gen.GasPrice)
	if err != nil {
		log.Fatalf("Failed to broadcast transactions: %v", err)
	}

	<-ethListener.quit
}
