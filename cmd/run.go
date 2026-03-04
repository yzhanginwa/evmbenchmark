/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/yzhanginwa/evmbenchmark/cmd/option"
	"github.com/yzhanginwa/evmbenchmark/lib/cmd/run"
	"github.com/spf13/cobra"
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "To run the benchmark",
	Long:  "To run the benchmark",
	Run: func(cmd *cobra.Command, args []string) {
		httpRpc, _ := cmd.Flags().GetString("http-rpc")
		wsRpc, _ := cmd.Flags().GetString("ws-rpc")
		faucetPrivateKey, _ := cmd.Flags().GetString("faucet-private-key")
		senderCount, _ := cmd.Flags().GetInt("sender-count")
		txType, _ := cmd.Flags().GetString("tx-type")
		mempool, _ := cmd.Flags().GetInt("mempool")
		autoTune, _ := cmd.Flags().GetBool("auto-tune")
		verbose, _ := cmd.Flags().GetBool("verbose")

		run.Run(httpRpc, wsRpc, faucetPrivateKey, senderCount, txType, mempool, autoTune, verbose)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	option.OptionsForRun(runCmd)
	runCmd.Flags().BoolP("auto-tune", "", false, "Automatically tune mempool size to maximize TPS")
	runCmd.Flags().BoolP("verbose", "v", false, "Print detailed status during auto-tuning")
}
