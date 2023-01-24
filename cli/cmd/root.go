package cmd

import (
	"barbe/analytics"
	"barbe/cli/logger"
	"context"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

var rootCmd = &cobra.Command{
	Use:   "barbe",
	Short: "A programmable syntax manipulation engine",
}

func init() {
	rootCmd.PersistentFlags().String("log-format", "auto", "Log format (auto, plain, json). Format json implies --no-input")
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Log level")
	rootCmd.PersistentFlags().Bool("no-input", false, "Disable input prompts")
	rootCmd.PersistentFlags().Bool("auto-approve", false, "Automatically approve all yes/no prompts")
	rootCmd.PersistentFlags().StringP("output", "o", "barbe_dist", "Output directory")
	rootCmd.PersistentFlags().Bool("debug-bags", false, "Outputs the resulting databags to the output directory, for debugging purposes")
	rootCmd.PersistentFlags().StringArrayP("env", "e", []string{}, "Environment variables to pass to the templates, this can be either a key=value pair (FOO=bar), the name of a env variable to copy (FOO), or a file path to a .env file (./.env)")

	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(
		versionCmd,
		generateCmd,
		applyCmd,
		destroyCmd,
	)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	viper.SetEnvPrefix("barbe")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	analytics.StartConsumer(context.Background())
}

func Execute() {
	defer analytics.Flush()
	if err := rootCmd.Execute(); err != nil {
		lg, closer := logger.New()
		defer closer()
		lg.Error().Err(err).Msg("failed to execute command")
	}
}
