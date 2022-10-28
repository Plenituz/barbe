package cmd

import (
	"barbe/analytics"
	"barbe/cli/cmd/cliutils"
	"barbe/cli/logger"
	"barbe/core"
	"context"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var destroyCmd = &cobra.Command{
	Use:     "destroy [GLOB...]",
	Short:   "Generate files based on the given configuration, and execute all the appliers that will destroy the previously deployed resources",
	Args:    cobra.ArbitraryArgs,
	Example: "barbe destroy config.hcl\nbarbe destroy **/*.hcl --output dist",
	Run: func(cmd *cobra.Command, args []string) {
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			panic(err)
		}

		lg := logger.New()
		ctx := lg.WithContext(cmd.Context())

		if len(args) == 0 {
			args = []string{"*.hcl"}
		}
		log.Ctx(ctx).Debug().Msgf("running with args: %v", args)

		allFiles, err := cliutils.ReadAllFilesMatching(ctx, args)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to read files")
		}

		fileNames := make([]string, 0, len(allFiles))
		for _, file := range allFiles {
			fileNames = append(fileNames, file.Name)
		}
		analytics.QueueEvent(ctx, analytics.AnalyticsEvent{
			EventType: "ExecutionStart",
			EventProperties: map[string]interface{}{
				"Files":     fileNames,
				"FileCount": len(allFiles),
				"Command":   "apply",
			},
		})

		err = cliutils.IterateDirectories(ctx, allFiles, func(files []core.FileDescription, ctx context.Context, maker *core.Maker) error {
			_, err = maker.Make(ctx, files, core.MakeCommandDestroy)
			if err != nil {
				log.Ctx(ctx).Fatal().Err(err).Msg("generation failed")
			}
			return nil
		})
		if err != nil {
			analytics.QueueEvent(ctx, analytics.AnalyticsEvent{
				EventType: "ExecutionEnd",
				EventProperties: map[string]interface{}{
					"Error": err.Error(),
				},
			})
			lg.Fatal().Err(err).Msg("")
		}
		analytics.QueueEvent(ctx, analytics.AnalyticsEvent{
			EventType: "ExecutionEnd",
			EventProperties: map[string]interface{}{
				"Success": true,
			},
		})
	},
}
