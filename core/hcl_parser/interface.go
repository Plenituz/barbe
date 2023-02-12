package hcl_parser

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
)

type HclParser struct{}

func (h HclParser) Name() string {
	return "hcl_parser"
}

func (h HclParser) CanParse(ctx context.Context, fileDesc fetcher.FileDescription) (bool, error) {
	l := fetcher.ExtractExtension(fileDesc.Name)
	return l == ".hcl" || l == ".tf", nil
}

func (h HclParser) Parse(ctx context.Context, fileDesc fetcher.FileDescription, container *core.ConfigContainer) error {
	return parseFromTemplate(ctx, container, fileDesc)
}
