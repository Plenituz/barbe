package jsonnet_templater

import (
	"context"
	_ "embed"
	"barbe/core"
)

type JsonnetTemplater struct{}

func (h JsonnetTemplater) Name() string {
	return "jsonnet_templater"
}

func (h JsonnetTemplater) Apply(ctx context.Context, container *core.ConfigContainer, templates []core.FileDescription) error {
	return applyTemplate(ctx, container, templates)
}

//go:embed barbe/utils.jsonnet
var Builtins string
