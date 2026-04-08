package run

import (
	"context"
	"log"

	"github.com/yzhanginwa/evmbenchmark/lib/generator"
	limiterpkg "github.com/yzhanginwa/evmbenchmark/lib/limiter"
)

func Run(httpRpc, wsRpc, faucetPrivateKey string, senderCount int, txType string, mempool int) {
	builder := NewTxBuilder(txType)
	if builder == nil {
		log.Fatalf("Transaction type %q is not valid", txType)
	}

	gen, err := generator.NewGenerator(httpRpc, faucetPrivateKey, senderCount)
	if err != nil {
		log.Fatalf("Failed to create generator: %v", err)
	}
	defer gen.Close()

	var senders []generator.SenderInfo

	switch txType {
	case "simple":
		senders, err = gen.PrepareSimple()
	case "erc20":
		senders, err = gen.PrepareERC20()
	case "uniswap":
		senders, err = gen.PrepareUniswap()
	}
	if err != nil {
		log.Fatalf("Failed to prepare: %v", err)
	}

	limiter := limiterpkg.NewRateLimiter(mempool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ethListener := NewEthereumListener(wsRpc, limiter)
	err = ethListener.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	err = ethListener.SubscribeNewHeads()
	if err != nil {
		log.Fatalf("Failed to subscribe to new heads: %v", err)
	}

	transmitter := NewTransmitter(httpRpc, limiter)

	err = transmitter.Broadcast(ctx, senders, builder)
	if err != nil {
		log.Fatalf("Failed to broadcast transactions: %v", err)
	}

	ethListener.Close()
	<-ethListener.quit
}
