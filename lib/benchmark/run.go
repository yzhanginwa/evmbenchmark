package benchmark

import (
	"context"
	"log"
	"time"

	"github.com/yzhanginwa/evmbenchmark/lib/generator"
)

func Run(httpRpc, wsRpc, faucetPrivateKey string, senderCount int, txType string, mempool int, duration time.Duration) {
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

	limiter := newRateLimiter(mempool)

	ctx, cancel := context.WithTimeout(context.Background(), duration)

	listener := newEthereumListener(wsRpc, limiter, cancel)
	err = listener.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	err = listener.SubscribeNewHeads()
	if err != nil {
		log.Fatalf("Failed to subscribe to new heads: %v", err)
	}

	transmitter := newTransmitter(httpRpc, limiter)

	err = transmitter.Broadcast(ctx, senders, builder)
	if err != nil {
		log.Fatalf("Failed to broadcast transactions: %v", err)
	}

	listener.Close()
	<-listener.quit
}
