package cmd

import (
	"barbe/analytics"
	"barbe/cli/cmd/cliutils"
	"barbe/cli/logger"
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"path"
)

var generateCmd = &cobra.Command{
	Use:     "generate [GLOB...]",
	Short:   "Generate files based on the given configuration",
	Args:    cobra.ArbitraryArgs,
	Example: "barbe generate config.hcl --output dist\nbarbe generate **/*.hcl --output dist",
	Run: func(cmd *cobra.Command, args []string) {
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			panic(err)
		}

		lg, closer := logger.New()
		defer closer()
		ctx := lg.WithContext(cmd.Context())

		if len(args) == 0 {
			args = []string{"*.hcl"}
		}
		log.Ctx(ctx).Debug().Msgf("running with args: %v", args)

		allFiles, err := cliutils.ReadAllFilesMatching(ctx, args)
		if err != nil {
			lg.Error().Err(err).Msg("failed to read files")
			return
		}

		fileNames := make([]string, 0, len(allFiles))
		for _, file := range allFiles {
			fileNames = append(fileNames, file.Name)
		}
		analytics.QueueEvent(ctx, analytics.AnalyticsEvent{
			EventType: "ExecutionStart",
			EventProperties: map[string]interface{}{
				"Files":       fileNames,
				"FileCount":   len(allFiles),
				"CurrentStep": "generate",
			},
		})

		err = cliutils.IterateDirectories(ctx, core.MakeCommandGenerate, allFiles, func(files []fetcher.FileDescription, ctx context.Context, maker *core.Maker) error {
			container, err := maker.Make(ctx, files)
			if err != nil {
				return errors.Wrap(err, "generation failed")
			}
			if viper.GetBool("debug-bags") {
				b, err := json.MarshalIndent(container, "", "  ")
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("failed to marshal container (for --debug-bags)")
				} else {
					outputFile := path.Join(maker.OutputDir, "debug-bags.json")
					err = os.WriteFile(outputFile, b, 0644)
					if err != nil {
						log.Ctx(ctx).Error().Err(err).Msg("failed to write debug-bags.json")
					}
					log.Ctx(ctx).Info().Msg("wrote databags at '" + outputFile + "'")
				}
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
			lg.Error().Err(err).Msg("")
			return
		}
		analytics.QueueEvent(ctx, analytics.AnalyticsEvent{
			EventType: "ExecutionEnd",
			EventProperties: map[string]interface{}{
				"Success": true,
			},
		})
	},
}
