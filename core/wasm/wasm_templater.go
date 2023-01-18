package wasm

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
	"path"
	"time"
)

var rpcFuncBase = map[string]RpcFunc{
	"sleep": func(args []any) (any, error) {
		millis, ok := args[0].(float64)
		if !ok {
			return nil, errors.New("sleep: invalid argument")
		}
		time.Sleep(time.Duration(millis) * time.Millisecond)
		return nil, nil
	},
}

type WasmTemplater struct {
	executor        *WasmExecutor
	compiledModules map[string]wazero.CompiledModule
}

func NewWasmTemplater(logger zerolog.Logger) *WasmTemplater {
	return &WasmTemplater{
		executor:        NewWasmExecutor(logger),
		compiledModules: map[string]wazero.CompiledModule{},
	}
}

func (h *WasmTemplater) Name() string {
	return "wasm_templater"
}

func (h *WasmTemplater) Apply(ctx context.Context, maker *core.Maker, input core.ConfigContainer, template fetcher.FileDescription) (core.ConfigContainer, error) {
	if path.Ext(template.Name) != ".wasm" {
		return *core.NewConfigContainer(), nil
	}
	output := core.NewConfigContainer()
	err := h.executeWasm(ctx, maker, input, output, template)
	if err != nil {
		return core.ConfigContainer{}, err
	}
	return *output, nil
}

func (h *WasmTemplater) executeWasm(ctx context.Context, maker *core.Maker, input core.ConfigContainer, output *core.ConfigContainer, template fetcher.FileDescription) error {
	code, ok := h.compiledModules[template.Name]
	if !ok {
		var err error
		code, err = h.executor.Compile(template.Content)
		if err != nil {
			return errors.Wrap(err, "failed to compile wasm")
		}
		h.compiledModules[template.Name] = code
	}

	ctxObjJson, err := json.Marshal(input.DataBags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal input databags")
	}

	funcs := map[string]RpcFunc{
		"exportDatabags": func(args []any) (any, error) {
			fmt.Println("exportDatabags", args)
			return nil, nil
		},
	}
	for k, v := range rpcFuncBase {
		funcs[k] = v
	}
	protocol := RpcProtocol{
		logger:              *zerolog.Ctx(ctx),
		RegisteredFunctions: funcs,
	}

	err = h.executor.Execute(code, protocol, ctxObjJson)
	if err != nil {
		return errors.Wrap(err, "failed to execute wasm for '"+template.Name+"'")
	}

	return nil
}
