package buildkit_runner

import (
	"archive/tar"
	"barbe/core"
	"barbe/core/buildkit_runner/buildkit_status"
	"barbe/core/buildkit_runner/buildkitd"
	"barbe/core/buildkit_runner/socketprovider"
	"barbe/core/chown_util"
	"barbe/core/fetcher"
	"barbe/core/state_display"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/containerd/containerd/platforms"
	gitioutil "github.com/go-git/go-git/v5/utils/ioutil"
	bk "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb"
	bkgw "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/buildinfo/types"
	"github.com/moby/buildkit/util/testutil"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
)

const bagName = "buildkit_run_in_container"

type BuildkitRunner struct {
	alreadyExecuted map[string]struct{}
}

func NewBuildkitRunner() *BuildkitRunner {
	return &BuildkitRunner{
		alreadyExecuted: make(map[string]struct{}),
	}
}

func (t *BuildkitRunner) Name() string {
	return "buildkit_runner"
}

func (t *BuildkitRunner) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	executables := make([]runnerExecutable, 0)
	for resourceType, m := range data.DataBags {
		if resourceType != bagName {
			continue
		}

		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				if _, ok := t.alreadyExecuted[databag.Name]; ok {
					continue
				}
				t.alreadyExecuted[databag.Name] = struct{}{}

				var err error
				config, err := parseRunnerConfig(ctx, databag.Value.ObjectConst)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error compiling buildkit_run_in_container")
				}
				executable, err := buildLlbDefinition(ctx, config)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error building llb definition")
				}
				if executable.Name == "" {
					executable.Name = databag.Name
				}
				executables = append(executables, executable)
			}
		}
	}
	if len(executables) == 0 {
		tmp := core.NewConfigContainer()
		return *tmp, nil
	}

	eg := errgroup.Group{}
	output := core.NewConcurrentConfigContainer()
	for _, executable := range executables {
		e := executable
		eg.Go(func() error {
			return executeRunner(ctx, e, output)
		})
	}
	err := eg.Wait()
	if err != nil {
		return core.ConfigContainer{}, errors.Wrap(err, "error executing buildkit_run_in_container")
	}
	return *output.Container(), nil
}

type runnerConfig struct {
	Message               string
	DisplayName           string
	RequireConfirmation   bool
	ExportedFiles         map[string]string
	ExportedFilesLocation string
	ReadBackFiles         []string
	Excludes              []string

	Dockerfile *string
	NoCache    bool
	//or
	BaseImageName *string
	EnvVars       map[string]string
	Commands      []string
	Workdir       *string
}

type runnerExecutable struct {
	//Name is just for display
	Name                  string
	Message               string
	RequireConfirmation   bool
	llbDefinition         llb.State
	ExportedFiles         map[string]string
	ExportedFilesLocation string
	ReadBackFiles         []string
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

	exportedFilesLocationToken := core.GetObjectKeyValues("exported_files_location", objConst)
	if len(exportedFilesLocationToken) != 0 {
		for _, token := range exportedFilesLocationToken {
			tmp, err := core.ExtractAsStringValue(token)
			if err != nil {
				log.Ctx(ctx).Warn().Msgf("error extracting exported_files_location value on buildkit_run_in_container")
			}
			output.ExportedFilesLocation = tmp
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

var dockerfile2LLBMutex = sync.Mutex{}

func buildLlbDefinition(ctx context.Context, runnerConfig runnerConfig) (runnerExecutable, error) {
	executable := runnerExecutable{
		Name:                  runnerConfig.DisplayName,
		ExportedFiles:         runnerConfig.ExportedFiles,
		Message:               runnerConfig.Message,
		RequireConfirmation:   runnerConfig.RequireConfirmation,
		ReadBackFiles:         runnerConfig.ReadBackFiles,
		ExportedFilesLocation: runnerConfig.ExportedFilesLocation,
	}
	if runnerConfig.Dockerfile != nil {
		opts := dockerfile2llb.ConvertOpt{
			Excludes: runnerConfig.Excludes,
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
		//unsure why but dockerfile2llb is not thread safe, it seems to re-use an image resolver cache?
		/*
			fatal error: concurrent map writes

			goroutine 85 [running]:
			runtime.throw({0x1561074?, 0x1462c00?})
			        /usr/local/go/src/runtime/panic.go:992 +0x71 fp=0xc0025aef38 sp=0xc0025aef08 pc=0x434a11
			runtime.mapassign_faststr(0x1386d40, 0xc0057096b0, {0xc00136c000, 0x2b})
			        /usr/local/go/src/runtime/map_faststr.go:295 +0x38b fp=0xc0025aefa0 sp=0xc0025aef38 pc=0x41328b
			github.com/moby/buildkit/client/llb/imagemetaresolver.(*imageMetaResolver).ResolveImageConfig(0xc0007e3640, {0x192d988, 0xc0007e3680}, {0xc001a6c1c0, 0x20}, {0xc001107180, {0x154c3a2, 0x7}, {0xc001a6a140, 0x3d}})
			        /home/dorian/go/pkg/mod/github.com/moby/buildkit@v0.10.4/client/llb/imagemetaresolver/resolver.go:98 +0x2e8 fp=0xc0025af118 sp=0xc0025aefa0 pc=0xfa0448
			github.com/moby/buildkit/frontend/dockerfile/dockerfile2llb.Dockerfile2LLB.func2.1()
			        /home/dorian/go/pkg/mod/github.com/moby/buildkit@v0.10.4/frontend/dockerfile/dockerfile2llb/convert.go:343 +0x582 fp=0xc0025aff78 sp=0xc0025af118 pc=0xfc2022
		*/
		dockerfile2LLBMutex.Lock()
		dockerfileLLb, _, _, err := dockerfile2llb.Dockerfile2LLB(ctx, []byte(*runnerConfig.Dockerfile), opts)
		dockerfile2LLBMutex.Unlock()
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

func executeRunner(ctx context.Context, executable runnerExecutable, output *core.ConcurrentConfigContainer) (e error) {
	//state_display.AddLogLine(state_display.FindActiveMajorStepWithMinorStepNamed("buildkit_runner"), "buildkit_runner", executable.Name)
	maker := ctx.Value("maker").(*core.Maker)
	outputDir := maker.OutputDir
	state_display.GlobalState.StartMinorStep(maker.CurrentStep, executable.Name)
	defer func() {
		state_display.GlobalState.EndMinorStepWith(maker.CurrentStep, executable.Name, e != nil)
	}()

	opts := bk.SolveOpt{
		LocalDirs: map[string]string{
			"src": ".",
		},
		Session: []session.Attachable{
			socketprovider.NewDockerSocketProvider(),
		},
	}

	var tarBuffer bytes.Buffer
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
	} else if executable.ExportedFilesLocation != "" {
		wc := gitioutil.WriteNopCloser(&tarBuffer)
		opts.Exports = []bk.ExportEntry{
			{
				Type: bk.ExporterTar,
				Output: func(m map[string]string) (io.WriteCloser, error) {
					return wc, nil
				},
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

	err = executeLlbDefinition(ctx, executable.Name, func(s string) {
		log.Ctx(ctx).Debug().Msg(s)
		state_display.GlobalState.AddLogLine(maker.CurrentStep, executable.Name, s)
	}, bkClient, definition, opts)
	if err != nil {
		return err
	}

	if len(executable.ExportedFiles) != 0 {
		exportedFiles := make([]string, 0, len(executable.ExportedFiles))
		for _, v := range executable.ExportedFiles {
			exportedFiles = append(exportedFiles, path.Join(outputDir, v))
		}
		defer chown_util.TryRectifyRootFiles(ctx, exportedFiles)
	} else if executable.ExportedFilesLocation != "" {
		tarMap, err := testutil.ReadTarToMap(tarBuffer.Bytes(), false)
		if err != nil {
			return errors.Wrap(err, "error reading tar")
		}
		exportedFilesContent, ok := tarMap[strings.TrimPrefix(executable.ExportedFilesLocation, "/")]
		if !ok {
			return errors.New("exported files location not found in tar")
		}
		var exportedFiles map[string]string
		err = json.Unmarshal(exportedFilesContent.Data, &exportedFiles)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling exported files")
		}

		for nameInContainer, nameInHost := range exportedFiles {
			nameInContainerNoSlash := strings.TrimPrefix(nameInContainer, "/")
			for fileName, file := range tarMap {
				if file.Header.Typeflag == tar.TypeDir {
					continue
				}
				if file.Data == nil {
					continue
				}
				if !strings.HasPrefix(fileName, nameInContainerNoSlash) {
					continue
				}
				fileName = strings.TrimPrefix(fileName, nameInContainerNoSlash)
				fileName = strings.TrimPrefix(fileName, "/")
				fileName = path.Join(nameInHost, fileName)
				filePath := path.Join(outputDir, fileName)

				err = os.MkdirAll(path.Dir(filePath), 0755)
				if err != nil {
					return errors.Wrap(err, "error creating directory")
				}
				err = ioutil.WriteFile(filePath, file.Data, 0644)
				if err != nil {
					return errors.Wrap(err, "error writing file")
				}
			}
		}
	}

	readBackFiles := make([]fetcher.FileDescription, 0, len(executable.ReadBackFiles))
	for _, file := range executable.ReadBackFiles {
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

func executeLlbDefinition(ctx context.Context, name string, logger func(logLine string), bkClient *bk.Client, definition *llb.Definition, opts bk.SolveOpt) error {
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
					logger(line)
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
