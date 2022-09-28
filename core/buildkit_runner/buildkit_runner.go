package buildkit_runner

import (
	"barbe/core"
	"barbe/core/buildkit_runner/buildkit_status"
	"barbe/core/buildkit_runner/buildkitd"
	"barbe/core/buildkit_runner/socketprovider"
	"barbe/core/chown_util"
	"bufio"
	"context"
	"fmt"
	"github.com/containerd/containerd/platforms"
	bk "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	bkgw "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/buildinfo/types"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path"
	"strings"
	"sync"
)

type BuildkitRunner struct{}

func (t BuildkitRunner) Name() string {
	return "buildkit_runner"
}

func (t BuildkitRunner) Transform(ctx context.Context, data *core.ConfigContainer) error {
	return run(ctx, data, "buildkit_run_in_container_transform")
}

func (t BuildkitRunner) Apply(ctx context.Context, data *core.ConfigContainer) error {
	return run(ctx, data, "buildkit_run_in_container_apply")
}

func run(ctx context.Context, data *core.ConfigContainer, databagType string) error {
	executables := make([]runnerExecutable, 0)
	for resourceType, m := range data.DataBags {
		if resourceType != databagType {
			continue
		}

		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				executedToken := core.GetObjectKeyValues("executed", databag.Value.ObjectConst)
				if len(executedToken) != 0 {
					continue
				}

				var err error
				config, err := parseRunnerConfig(ctx, databag.Value.ObjectConst)
				if err != nil {
					return errors.Wrap(err, "error compiling buildkit_run_in_container")
				}
				executable, err := buildLlbDefinition(ctx, config)
				if err != nil {
					return errors.Wrap(err, "error building llb definition")
				}
				executables = append(executables, executable)

				err = data.Insert(core.DataBag{
					Name:   databag.Name,
					Type:   databag.Type,
					Labels: databag.Labels,
					Value: core.SyntaxToken{
						Type: core.TokenTypeObjectConst,
						ObjectConst: []core.ObjectConstItem{
							{
								Key: "executed",
								Value: core.SyntaxToken{
									Type:  core.TokenTypeLiteralValue,
									Value: true,
								},
							},
						},
					},
				})
				if err != nil {
					return errors.Wrap(err, "error inserting buildkit_run_in_container")
				}
			}
		}
	}
	if len(executables) == 0 {
		return nil
	}

	eg := errgroup.Group{}
	for _, executable := range executables {
		e := executable
		eg.Go(func() error {
			return executeRunner(ctx, e, data)
		})
	}
	err := eg.Wait()
	if err != nil {
		return errors.Wrap(err, "error executing buildkit_run_in_container")
	}
	return nil
}

type runnerConfig struct {
	Message             string
	RequireConfirmation bool
	ExportedFiles       map[string]string
	ReadBackFiles       []string

	Dockerfile *string
	NoCache    bool
	//or
	BaseImageName *string
	EnvVars       map[string]string
	Commands      []string
	Workdir       *string
}

type runnerExecutable struct {
	Message             string
	RequireConfirmation bool
	llbDefinition       llb.State
	ExportedFiles       map[string]string
	ReadBackFiles       []string
}

func parseRunnerConfig(ctx context.Context, objConst []core.ObjectConstItem) (runnerConfig, error) {
	output := runnerConfig{
		ExportedFiles: map[string]string{},
		EnvVars:       map[string]string{},
	}

	dockerfileToken := core.GetObjectKeyValues("dockerfile", objConst)
	if len(dockerfileToken) != 0 {
		if len(dockerfileToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple dockerfile found on buildkit_run_in_container, using the first one")
		}
		dockerfile, err := core.ExtractAsStringValue(dockerfileToken[0])
		if err != nil {
			return runnerConfig{}, errors.Wrap(err, "error extracting dockerfile value on buildkit_run_in_container")
		}
		output.Dockerfile = &dockerfile
	}

	readBackFilesToken := core.GetObjectKeyValues("read_back", objConst)
	if len(readBackFilesToken) != 0 {
		for _, readBackFileToken := range readBackFilesToken {
			switch readBackFileToken.Type {
			case core.TokenTypeArrayConst:
				for _, item := range readBackFileToken.ArrayConst {
					readBackFile, err := core.ExtractAsStringValue(item)
					if err != nil {
						return runnerConfig{}, errors.Wrap(err, "error extracting read_back value on buildkit_run_in_container")
					}
					output.ReadBackFiles = append(output.ReadBackFiles, readBackFile)
				}

			default:
				readBack, err := core.ExtractAsStringValue(readBackFileToken)
				if err != nil {
					return runnerConfig{}, errors.Wrap(err, "error extracting read_back value on buildkit_run_in_container")
				}
				output.ReadBackFiles = append(output.ReadBackFiles, readBack)
			}
		}
	}

	messageToken := core.GetObjectKeyValues("message", objConst)
	if len(messageToken) != 0 {
		for _, token := range messageToken {
			tmp, err := core.ExtractAsStringValue(token)
			if err != nil {
				log.Ctx(ctx).Warn().Msgf("error extracting message value on buildkit_run_in_container")
			}
			if output.Message != "" {
				output.Message += "\n"
			}
			output.Message += tmp
		}
	}

	requireConfirmationToken := core.GetObjectKeyValues("require_confirmation", objConst)
	if len(requireConfirmationToken) != 0 {
		if len(requireConfirmationToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple require_confirmation found on buildkit_run_in_container, using the first one")
		}
		if requireConfirmationToken[0].Type != core.TokenTypeLiteralValue {
			return runnerConfig{}, errors.New("error extracting require_confirmation value on buildkit_run_in_container")
		}
		output.RequireConfirmation = requireConfirmationToken[0].Value.(bool)
	}

	noCacheToken := core.GetObjectKeyValues("no_cache", objConst)
	if len(noCacheToken) != 0 {
		if len(noCacheToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple no_cache found on buildkit_run_in_container, using the first one")
		}
		if noCacheToken[0].Type != core.TokenTypeLiteralValue {
			log.Ctx(ctx).Warn().Msg("no_cache on buildkit_run_in_container must be a boolean")
		}
		if b, ok := noCacheToken[0].Value.(bool); ok {
			output.NoCache = b
		} else {
			log.Ctx(ctx).Warn().Msg("no_cache on buildkit_run_in_container must be a boolean")
		}
	}

	baseImageToken := core.GetObjectKeyValues("base_image", objConst)
	if len(baseImageToken) == 0 && output.Dockerfile == nil {
		return runnerConfig{}, errors.New("either base_image or dockerfile must be defined on buildkit_run_in_container")
	}
	if len(baseImageToken) > 1 {
		log.Ctx(ctx).Warn().Msg("multiple base_image found on buildkit_run_in_container, using the first one")
	}
	if len(baseImageToken) > 0 {
		baseImageName, err := core.ExtractAsStringValue(baseImageToken[0])
		if err != nil {
			return runnerConfig{}, errors.Wrap(err, "error extracting base_image value on buildkit_run_in_container")
		}
		output.BaseImageName = &baseImageName
	}

	envTokens := core.GetObjectKeyValues("env", objConst)
	for _, envToken := range envTokens {
		if envToken.Type != core.TokenTypeObjectConst {
			log.Ctx(ctx).Warn().Msg("buildkit_run_in_container env is not an object, ignoring it")
			continue
		}
		for _, pair := range envToken.ObjectConst {
			valueStr, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				log.Ctx(ctx).Err(err).Msgf("buildkit_run_in_container env value is not a string: %+v", pair.Value)
				continue
			}
			output.EnvVars[pair.Key] = valueStr
		}
	}

	commandsKeys := map[string]struct{}{
		"commands": {},
		"command":  {},
	}
	commandTokens := core.GetObjectKeysValues(commandsKeys, objConst)
	for _, commandToken := range commandTokens {
		if commandToken.Type == core.TokenTypeArrayConst {
			for _, commandTokenItem := range commandToken.ArrayConst {
				commandStr, err := core.ExtractAsStringValue(commandTokenItem)
				if err != nil {
					return runnerConfig{}, errors.Wrap(err, "error extracting command value as string on buildkit_run_in_container")
				}
				output.Commands = append(output.Commands, commandStr)
			}
		} else {
			commandStr, err := core.ExtractAsStringValue(commandToken)
			if err != nil {
				return runnerConfig{}, errors.Wrap(err, "error extracting command value as string on buildkit_run_in_container")
			}
			output.Commands = append(output.Commands, commandStr)
		}
	}

	exportedFilesKeys := map[string]struct{}{
		"exported_files": {},
		"exported_file":  {},
	}
	exportedFilesTokens := core.GetObjectKeysValues(exportedFilesKeys, objConst)
	for _, exportedFileToken := range exportedFilesTokens {
		if exportedFileToken.Type == core.TokenTypeArrayConst {
			for _, exportedFileTokenItem := range exportedFileToken.ArrayConst {
				exportedFileStr, err := core.ExtractAsStringValue(exportedFileTokenItem)
				if err != nil {
					return runnerConfig{}, errors.Wrap(err, "error extracting exported_file value as string on buildkit_run_in_container")
				}
				output.ExportedFiles[exportedFileStr] = "."
			}
		} else if exportedFileToken.Type == core.TokenTypeObjectConst {
			for _, pair := range exportedFileToken.ObjectConst {
				exportedFileStr, err := core.ExtractAsStringValue(pair.Value)
				if err != nil {
					return runnerConfig{}, errors.Wrap(err, "error extracting exported_file value as string on buildkit_run_in_container")
				}
				output.ExportedFiles[pair.Key] = exportedFileStr
			}
		} else {
			exportedFileStr, err := core.ExtractAsStringValue(exportedFileToken)
			if err != nil {
				return runnerConfig{}, errors.Wrap(err, "error extracting exported_file value as string on buildkit_run_in_container")
			}
			output.ExportedFiles[exportedFileStr] = "."
		}
	}

	workingDirToken := core.GetObjectKeyValues("workdir", objConst)
	if len(workingDirToken) > 0 {
		if len(workingDirToken) > 1 {
			log.Ctx(ctx).Warn().Msg("multiple workdir found on buildkit_run_in_container, using the first one")
		}
		workingDir, err := core.ExtractAsStringValue(workingDirToken[0])
		if err != nil {
			return runnerConfig{}, errors.Wrap(err, "error extracting workdir value as string on buildkit_run_in_container")
		}
		output.Workdir = &workingDir
	}
	return output, nil
}

func buildLlbDefinition(ctx context.Context, runnerConfig runnerConfig) (runnerExecutable, error) {
	executable := runnerExecutable{
		ExportedFiles:       runnerConfig.ExportedFiles,
		Message:             runnerConfig.Message,
		RequireConfirmation: runnerConfig.RequireConfirmation,
		ReadBackFiles:       runnerConfig.ReadBackFiles,
	}
	if runnerConfig.Dockerfile != nil {
		opts := dockerfile2llb.ConvertOpt{
			ContextByName: func(ctx context.Context, name, resolveMode string, p *specs.Platform) (*llb.State, *dockerfile2llb.Image, *binfotypes.BuildInfo, error) {
				if !strings.HasPrefix(name, "docker.io/library/src") {
					return nil, nil, nil, nil
				}
				//this is when we have a "COPY --from=src ./ /src"
				buildContext := llb.Scratch().
					File(llb.Copy(llb.Local("src"), "./", "/")).
					Dir("/")
				return &buildContext, nil, nil, nil
			},
		}
		if runnerConfig.NoCache {
			opts.IgnoreCache = []string{}
		}
		dockerfileLLb, _, _, err := dockerfile2llb.Dockerfile2LLB(ctx, []byte(*runnerConfig.Dockerfile), opts)
		if err != nil {
			return runnerExecutable{}, errors.Wrap(err, "error converting dockerfile to llb")
		}
		executable.llbDefinition = *dockerfileLLb
	} else {
		if runnerConfig.BaseImageName == nil {
			return runnerExecutable{}, errors.New("no base image name provided")
		}
		baseImage := llb.Image(*runnerConfig.BaseImageName).
			File(llb.Copy(llb.Local("src"), "./", "/src")).
			Dir("/src")

		if runnerConfig.Workdir != nil {
			baseImage = baseImage.Dir(*runnerConfig.Workdir)
		}
		for k, v := range runnerConfig.EnvVars {
			baseImage = baseImage.AddEnv(k, v)
		}
		execState := baseImage.Run(llb.Shlex("true"))
		for _, command := range runnerConfig.Commands {
			execState = execState.Run(llb.Shlex(command))
		}
		executable.llbDefinition = execState.Root()
	}
	return executable, nil
}

func executeRunner(ctx context.Context, executable runnerExecutable, container *core.ConfigContainer) error {
	maker := ctx.Value("maker").(*core.Maker)
	outputDir := maker.OutputDir

	opts := bk.SolveOpt{
		LocalDirs: map[string]string{
			"src": ".",
		},
		Session: []session.Attachable{
			socketprovider.NewDockerSocketProvider(),
		},
	}

	if len(executable.ExportedFiles) != 0 {
		root := executable.llbDefinition
		executable.llbDefinition = llb.Scratch()
		for containerPath, hostPath := range executable.ExportedFiles {
			executable.llbDefinition = executable.llbDefinition.
				File(
					llb.Copy(root, containerPath, hostPath, &llb.CopyInfo{
						CreateDestPath: true,
					}),
				)
		}
		opts.Exports = []bk.ExportEntry{
			{
				Type:      bk.ExporterLocal,
				OutputDir: outputDir,
			},
		}
	}
	if executable.Message != "" {
		log.Ctx(ctx).Info().Msg(executable.Message)
	}
	if executable.RequireConfirmation {
		var resp string
		_, err := fmt.Scanln(&resp)
		if err != nil {
			return errors.Wrap(err, "couldn't read input")
		}
		if resp != "yes" {
			return nil
		}
	}

	bkClient, err := getBuildkitClient(ctx)
	if err != nil {
		return err
	}
	platform, err := detectPlatform(ctx, bkClient)
	if err != nil {
		return err
	}
	definition, err := executable.llbDefinition.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return errors.Wrap(err, "error marshalling llb definition")
	}
	err = executeLlbDefinition(ctx, bkClient, definition, opts)
	if err != nil {
		return err
	}

	exportedFiles := make([]string, 0, len(executable.ExportedFiles))
	for _, v := range executable.ExportedFiles {
		exportedFiles = append(exportedFiles, path.Join(outputDir, v))
	}
	chown_util.TryRectifyRootFiles(ctx, exportedFiles)

	readBackFiles := make([]core.FileDescription, 0, len(executable.ReadBackFiles))
	for _, file := range executable.ReadBackFiles {
		fullPath := path.Join(outputDir, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return errors.Wrap(err, "error reading back file '"+file+"'")
		}
		readBackFiles = append(readBackFiles, core.FileDescription{
			Name:    fullPath,
			Content: content,
		})
	}
	err = maker.ParseFiles(ctx, readBackFiles, container)
	if err != nil {
		return errors.Wrap(err, "error parsing read back files")
	}

	return nil
}

func executeLlbDefinition(ctx context.Context, bkClient *bk.Client, definition *llb.Definition, opts bk.SolveOpt) error {
	buildFunc := func(ctx context.Context, c bkgw.Client) (*bkgw.Result, error) {
		sreq := bkgw.SolveRequest{
			Definition: definition.ToPB(),
		}
		res, err := c.Solve(ctx, sreq)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	wg := sync.WaitGroup{}
	defer wg.Wait()

	// Catch build events
	// Closed by buildkit
	buildCh := make(chan *bk.SolveStatus)
	wg.Add(1)
	go func() {
		defer wg.Done()

		reader, writer := io.Pipe()
		wg1 := sync.WaitGroup{}
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				line := scanner.Text()
				if line != "" {
					log.Ctx(ctx).Debug().Msg(line)
				}
			}
		}()

		err := buildkit_status.DisplaySolveStatus(ctx, "", nil, writer, buildCh)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("error displaying buildkit status")
		}
		reader.Close()
		writer.Close()
		wg1.Wait()
	}()

	_, err := bkClient.Build(ctx, opts, "", buildFunc, buildCh)
	if err != nil {
		return errors.Wrap(err, "error running buildkit build")
	}
	return nil
}

var bkHost *string
var bkPlatform *specs.Platform

func getBuildkitClient(ctx context.Context) (*bk.Client, error) {
	if bkHost == nil {
		host := os.Getenv("BUILDKIT_HOST")
		if host == "" {
			h, err := buildkitd.Start(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "error starting buildkit daemon, do you have docker installed? You might need elevated privileges")
			}
			host = h
		}
		bkHost = &host
	}
	c, err := bk.New(ctx, *bkHost)
	if err != nil {
		return nil, errors.Wrap(err, "error creating buildkit client")
	}
	return c, nil
}

func detectPlatform(ctx context.Context, client *bk.Client) (specs.Platform, error) {
	if bkPlatform != nil {
		return *bkPlatform, nil
	}
	w, err := client.ListWorkers(ctx)
	if err != nil {
		return specs.Platform{}, errors.Wrap(err, "error listing buildkit workers")
	}

	if len(w) > 0 && len(w[0].Platforms) > 0 {
		dPlatform := w[0].Platforms[0]
		bkPlatform = &dPlatform
		return dPlatform, nil
	}
	tmp := platforms.DefaultSpec()
	bkPlatform = &tmp
	return *bkPlatform, nil
}
