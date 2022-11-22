package import_component

import (
	"barbe/core"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const bagName = "barbe_import_component"

type ComponentImporter struct {
	alreadyImported map[string]struct{}
}

func NewComponentImporter() *ComponentImporter {
	return &ComponentImporter{
		alreadyImported: map[string]struct{}{},
	}
}

func (t *ComponentImporter) Name() string {
	return "component_importer"
}

func (t *ComponentImporter) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	output := core.NewConfigContainer()
	maker := ctx.Value("maker").(*core.Maker)
	for resourceType, m := range data.DataBags {
		if resourceType != bagName {
			continue
		}

		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}
				if _, ok := t.alreadyImported[databag.Name]; ok {
					continue
				}
				t.alreadyImported[databag.Name] = struct{}{}

				componentUrlTokens := core.GetObjectKeyValues("url", databag.Value.ObjectConst)
				if len(componentUrlTokens) == 0 {
					continue
				}
				if len(componentUrlTokens) > 1 {
					log.Ctx(ctx).Warn().Msgf("multiple 'url' keys found in import_component databag '%s', using first one", databag.Name)
				}
				componentUrl, err := core.ExtractAsStringValue(componentUrlTokens[0])
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error extracting component url")
				}

				file, err := maker.Fetcher.Fetch(componentUrl)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error fetching component")
				}

				newBags, err := maker.ApplyComponent(ctx, file, data)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error applying component '"+componentUrl+"'")
				}
				if newBags.IsEmpty() {
					continue
				}
				err = output.MergeWith(newBags)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrap(err, "error merging databags")
				}
			}
		}
	}

	return *output, nil
}
