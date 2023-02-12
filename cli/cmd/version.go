package cmd

import (
	"barbe/core/version"
	"fmt"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:          "version",
	Short:        "Print version",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("barbe " + version.Version)
	},
}
