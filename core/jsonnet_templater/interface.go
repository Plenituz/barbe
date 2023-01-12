package jsonnet_templater

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	_ "embed"
	"path"
)

type JsonnetTemplater struct{}

func (h JsonnetTemplater) Name() string {
	return "jsonnet_templater"
}

func (h JsonnetTemplater) Apply(ctx context.Context, maker *core.Maker, input core.ConfigContainer, template fetcher.FileDescription) (core.ConfigContainer, error) {
	if path.Ext(template.Name) != ".jsonnet" {
		c := core.NewConfigContainer()
		return *c, nil
	}
	output := core.NewConfigContainer()
	err := executeJsonnet(ctx, maker, input, output, template)
	if err != nil {
		return core.ConfigContainer{}, err
	}
	return *output, nil
}

//go:embed barbe/utils.jsonnet
var Builtins string
