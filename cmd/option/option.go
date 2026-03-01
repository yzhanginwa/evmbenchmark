package option

import (
	"github.com/spf13/cobra"
)

// use the following 24 mnemonic words for faucet key:
//
// abandon abandon abandon abandon abandon abandon abandon abandon
// abandon abandon abandon abandon abandon abandon abandon abandon
// abandon abandon abandon abandon abandon abandon abandon art
//
// the address is: 0xf278cf59f82edcf871d630f28ecc8056f25c1cdb
//

const FaucetPrivateKey = "0x1053fae1b3ac64f178bcc21026fd06a3f4544ec2f35338b001f02d1d8efa3d5f"

func OptionsForGeneration(cmd *cobra.Command) {
	cmd.Flags().StringP("faucet-private-key", "f", FaucetPrivateKey, "Private key of a faucet account")
	cmd.Flags().IntP("sender-count", "s", 4, "The number of senders of generated transactions")
	cmd.Flags().IntP("tx-count", "t", 100000, "The number of tx count each sender will broadcast")
	cmd.Flags().StringP("tx-type", "p", "simple", "Transaction type: simple, erc20, or uniswap")
}

func OptionsForTxStore(cmd *cobra.Command) {
	cmd.Flags().StringP("tx-store-dir", "d", "/tmp/0g-benchmark-dir", "The directory of storing generated transactions")
}
