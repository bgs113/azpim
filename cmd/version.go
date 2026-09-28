package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Args:  cobra.NoArgs,
	Short: "Print the azpim version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(cmd.Root().Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
