package wasm

import (
	"barbe/core/chown_util"
	"barbe/core/version"
	"bufio"
	"context"
	_ "embed"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"os"
	"sync"
	"time"
)

const (
	wazeroCachePath = "~/.cache/barbe/spidermonkey_for_" + version.Version
)

//https://spidermonkey.dev/
// find the latest version of spidermonkey wasm at
//https://mozilla-spidermonkey.github.io/sm-wasi-demo/ (it's getting requested by the webpage)
//you can find more details on the available globals at:
//https://github.com/mozilla/gecko-dev/blob/master/js/src/shell/js.cpp
//https://github.com/mozilla/gecko-dev/blob/master/js/src/shell/OSObject.cpp
//go:embed js.wasm
var spiderMonkey []byte

type SpiderMonkeyExecutor struct {
	logger zerolog.Logger

	wasmRuntimeIntepreter       wazero.Runtime
	wasmRuntimeCompiled         wazero.Runtime
	spiderMonkeyCodeInterpreter wazero.CompiledModule
	spiderMonkeyCodeCompiled    wazero.CompiledModule

	ctx        context.Context
	cancel     context.CancelFunc
	wgAllExecs sync.WaitGroup
}

func NewSpiderMonkeyExecutor(logger zerolog.Logger, outputDir string) (*SpiderMonkeyExecutor, error) {
	ctx, cancel := context.WithCancel(context.Background())

	exec := &SpiderMonkeyExecutor{
		logger:     logger,
		cancel:     cancel,
		wgAllExecs: sync.WaitGroup{},
	}

	cacheDir, err := homedir.Expand(wazeroCachePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to expand wazero cache path")
	}

	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		ctx, err = experimental.WithCompilationCacheDirName(ctx, cacheDir)
		if err != nil {
			return nil, errors.Wrap(err, "failed to set wazero cache dir")
		}
		compiledRuntime := wazero.NewRuntime(ctx)
		wasi_snapshot_preview1.MustInstantiate(ctx, compiledRuntime)

		spiderMonkeyCompiled, err := compiledRuntime.CompileModule(ctx, spiderMonkey)
		if err != nil {
			logger.Error().Err(err).Msg("error compiling spidermonkey")
		}
		exec.wasmRuntimeCompiled = compiledRuntime
		exec.spiderMonkeyCodeCompiled = spiderMonkeyCompiled
		exec.ctx = ctx
		chown_util.TryRectifyRootFiles(ctx, []string{cacheDir})
		return exec, nil
	}

	interpreter := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	wasi_snapshot_preview1.MustInstantiate(ctx, interpreter)

	//this takes a while but is way faster than the compiled version
	spiderMonkeyInterpreter, err := interpreter.CompileModule(ctx, spiderMonkey)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "error compiling spidermonkey (interpreter)")
	}

	exec.ctx = ctx
	exec.wasmRuntimeIntepreter = interpreter
	exec.spiderMonkeyCodeInterpreter = spiderMonkeyInterpreter

	go func() {
		ctx, err = experimental.WithCompilationCacheDirName(ctx, cacheDir)
		if err != nil {
			logger.Error().Err(err).Msg("failed to set wazero cache dir")
			return
		}
		compiledRuntime := wazero.NewRuntime(ctx)
		wasi_snapshot_preview1.MustInstantiate(ctx, compiledRuntime)

		spiderMonkeyCompiled, err := compiledRuntime.CompileModule(ctx, spiderMonkey)
		if err != nil {
			logger.Error().Err(err).Msg("error compiling spidermonkey")
		}
		exec.wasmRuntimeCompiled = compiledRuntime
		exec.spiderMonkeyCodeCompiled = spiderMonkeyCompiled
		exec.ctx = ctx
	}()

	return exec, nil
}

func (s *SpiderMonkeyExecutor) Execute(protocol RpcProtocol, fileName string, jsContent []byte, input []byte, envVars map[string]string, state []byte) error {
	s.wgAllExecs.Add(1)
	defer s.wgAllExecs.Done()
	fakeFs := semiRealFs{
		fileName:             {Data: jsContent},
		"__barbe_input.json": {Data: input},
		"__barbe_state.json": {Data: state},
	}

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return errors.Wrap(err, "error creating stdin pipe")
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return errors.Wrap(err, "error creating stdout pipe")
	}
	closePipes := func() {
		stdinReader.Close()
		stdinWriter.Close()
		stdoutReader.Close()
		stdoutWriter.Close()
	}
	defer closePipes()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	lines := make(chan []byte)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			lines <- scanner.Bytes()
		}
		close(lines)
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				closePipes()
				return

			case line, ok := <-lines:
				if !ok {
					return
				}
				resp, err := protocol.HandleMessage(line)
				if err != nil {
					s.logger.Error().Err(err).Msg("error handling rpc message")
					continue
				}
				if len(resp) == 0 {
					s.logger.Debug().Msgf(string(line))
					continue
				}
				_, err = stdinWriter.Write(append(resp, []byte("\n")...))
				if err != nil {
					s.logger.Error().Err(err).Msgf("error writing response to rpc function: %s", string(resp))
					return
				}
			}
		}
	}()

	config := wazero.NewModuleConfig().
		WithStdin(stdinReader).
		WithStdout(stdoutWriter).
		WithStderr(os.Stderr).
		WithFS(fakeFs).
		WithArgs("js", "-f", fileName).
		WithName(uuid.NewString())

	for k, v := range envVars {
		config = config.WithEnv(k, v)
	}

	runtime := s.wasmRuntimeIntepreter
	spiderMonkeyCode := s.spiderMonkeyCodeInterpreter
	if s.wasmRuntimeCompiled != nil && s.spiderMonkeyCodeCompiled != nil {
		runtime = s.wasmRuntimeCompiled
		spiderMonkeyCode = s.spiderMonkeyCodeCompiled
	}

	t := time.Now()
	mod, err := runtime.InstantiateModule(s.ctx, spiderMonkeyCode, config)
	s.logger.Debug().Msgf("'%s' execution took, %s", fileName, time.Since(t))
	if err != nil {
		//fmt.Println("error:", err)
		return errors.Wrap(err, "error instantiating module")
	}
	err = mod.Close(s.ctx)
	if err != nil {
		return errors.Wrap(err, "error closing module")
	}

	//this forces the logs goroutine to finish, allowing us to wait for the wait group
	stdoutWriter.Close()
	wg.Wait()
	closePipes()
	return nil
}

func (s *SpiderMonkeyExecutor) Close() error {
	s.cancel()
	s.wgAllExecs.Wait()
	if s.spiderMonkeyCodeInterpreter != nil {
		s.spiderMonkeyCodeInterpreter.Close(s.ctx)
	}
	if s.spiderMonkeyCodeCompiled != nil {
		s.spiderMonkeyCodeCompiled.Close(s.ctx)
	}
	if s.spiderMonkeyCodeInterpreter != nil {
		s.spiderMonkeyCodeInterpreter.Close(s.ctx)
	}
	if s.spiderMonkeyCodeCompiled != nil {
		s.spiderMonkeyCodeCompiled.Close(s.ctx)
	}
	return nil
}
