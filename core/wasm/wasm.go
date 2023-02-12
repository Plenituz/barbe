package wasm

import (
	"bufio"
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/assemblyscript"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"io"
	"os"
	"sync"
)

type WasmExecutor struct {
	logger zerolog.Logger

	wasmRuntime wazero.Runtime

	ctx        context.Context
	cancel     context.CancelFunc
	wgAllExecs sync.WaitGroup
}

func NewWasmExecutor(logger zerolog.Logger) *WasmExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	assemblyscript.MustInstantiate(ctx, r)
	//emscripten.MustInstantiate(ctx, r)

	return &WasmExecutor{
		logger:      logger,
		wasmRuntime: r,
		ctx:         ctx,
		cancel:      cancel,
		wgAllExecs:  sync.WaitGroup{},
	}
}

func (s *WasmExecutor) Compile(wasmContent []byte) (wazero.CompiledModule, error) {
	return s.wasmRuntime.CompileModule(s.ctx, wasmContent)
}

func (s *WasmExecutor) Execute(compiledCode wazero.CompiledModule, protocol RpcProtocol, input []byte) error {
	return errors.New("not maintained, create an issue to prioritize pure wasm support")
	s.wgAllExecs.Add(1)
	defer s.wgAllExecs.Done()

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
		//bufio.Scanner doesn't work here because it breaks if the received data is too large
		reader := bufio.NewReader(stdoutReader)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				//this will probably make the whole process hang if it happens
				s.logger.Error().Msgf("error reading stdout: %s", err.Error())
				break
			}
			lines <- line
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
		WithFS(semiRealFs{}).
		WithName(uuid.NewString())

	mod, err := s.wasmRuntime.InstantiateModule(s.ctx, compiledCode, config)
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

func (s *WasmExecutor) Close() error {
	s.cancel()
	s.wgAllExecs.Wait()
	s.wasmRuntime.Close(s.ctx)
	return nil
}
