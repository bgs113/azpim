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
		fmt.Fprintf(cmd.OutOrStdout(), "azpim %s\n", cmd.Root().Version) // same line as --version
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
