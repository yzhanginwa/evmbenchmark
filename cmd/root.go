package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yzhanginwa/evmbenchmark/lib/benchmark"
)

// Default faucet private key derived from the following 24 mnemonic words:
//
// abandon abandon abandon abandon abandon abandon abandon abandon
// abandon abandon abandon abandon abandon abandon abandon abandon
// abandon abandon abandon abandon abandon abandon abandon art
//
// Address: 0xf278cf59f82edcf871d630f28ecc8056f25c1cdb
const defaultFaucetPrivateKey = "0x1053fae1b3ac64f178bcc21026fd06a3f4544ec2f35338b001f02d1d8efa3d5f"

var rootCmd = &cobra.Command{
	Use:   "evmbenchmark",
	Short: "EVM blockchain benchmark tool",
	Run: func(cmd *cobra.Command, args []string) {
		httpRpc, _ := cmd.Flags().GetString("http-rpc")
		wsRpc, _ := cmd.Flags().GetString("ws-rpc")
		faucetPrivateKey, _ := cmd.Flags().GetString("faucet-private-key")
		senderCount, _ := cmd.Flags().GetInt("sender-count")
		txType, _ := cmd.Flags().GetString("tx-type")
		mempool, _ := cmd.Flags().GetInt("mempool")
		duration, _ := cmd.Flags().GetDuration("duration")

		benchmark.Run(httpRpc, wsRpc, faucetPrivateKey, senderCount, txType, mempool, duration)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("http-rpc", "", "http://127.0.0.1:8545", "RPC HTTP Endpoint")
	rootCmd.Flags().StringP("ws-rpc", "", "ws://127.0.0.1:8546", "RPC WS Endpoint")
	rootCmd.Flags().IntP("mempool", "", 5000, "Mempool size")
	rootCmd.Flags().StringP("faucet-private-key", "f", defaultFaucetPrivateKey, "Private key of a faucet account")
	rootCmd.Flags().IntP("sender-count", "s", 4, "The number of senders")
	rootCmd.Flags().StringP("tx-type", "p", "simple", "Transaction type: simple, erc20, or uniswap")
	rootCmd.Flags().DurationP("duration", "d", 60*time.Second, "Benchmark duration (e.g. 60s, 2m)")
}
