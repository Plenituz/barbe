package cmd

import (
	"barbe/cli/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

var rootCmd = &cobra.Command{
	Use:   "barbe",
	Short: "A programmable syntax manipulation engine",
}

func init() {
	rootCmd.PersistentFlags().String("log-format", "auto", "Log format (auto, plain, json)")
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level")
	rootCmd.PersistentFlags().StringP("output", "o", "dist", "Output directory")

	generateCmd.PersistentFlags().Bool("debug-bags", false, "Outputs the resulting databags to the output directory, for debugging purposes")

	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(
		versionCmd,
		generateCmd,
		applyCmd,
	)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	viper.SetEnvPrefix("barbe")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func Execute() {
	lg := logger.New()
	if err := rootCmd.Execute(); err != nil {
		lg.Fatal().Err(err).Msg("failed to execute command")
	}
}
