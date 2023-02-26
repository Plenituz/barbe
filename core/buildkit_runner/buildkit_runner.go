package buildkit_runner

import (
	"barbe/cli/logger"
	"barbe/core"
	"barbe/core/buildkit_runner/buildkit_status"
	"barbe/core/buildkit_runner/buildkitd"
	"barbe/core/buildkit_runner/socketprovider"
	"barbe/core/fetcher"
	"barbe/core/state_display"
	"bufio"
	"context"
	"github.com/containerd/containerd/platforms"
	"github.com/docker/cli/cli/config"
	bk "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	bkgw "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
)

const bagName = "buildkit_run_in_container"

type BuildkitRunner struct {
	mutex           sync.Mutex
	alreadyExecuted map[string]struct{}
}

func NewBuildkitRunner() *BuildkitRunner {
	return &BuildkitRunner{
		mutex:           sync.Mutex{},
		alreadyExecuted: make(map[string]struct{}),
	}
}

func (t *BuildkitRunner) Name() string {
	return "buildkit_runner"
}

func (t *BuildkitRunner) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	runnerConfigs := make([]runnerConfig, 0)
	for resourceType, m := range data.DataBags {
		if resourceType != bagName {
			continue
		}

		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				t.mutex.Lock()
				if _, ok := t.alreadyExecuted[databag.Name]; ok {
					t.mutex.Unlock()
					continue
				}
				t.alreadyExecuted[databag.Name] = struct{}{}
				t.mutex.Unlock()

				var err error
				c, err := parseRunnerConfig(ctx, databag.Value.ObjectConst)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error compiling buildkit_run_in_container")
				}
				if c.DisplayName == "" {
					c.DisplayName = databag.Name
				}
				runnerConfigs = append(runnerConfigs, c)
			}
		}
	}
	if len(runnerConfigs) == 0 {
		return *core.NewConfigContainer(), nil
	}

	if bkHost == nil {
		err := buildkitd.CheckDocker(ctx)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			switch {
			case strings.Contains(errStr, "cannot connect to the docker daemon"):
				msg := "no container runtime running, please start one and try again."
				switch runtime.GOOS {
				case "darwin":
					msg += " On macOS you can use Docker Desktop (https://www.docker.com/get-started/) or Colima (brew install colima && colima start) or Podman (https://podman.io/getting-started/installation)."
				case "linux":
					msg += " On Linux you can use Docker (https://docs.docker.com/desktop/install/linux-install/) or Podman (https://podman.io/getting-started/installation)."
				case "windows":
					msg += " On Windows you can use Docker Desktop (https://www.docker.com/get-started/)."
				}
				return core.ConfigContainer{}, errors.New(msg)

			case strings.Contains(errStr, "executable file not found"):
				return core.ConfigContainer{}, errors.Wrap(err, "the docker CLI is not installed, please install it and try again")
			}
		}
	}

	eg := errgroup.Group{}
	output := core.NewConcurrentConfigContainer()
	for _, rConf := range runnerConfigs {
		rConf := rConf
		eg.Go(func() error {
			return executeRunner(ctx, rConf, output)
		})
	}
	err := eg.Wait()
	if err != nil {
		return core.ConfigContainer{}, errors.Wrap(err, "error executing buildkit_run_in_container")
	}
	return *output.Container(), nil
}

type runnerConfig struct {
	Message             string
	DisplayName         string
	RequireConfirmation bool
	InputFiles          map[string]string
	ExportedFiles       map[string]string
	ReadBackFiles       []string
	Excludes            []string

	Dockerfile *string
	NoCache    bool
}

func parseRunnerConfig(ctx context.Context, objConst []core.ObjectConstItem) (runnerConfig, error) {
	output := runnerConfig{
		ExportedFiles: map[string]string{},
		InputFiles:    map[string]string{},
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
	excludesToken := core.GetObjectKeyValues("excludes", objConst)
	if len(excludesToken) != 0 {
		for _, excludeToken := range excludesToken {
			switch excludeToken.Type {
			case core.TokenTypeArrayConst:
				for _, item := range excludeToken.ArrayConst {
					exclude, err := core.ExtractAsStringValue(item)
					if err != nil {
						return runnerConfig{}, errors.Wrap(err, "error extracting exclude value on buildkit_run_in_container")
					}
					output.Excludes = append(output.Excludes, exclude)
				}

			default:
				exclude, err := core.ExtractAsStringValue(excludeToken)
				if err != nil {
					return runnerConfig{}, errors.Wrap(err, "error extracting exclude value on buildkit_run_in_container")
				}
				output.Excludes = append(output.Excludes, exclude)
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
	displayNameToken := core.GetObjectKeyValues("display_name", objConst)
	if len(displayNameToken) != 0 {
		for _, token := range displayNameToken {
			tmp, err := core.ExtractAsStringValue(token)
			if err != nil {
				log.Ctx(ctx).Warn().Msgf("error extracting display_name value on buildkit_run_in_container")
			}
			output.DisplayName = tmp
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

	inputFilesTokens := core.GetObjectKeyValues("input_files", objConst)
	for _, inputFilesToken := range inputFilesTokens {
		if inputFilesToken.Type != core.TokenTypeObjectConst {
			log.Ctx(ctx).Warn().Msg("buildkit_run_in_container input_files is not an object, ignoring it")
			continue
		}
		for _, pair := range inputFilesToken.ObjectConst {
			exportedFileStr, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return runnerConfig{}, errors.Wrap(err, "error extracting input_files value as string on buildkit_run_in_container")
			}
			output.InputFiles[pair.Key] = exportedFileStr
		}
	}
	return output, nil
}

func buildLlbDefinition(ctx context.Context, runnerConfig runnerConfig, bkgwClient bkgw.Client, platform *specs.Platform) (*llb.State, error) {
	dockerOpts := dockerfile2llb.ConvertOpt{
		Excludes:       runnerConfig.Excludes,
		MetaResolver:   bkgwClient,
		TargetPlatform: platform,
		ContextByName: func(ctx context.Context, name, resolveMode string, p *specs.Platform) (*llb.State, *dockerfile2llb.Image, error) {
			if !strings.HasPrefix(name, "docker.io/library/src") {
				return nil, nil, nil
			}
			//this is when we have a "COPY --from=src ./ /src"
			buildContext := llb.Scratch().
				File(llb.Copy(llb.Local("src"), "./", "/")).
				Dir("/")
			for name, content := range runnerConfig.InputFiles {
				buildContext = buildContext.File(llb.Mkfile(name, 0755, []byte(content)))
			}

			return &buildContext, nil, nil
		},
	}
	if runnerConfig.NoCache {
		dockerOpts.IgnoreCache = []string{}
	}
	dockerfileLLb, _, _, err := dockerfile2llb.Dockerfile2LLB(ctx, []byte(*runnerConfig.Dockerfile), dockerOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error converting dockerfile to llb")
	}

	llbDef := *dockerfileLLb

	if len(runnerConfig.ExportedFiles) != 0 {
		root := llbDef
		llbDef = llb.Scratch()
		for containerPath, hostPath := range runnerConfig.ExportedFiles {
			llbDef = llbDef.File(
				llb.Copy(root, containerPath, hostPath, &llb.CopyInfo{
					CreateDestPath: true,
				}),
			)
		}
	}
	return &llbDef, nil
}

func makeSolveOptions(ctx context.Context, runnerConfig runnerConfig) bk.SolveOpt {
	opts := bk.SolveOpt{
		LocalDirs: map[string]string{
			"src": ".",
		},
		Session: []session.Attachable{
			socketprovider.NewDockerSocketProvider(),
			authprovider.NewDockerAuthProvider(config.LoadDefaultConfigFile(os.Stderr)),
		},
	}
	//if there are exported files, the output state will be a scratch state with the exported files copied from the build state
	if len(runnerConfig.ExportedFiles) != 0 {
		opts.Exports = []bk.ExportEntry{
			{
				Type:      bk.ExporterLocal,
				OutputDir: ctx.Value("maker").(*core.Maker).OutputDir,
			},
		}
	}
	return opts
}

func executeRunner(ctx context.Context, rConf runnerConfig, output *core.ConcurrentConfigContainer) (e error) {
	maker := ctx.Value("maker").(*core.Maker)
	outputDir := maker.OutputDir
	state_display.GlobalState.StartMinorStep(maker.CurrentStep, rConf.DisplayName)
	defer func() {
		state_display.GlobalState.EndMinorStepWith(maker.CurrentStep, rConf.DisplayName, e != nil)
	}()

	if rConf.Message != "" {
		if rConf.RequireConfirmation {
			resp, err := logger.PromptUserYesNo(ctx, rConf.Message)
			if err != nil {
				return errors.Wrap(err, "error prompting user")
			}
			if !resp {
				return nil
			}
		} else {
			log.Ctx(ctx).Info().Msg(rConf.Message)
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

	logBuffer := strings.Builder{}
	dispatchLog := func(s string) {
		logBuffer.WriteString(s + "\n")
		log.Ctx(ctx).Debug().Msg(s)
		state_display.GlobalState.AddLogLine(maker.CurrentStep, rConf.DisplayName, s)
	}

	buildFunc := func(ctx context.Context, c bkgw.Client) (*bkgw.Result, error) {
		llbDef, err := buildLlbDefinition(ctx, rConf, c, &platform)
		if err != nil {
			return nil, err
		}

		definition, err := llbDef.Marshal(ctx, llb.Platform(platform))
		if err != nil {
			return nil, errors.Wrap(err, "error marshalling llb definition")
		}
		sreq := bkgw.SolveRequest{
			Definition: definition.ToPB(),
		}
		res, err := c.Solve(ctx, sreq)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	err = executeLlbDefinition(ctx, rConf.DisplayName, bkClient, makeSolveOptions(ctx, rConf), dispatchLog, buildFunc)
	if err != nil {
		logFilePath := path.Join(outputDir, strings.ReplaceAll(rConf.DisplayName, "/", "_")+".log")
		errWrite := os.WriteFile(logFilePath, []byte(logBuffer.String()), 0644)
		if errWrite != nil {
			log.Ctx(ctx).Warn().Err(errWrite).Msg("error writing log file")
		}
		return errors.Wrap(err, "full command log available at '"+logFilePath+"'")
	}
	//not fully necessary, but release the memory before reading back files in case we need the memory
	logBuffer.Reset()

	readBackFiles := make([]fetcher.FileDescription, 0, len(rConf.ReadBackFiles))
	for _, file := range rConf.ReadBackFiles {
		fullPath := path.Join(outputDir, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return errors.Wrap(err, "error reading back file '"+file+"'")
		}
		readBackFiles = append(readBackFiles, fetcher.FileDescription{
			Name:    fullPath,
			Content: content,
		})
	}

	tmp := core.NewConfigContainer()
	err = maker.ParseFiles(ctx, readBackFiles, tmp)
	if err != nil {
		return errors.Wrap(err, "error parsing read back files")
	}
	err = output.MergeWith(*tmp)
	if err != nil {
		return errors.Wrap(err, "error merging read back files")
	}
	return nil
}

func executeLlbDefinition(ctx context.Context, name string, bkClient *bk.Client, opts bk.SolveOpt, logger func(logLine string), buildFunc bkgw.BuildFunc) error {
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
			//bufio.Scanner doesn't work here because it breaks if the received data is too large
			reader := bufio.NewReader(reader)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						break
					}
					if strings.Contains(err.Error(), "read/write on closed pipe") {
						break
					}
					logger("error reading bk stdout: " + err.Error())
					break
				}
				l := strings.TrimSuffix(string(line), "\n")
				if l != "" {
					logger(l)
				}
			}
		}()

		err := buildkit_status.DisplaySolveStatus(ctx, name, nil, writer, buildCh)
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
	c, err := bk.New(ctx, *bkHost, bk.WithFailFast())
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
