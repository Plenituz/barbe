package cliutils

import (
	"barbe/core"
	"barbe/core/aws_session_provider"
	"barbe/core/buildkit_runner"
	"barbe/core/chown_util"
	"barbe/core/fetcher"
	"barbe/core/gcp_token_provider"
	"barbe/core/hcl_parser"
	"barbe/core/import_component"
	"barbe/core/json_parser"
	"barbe/core/jsonnet_templater"
	"barbe/core/raw_file"
	"barbe/core/simplifier_transform"
	"barbe/core/terraform_fmt"
	"barbe/core/traversal_manipulator"
	"barbe/core/zipper_fmt"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func ReadAllFilesMatching(ctx context.Context, globExprs []string) ([]fetcher.FileDescription, error) {
	allFiles := make([]fetcher.FileDescription, 0)
	dedupMap := make(map[string]struct{})
	for _, globExpr := range globExprs {
		matches, err := glob(globExpr)
		if err != nil {
			log.Ctx(ctx).Fatal().Err(err).Msg("glob matching failed")
		}
		for _, match := range matches {
			fileContent, err := os.ReadFile(match)
			if err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("reading file failed")
				continue
			}
			if _, ok := dedupMap[match]; ok {
				continue
			}
			dedupMap[match] = struct{}{}
			allFiles = append(allFiles, fetcher.FileDescription{
				Name:    match,
				Content: fileContent,
			})
		}
	}
	return allFiles, nil
}

func IterateDirectories(ctx context.Context, command core.MakeCommand, allFiles []fetcher.FileDescription, f func(dirFiles []fetcher.FileDescription, ctx context.Context, maker *core.Maker) error) error {
	grouped := groupFilesByDirectory(allFiles)
	for dir, files := range grouped {
		err := func() error {
			log.Ctx(ctx).Debug().Msg("executing maker for directory: '" + dir + "'")
			fileNames := make([]string, 0, len(files))
			for _, file := range files {
				fileNames = append(fileNames, file.Name)
			}
			log.Ctx(ctx).Debug().Msg("with files: [" + strings.Join(fileNames, ", ") + "]")

			maker := makeMaker(command, path.Join(viper.GetString("output"), dir))

			if os.Getenv("BARBE_LOCAL") != "" {
				localDirs := strings.Split(os.Getenv("BARBE_LOCAL"), ":")
				maker.Fetcher.UrlTransformer = func(s string) string {
					parsedUrl, err := url.Parse(s)
					if err != nil {
						log.Ctx(ctx).Warn().Err(err).Msg("failed to parse component name in url transformer")
						return s
					}
					split := strings.Split(parsedUrl.Path, "/")
					if len(split) < 3 {
						log.Ctx(ctx).Warn().Err(err).Msg("failed to parse component name in url transformer")
						return s
					}
					componentName := split[2]
					ext := split[len(split)-1]
					lookingFor := componentName + ext

					foundErr := errors.New("found")
					found := ""
					for _, dir := range localDirs {
						err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
							if err != nil {
								log.Ctx(ctx).Warn().Err(err).Msg("failed to walk dir in url transformer")
								return nil
							}
							if d.Name() != lookingFor {
								return nil
							}
							found = path
							return foundErr
						})
						if err != nil && !errors.Is(err, foundErr) {
							log.Ctx(ctx).Warn().Err(err).Msg("failed to walk dir in url transformer")
							continue
						}
						if found != "" {
							break
						}
					}

					if found == "" {
						log.Ctx(ctx).Warn().Err(err).Msg("failed to find local component in url transformer")
						return s
					}
					return found
				}
			}

			innerCtx := context.WithValue(ctx, "maker", maker)

			err := os.MkdirAll(maker.OutputDir, 0755)
			if err != nil {
				log.Ctx(innerCtx).Fatal().Err(err).Msg("failed to create output directory")
			}
			defer chown_util.TryRectifyRootFiles(innerCtx, []string{maker.OutputDir})

			err = f(files, innerCtx, maker)
			if err != nil {
				return err
			}

			allPaths := make([]string, 0)
			defer chown_util.TryRectifyRootFiles(innerCtx, allPaths)
			err = filepath.WalkDir(maker.OutputDir, func(path string, d fs.DirEntry, err error) error {
				allPaths = append(allPaths, path)
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

// Glob adds double-star support to the core path/filepath Glob function.
// inspired by https://github.com/yargevad/filepathx
func glob(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		// passthru to core package if no double-star
		return filepath.Glob(pattern)
	}
	return expand(strings.Split(pattern, "**"))
}

func expand(globs []string) ([]string, error) {
	var matches = []string{""} // accumulate here
	for i, glob := range globs {
		if glob == "" && i == 0 {
			glob = "./"
		}
		var hits []string
		var hitMap = map[string]bool{}
		for _, match := range matches {
			paths, err := filepath.Glob(match + glob)
			if err != nil {
				return nil, err
			}
			for _, path := range paths {
				err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					// save deduped match from current iteration
					if _, ok := hitMap[path]; !ok {
						hits = append(hits, path)
						hitMap[path] = true
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
		}
		matches = hits
	}

	// fix up return value for nil input
	if globs == nil && len(matches) > 0 && matches[0] == "" {
		matches = matches[1:]
	}

	return matches, nil
}

func groupFilesByDirectory(files []fetcher.FileDescription) map[string][]fetcher.FileDescription {
	result := make(map[string][]fetcher.FileDescription)
	for _, file := range files {
		dir := filepath.Dir(file.Name)
		result[dir] = append(result[dir], file)
	}
	return result
}

func makeMaker(command core.MakeCommand, dir string) *core.Maker {
	maker := core.NewMaker(command)
	maker.OutputDir = dir
	maker.Parsers = []core.Parser{
		hcl_parser.HclParser{},
		json_parser.JsonParser{},
	}
	maker.Templaters = []core.TemplateEngine{
		//hcl_templater.HclTemplater{},
		//cue_templater.CueTemplater{},
		jsonnet_templater.JsonnetTemplater{},
	}
	maker.Transformers = []core.Transformer{
		//the simplifier being first is very important, it simplifies syntax that is equivalent
		//to make it a lot easier for the transformers to work with
		simplifier_transform.SimplifierTransformer{},
		traversal_manipulator.NewTraversalManipulator(),
		aws_session_provider.AwsSessionProviderTransformer{},
		gcp_token_provider.GcpTokenProviderTransformer{},
		buildkit_runner.NewBuildkitRunner(),
		import_component.NewComponentImporter(),
	}
	maker.Formatters = []core.Formatter{
		terraform_fmt.TerraformFormatter{},
		zipper_fmt.ZipperFormatter{},
		raw_file.RawFileFormatter{},
	}
	return maker
}
