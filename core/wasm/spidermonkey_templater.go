package wasm

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"path"
	"sync"
)

type SpiderMonkeyTemplater struct {
	executor *SpiderMonkeyExecutor
	err      error
	wg       *sync.WaitGroup
}

func NewSpiderMonkeyTemplater(logger zerolog.Logger) *SpiderMonkeyTemplater {
	wg := &sync.WaitGroup{}
	templater := &SpiderMonkeyTemplater{
		wg: wg,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		engine, err := NewSpiderMonkeyExecutor(logger)
		if err != nil {
			templater.err = err
			return
		}
		templater.executor = engine
	}()
	return templater
}

func (h *SpiderMonkeyTemplater) Name() string {
	return "jsonnet_templater"
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

	return nil
}
