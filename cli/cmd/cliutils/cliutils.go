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
	"barbe/core/wasm"
	"barbe/core/zipper_fmt"
	"context"
	"github.com/hashicorp/go-envparse"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func makeConfiguredFetcher(ctx context.Context) *fetcher.Fetcher {
	mFetcher := fetcher.NewFetcher()
	// anyfront/*.*=anyfront/*.*:dev
	// */aws_iam_role=anyfront/aws_iam_role:dev
	if os.Getenv("BARBE_VERSION_MAP") != "" {
		versionMap := make(map[string]string)
		pairs := strings.Split(os.Getenv("BARBE_VERSION_MAP"), ",")
		for _, pair := range pairs {
			split := strings.SplitN(pair, "=", 2)
			if len(split) != 2 {
				log.Ctx(ctx).Warn().Msgf("invalid version map entry: '%s'", pair)
			}
			versionMap[split[0]] = split[1]
		}
		//very slow but also very not meant to be used with a lot of entries
		mFetcher.UrlTransformer = append(mFetcher.UrlTransformer, func(urlToTransform string) string {
			owner, component, ext, tag, err := fetcher.ParseHubIdOrUrl(urlToTransform)
			if err != nil {
				return urlToTransform
			}
			for matcher, replacement := range versionMap {
				matcherOwner, matcherComponent, matcherExt, matcherTag, err := fetcher.ParseHubIdOrUrl(matcher)
				if err != nil {
					return urlToTransform
				}
				ownerIsAMatch := matcherOwner == "*" || matcherOwner == owner
				componentIsAMatch := matcherComponent == "*" || matcherComponent == component
				extIsAMatch := matcherExt == ".*" || matcherExt == ext
				tagIsAMatch := matcherTag == "*" || matcherTag == "" || matcherTag == tag
				if ownerIsAMatch && componentIsAMatch && extIsAMatch && tagIsAMatch {
					rOwner, rComponent, rExt, rTag, err := fetcher.ParseHubIdOrUrl(replacement)
					if err != nil {
						return replacement
					}
					if rOwner == "*" {
						rOwner = owner
					}
					if rComponent == "*" {
						rComponent = component
					}
					if rExt == ".*" {
						rExt = ext
					}
					if rTag == "*" {
						rTag = tag
					}
					return fetcher.MakeBarbeHubUrl(rOwner, rComponent, rExt, rTag)
				}
			}
			return urlToTransform
		})
	}
	if os.Getenv("BARBE_LOCAL") != "" {
		localDirs := strings.Split(os.Getenv("BARBE_LOCAL"), ":")
		mFetcher.UrlTransformer = append(mFetcher.UrlTransformer, func(s string) string {
			owner, component, ext, _, err := fetcher.ParseHubIdOrUrl(s)
			if err != nil {
				return s
			}
			lookingFor := component + ext

			found := make([]string, 0)
			for _, dir := range localDirs {
				err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						log.Ctx(ctx).Warn().Err(err).Msg("failed to walk dir in url transformer")
						return nil
					}
					if d.Name() != lookingFor {
						return nil
					}
					index := strings.Index(path, owner)
					if index == -1 || index > strings.Index(path, component) {
						return nil
					}
					found = append(found, path)
					return nil
				})
				if err != nil {
					log.Ctx(ctx).Warn().Err(err).Msg("failed to walk dir in url transformer")
					continue
				}
			}
			if len(found) == 0 {
				//log.Ctx(ctx).Warn().Err(err).Msg("failed to find local component in url transformer")
				return s
			}
			sort.SliceStable(found, func(i, j int) bool {
				depthI := strings.Count(found[i], "/")
				depthJ := strings.Count(found[j], "/")
				if depthI == depthJ {
					return found[i] < found[j]
				}
				return depthI < depthJ
			})
			return found[0]
		})
	}
	return mFetcher
}

func ReadAllFilesMatching(ctx context.Context, globExprs []string) ([]fetcher.FileDescription, error) {
	mFetcher := makeConfiguredFetcher(ctx)
	allFiles := make([]fetcher.FileDescription, 0)
	dedupMap := make(map[string]struct{})
	for _, globExpr := range globExprs {
		matches, err := glob(globExpr)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to glob %s", globExpr)
		}

		if len(matches) == 0 {
			f, err := mFetcher.Fetch(globExpr)
			if err != nil {
				log.Ctx(ctx).Debug().Err(err).Msg("fetching file failed")
				continue
			}
			allFiles = append(allFiles, f)
		} else {
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
	}
	return allFiles, nil
}

func IterateDirectories(ctx context.Context, command core.MakeCommand, allFiles []fetcher.FileDescription, f func(dirFiles []fetcher.FileDescription, ctx context.Context, maker *core.Maker) error) error {
	grouped, err := groupFilesByDirectory(allFiles)
	if err != nil {
		return errors.Wrap(err, "failed to group files by directory")
	}
	for dir, files := range grouped {
		err := func() error {
			log.Ctx(ctx).Debug().Msg("executing maker for directory: '" + dir + "'")
			fileNames := make([]string, 0, len(files))
			for _, file := range files {
				fileNames = append(fileNames, file.Name)
			}
			log.Ctx(ctx).Debug().Msg("with files: [" + strings.Join(fileNames, ", ") + "]")

			maker, err := makeMaker(ctx, command, path.Join(viper.GetString("output"), dir))
			if err != nil {
				return errors.Wrap(err, "failed to create maker")
			}

			innerCtx := context.WithValue(ctx, "maker", maker)

			err = os.MkdirAll(maker.OutputDir, 0755)
			if err != nil {
				return errors.Wrapf(err, "failed to create output dir %s", maker.OutputDir)
			}
			readMeFile := path.Join(maker.OutputDir, "README.md")
			if _, err := os.Stat(readMeFile); os.IsNotExist(err) {
				err = os.WriteFile(readMeFile, []byte("This directory was generated by barbe. \n\nDo not edit manually. \n\nIt is safe to delete this folder if you have a proper state store configured (ex: `state_store{ s3 {} }`). \n\nThis folder should not be pushed to source control (add it to your .gitignore)"), 0644)
				if err != nil {
					return errors.Wrapf(err, "failed to write readme file %s", readMeFile)
				}
			}
			defer chown_util.TryRectifyRootFiles(innerCtx, []string{maker.OutputDir, readMeFile})

			err = f(files, innerCtx, maker)
			if err != nil {
				return err
			}

			allPaths := make([]string, 0)
			err = filepath.WalkDir(maker.OutputDir, func(path string, d fs.DirEntry, err error) error {
				allPaths = append(allPaths, path)
				return nil
			})
			if err != nil {
				return err
			}
			chown_util.TryRectifyRootFiles(innerCtx, allPaths)

			if command == core.MakeCommandDestroy {
				err = os.RemoveAll(maker.OutputDir)
				if err != nil {
					log.Ctx(ctx).Warn().Err(err).Msg("failed to remove output dir after destroy")
				}
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

func groupFilesByDirectory(files []fetcher.FileDescription) (map[string][]fetcher.FileDescription, error) {
	result := make(map[string][]fetcher.FileDescription)
	for _, file := range files {
		if _, _, _, _, err := fetcher.ParseBarbeHubIdentifier(file.Name); err == nil {
			result["."] = append(result["."], file)
			continue
		}
		if _, _, _, _, err := fetcher.ParseBarbeHubUrl(file.Name); err == nil {
			result["."] = append(result["."], file)
			continue
		}
		if strings.HasPrefix(file.Name, "http://") || strings.HasPrefix(file.Name, "https://") {
			result["."] = append(result["."], file)
			continue
		}
		dir := filepath.Dir(file.Name)
		result[dir] = append(result[dir], file)
	}
	return result, nil
}

func makeMaker(ctx context.Context, command core.MakeCommand, dir string) (*core.Maker, error) {
	maker := core.NewMaker(command, makeConfiguredFetcher(ctx))
	maker.OutputDir = dir
	maker.Parsers = []core.Parser{
		hcl_parser.HclParser{},
		json_parser.JsonParser{},
	}
	maker.Templaters = []core.TemplateEngine{
		//hcl_templater.HclTemplater{},
		//cue_templater.CueTemplater{},
		jsonnet_templater.JsonnetTemplater{},
		wasm.NewWasmTemplater(*zerolog.Ctx(ctx)),
		wasm.NewSpiderMonkeyTemplater(*zerolog.Ctx(ctx)),
	}
	maker.Transformers = []core.Transformer{
		//the simplifier being first is very important, it simplifies syntax that is equivalent
		//to make it a lot easier for the transformers to work with
		simplifier_transform.SimplifierTransformer{},
		traversal_manipulator.NewTraversalManipulator(),
		aws_session_provider.AwsSessionProviderTransformer{},
		gcp_token_provider.GcpTokenProviderTransformer{},
		raw_file.RawFileFormatter{},
		buildkit_runner.NewBuildkitRunner(),
		import_component.NewComponentImporter(),
	}
	maker.Formatters = []core.Formatter{
		terraform_fmt.TerraformFormatter{},
		zipper_fmt.ZipperFormatter{},
		raw_file.RawFileFormatter{},
	}
	maker.Env = map[string]string{}
	envArgs := viper.GetStringSlice("env")
	for _, envArg := range envArgs {
		if _, err := os.Stat(envArg); !os.IsNotExist(err) {
			err = (func() error {
				file, err := os.Open(envArg)
				if err != nil {
					return errors.Wrap(err, "couldnt read env file at '"+envArg+"'")
				}
				defer file.Close()
				m, err := envparse.Parse(file)
				if err != nil {
					return errors.Wrap(err, "couldnt parse env file at '"+envArg+"'")
				}
				for k, v := range m {
					maker.Env[k] = v
				}
				return nil
			})()
			if err != nil {
				return nil, err
			}
			continue
		}
		if envVal, ok := os.LookupEnv(envArg); ok {
			maker.Env[envArg] = envVal
			continue
		}
		m, err := envparse.Parse(strings.NewReader(envArg))
		if err != nil {
			return nil, errors.Wrap(err, "couldnt parse --env '"+envArg+"'")
		}
		for k, v := range m {
			maker.Env[k] = v
		}
	}
	//default keys that are included because they are known to not
	//contain sensitive information and are useful to many use cases
	defaultEnv := []string{"AWS_REGION", "BARBE_VERBOSE"}
	for _, k := range defaultEnv {
		if _, ok := maker.Env[k]; !ok {
			if envVal, ok := os.LookupEnv(k); ok {
				maker.Env[k] = envVal
			}
		}
	}

	return maker, nil
}
