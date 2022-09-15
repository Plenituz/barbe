package cmd

import (
	"barbe/cli/logger"
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var applyCmd = &cobra.Command{
	Use:     "apply [GLOB...]",
	Short:   "Generate files based on the given configuration, and execute all the appliers that will deploy the generated files",
	Args:    cobra.ArbitraryArgs,
	Example: "barbe apply config.hcl --output dist\nbarbe apply **/*.hcl --output dist",
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

		allFiles := make([]core.FileDescription, 0)
		for _, arg := range args {
			matches, err := glob(arg)
			if err != nil {
				log.Ctx(ctx).Fatal().Err(err).Msg("glob matching failed")
			}
			for _, match := range matches {
				fileContent, err := os.ReadFile(match)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("reading file failed")
					continue
				}
				allFiles = append(allFiles, core.FileDescription{
					Name:    match,
					Content: fileContent,
				})
			}
		}

		grouped := groupFilesByDirectory(dedup(allFiles))
		for dir, files := range grouped {
			log.Ctx(ctx).Debug().Msg("executing maker for directory: '" + dir + "'")

			fileNames := make([]string, 0, len(files))
			for _, file := range files {
				fileNames = append(fileNames, file.Name)
			}
			log.Ctx(ctx).Debug().Msg("with files: [" + strings.Join(fileNames, ", ") + "]")

			maker := makeMaker(path.Join(viper.GetString("output"), dir))
			innerCtx := context.WithValue(ctx, "maker", maker)

			err := os.MkdirAll(maker.OutputDir, 0755)
			if err != nil {
				log.Ctx(innerCtx).Fatal().Err(err).Msg("failed to create output directory")
			}
			chown_util.TryRectifyRootFiles(innerCtx, []string{maker.OutputDir})

			err = maker.Make(innerCtx, files, true)
			if err != nil {
				log.Ctx(innerCtx).Fatal().Err(err).Msg("generation failed")
			}

			allPaths := make([]string, 0)
			filepath.WalkDir(maker.OutputDir, func(path string, d fs.DirEntry, err error) error {
				allPaths = append(allPaths, path)
				return nil
			})
			chown_util.TryRectifyRootFiles(innerCtx, allPaths)
		}
	},
}
