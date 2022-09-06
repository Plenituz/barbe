package cue_templater

import (
	"barbe/core"
	"context"
	"embed"
)

type CueTemplater struct{}

func (h CueTemplater) Name() string {
	return "cue_templater"
}

func (h CueTemplater) Apply(ctx context.Context, container *core.ConfigContainer, templates []core.FileDescription) error {
	return applyTemplate(ctx, container, templates)
}

//go:embed barbe/*.cue
var Builtins embed.FS
