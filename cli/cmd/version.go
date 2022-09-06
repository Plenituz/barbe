package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"barbe/core"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("barbe " + core.Version)
	},
}
