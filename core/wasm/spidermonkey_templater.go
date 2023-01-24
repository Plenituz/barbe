package wasm

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	"encoding/json"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"path"
	"sync"
)

type sugarBag struct {
	Name   string
	Type   string
	Labels []string
	Meta   map[string]interface{}
	Value  interface{}
}

type SpiderMonkeyTemplater struct {
	executor *SpiderMonkeyExecutor
	err      error
	wg       *sync.WaitGroup
}

func decodeSugarCoatedDatabags(databags interface{}) ([]core.DataBag, error) {
	var parsedPipelineItem struct {
		Databags []sugarBag `mapstructure:"databags"`
	}
	err := mapstructure.Decode(databags, &parsedPipelineItem)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode databags")
	}
	output := make([]core.DataBag, 0, len(parsedPipelineItem.Databags))
	for _, bag := range parsedPipelineItem.Databags {
		token, err := core.GoValueToToken(bag.Value)
		if err != nil {
			return nil, errors.Wrap(err, "error decoding syntax token from jsonnet template")
		}

		if bag.Labels == nil {
			bag.Labels = []string{}
		}
		realBag := core.DataBag{
			Name:   bag.Name,
			Type:   bag.Type,
			Labels: bag.Labels,
			Value:  token,
		}
		output = append(output, realBag)
	}
	return output, nil
}

func NewSpiderMonkeyTemplater(logger zerolog.Logger, outputDir string) *SpiderMonkeyTemplater {
	wg := &sync.WaitGroup{}
	templater := &SpiderMonkeyTemplater{
		wg: wg,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		engine, err := NewSpiderMonkeyExecutor(logger, outputDir)
		if err != nil {
			templater.err = err
			return
		}
		templater.executor = engine
	}()
	return templater
}

func (h *SpiderMonkeyTemplater) Name() string {
	return "js_spidermonkey_templater"
}

func (h *SpiderMonkeyTemplater) Apply(ctx context.Context, maker *core.Maker, input core.ConfigContainer, template fetcher.FileDescription) (core.ConfigContainer, error) {
	if path.Ext(template.Name) != ".js" {
		return *core.NewConfigContainer(), nil
	}
	output := core.NewConfigContainer()
	err := h.executeJs(ctx, maker, input, output, template)
	if err != nil {
		return core.ConfigContainer{}, err
	}
	return *output, nil
}

func (h *SpiderMonkeyTemplater) executeJs(ctx context.Context, maker *core.Maker, input core.ConfigContainer, output *core.ConfigContainer, template fetcher.FileDescription) error {
	if h.executor == nil {
		h.wg.Wait()
	}
	if h.err != nil {
		return errors.Wrap(h.err, "error initializing spidermonkey")
	}
	if h.executor == nil {
		return errors.New("no executor found after spidermonkey initialization")
	}

	ctxObjJson, err := json.Marshal(input.DataBags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal input databags")
	}

	funcs := map[string]RpcFunc{
		"exportDatabags": func(args []any) (any, error) {
			decodedBags, err := decodeSugarCoatedDatabags(args[0])
			if err != nil {
				return nil, err
			}
			for _, bag := range decodedBags {
				err = output.Insert(bag)
				if err != nil {
					return nil, errors.Wrap(err, "failed to insert databag")
				}
			}
			return nil, nil
		},
		"importComponents":   importComponentTemplate(ctx),
		"transformContainer": transformContainerTemplate(ctx),
	}
	for k, v := range rpcFuncBase {
		funcs[k] = v
	}
	protocol := RpcProtocol{
		logger:              *zerolog.Ctx(ctx),
		RegisteredFunctions: funcs,
	}

	envVars := map[string]string{}
	for k, v := range maker.Env {
		envVars[k] = v
	}
	envVars["BARBE_COMMAND"] = maker.Command
	envVars["BARBE_LIFECYCLE_STEP"] = maker.CurrentStep
	envVars["BARBE_OUTPUT_DIR"] = maker.OutputDir

	state := maker.StateHandler.GetState(core.ContextScopeKey(ctx))
	stateJson, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "failed to marshal state object")
	}

	err = h.executor.Execute(protocol, path.Base(template.Name), template.Content, ctxObjJson, envVars, stateJson)
	if err != nil {
		return errors.Wrap(err, "failed to execute wasm for '"+template.Name+"'")
	}

	return nil
}
