package wasm

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"os"
	"sync"
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

	Protocol RpcProtocol

	wasmRuntime      wazero.Runtime
	spiderMonkeyCode wazero.CompiledModule

	ctx        context.Context
	cancel     context.CancelFunc
	wgAllExecs sync.WaitGroup
}

func NewSpiderMonkeyExecutor(logger zerolog.Logger) (*SpiderMonkeyExecutor, error) {
	ctx, cancel := context.WithCancel(context.Background())

	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	//assemblyscript.MustInstantiate(ctx, r)

	//this takes a while
	spiderMonkeyCode, err := r.CompileModule(ctx, spiderMonkey)
	if err != nil {
		cancel()
		return nil, errors.Wrap(err, "error compiling spidermonkey")
	}

	return &SpiderMonkeyExecutor{
		logger:           logger,
		wasmRuntime:      r,
		spiderMonkeyCode: spiderMonkeyCode,
		ctx:              ctx,
		cancel:           cancel,
		wgAllExecs:       sync.WaitGroup{},
		Protocol: RpcProtocol{
			logger:              logger,
			RegisteredFunctions: map[string]RpcFunc{},
		},
	}, nil
}

func handleJsMessage(text []byte, logger zerolog.Logger, registeredFunctions map[string]func(args []any) (any, error), stdinWriter *os.File) {
	var req rpcRequest
	err := json.Unmarshal(text, &req)
	//all logs arrive here, so if we cant parse it, it's probably just a log
	if err != nil {
		logger.Debug().Msgf("js: %s", string(text))
		return
	}
	if req.Method == "" {
		logger.Debug().Msgf("js: %s", string(text))
		return
	}
	f, ok := registeredFunctions[req.Method]
	if !ok {
		logger.Debug().Msgf("js: %s", string(text))
		return
	}

	result, err := f(req.Params)
	if err != nil {
		logger.Error().Str("req", string(text)).Err(err).Msgf("error executing js called function '%s'", req.Method)
		resp, err := json.Marshal(rpcResponse{
			Error: err.Error(),
		})
		if err != nil {
			logger.Error().Err(err).Msg("error marshaling error response to js called function")
			return
		}
		_, err = stdinWriter.WriteString(string(resp) + "\n")
		if err != nil {
			logger.Error().Err(err).Msg("error writing error response to js called function")
			return
		}
		return
	}

	resp, err := json.Marshal(rpcResponse{
		Result: result,
	})
	if err != nil {
		logger.Error().Err(err).Msg("error marshaling success response to js called function")
		return
	}
	_, err = stdinWriter.WriteString(string(resp) + "\n")
	if err != nil {
		logger.Error().Err(err).Msg("error writing success response to js called function")
		return
	}
}

func (s *SpiderMonkeyExecutor) Execute(jsContent []byte) error {
	s.wgAllExecs.Add(1)
	defer s.wgAllExecs.Done()
	fakeFs := semiRealFs{
		"__barbe_index.js": {Data: jsContent},
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
				resp, err := s.Protocol.HandleMessage(line)
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
		WithArgs("js", "-f", "__barbe_index.js").
		WithName(uuid.NewString())

	mod, err := s.wasmRuntime.InstantiateModule(s.ctx, s.spiderMonkeyCode, config)
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
	return nil
}
