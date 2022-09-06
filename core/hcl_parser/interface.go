package hcl_parser

import (
	"context"
	"barbe/core"
	"strings"
)

type HclParser struct{}

func (h HclParser) Name() string {
	return "hcl_parser"
}

func (h HclParser) CanParse(ctx context.Context, fileDesc core.FileDescription) (bool, error) {
	return strings.HasSuffix(strings.ToLower(fileDesc.Name), ".hcl"), nil
}

func (h HclParser) Parse(ctx context.Context, fileDesc core.FileDescription, container *core.ConfigContainer) error {
	return parseFromTemplate(ctx, container, fileDesc)
}
