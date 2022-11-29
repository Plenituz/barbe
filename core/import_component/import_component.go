package import_component

import (
	"barbe/core"
	"context"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const bagName = "barbe_import_component"

type ComponentImporter struct {
	alreadyImported map[string][]core.ConfigContainer
}

func NewComponentImporter() *ComponentImporter {
	return &ComponentImporter{
		alreadyImported: map[string][]core.ConfigContainer{},
	}
}

func (t *ComponentImporter) Name() string {
	return "component_importer"
}

func (t *ComponentImporter) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	output := core.NewConcurrentConfigContainer()
	maker := ctx.Value("maker").(*core.Maker)
	eg := errgroup.Group{}
	eg.SetLimit(50)
	for resourceType, m := range data.DataBags {
		if resourceType != bagName {
			continue
		}
		for _, group := range m {
		LOOP:
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}

				input := core.NewConfigContainer()
				inputTokens := core.GetObjectKeyValues("input", databag.Value.ObjectConst)
				for _, inputToken := range inputTokens {
					if inputToken.Type != core.TokenTypeObjectConst {
						continue
					}
					for _, typePair := range inputToken.ObjectConst {
						if typePair.Value.Type != core.TokenTypeObjectConst {
							continue
						}
						for _, namePair := range typePair.Value.ObjectConst {
							if namePair.Value.Type != core.TokenTypeArrayConst {
								continue
							}
							for _, databagToken := range namePair.Value.ArrayConst {
								if databagToken.Type != core.TokenTypeObjectConst {
									continue
								}
								value := core.GetObjectKeyValues("Value", databagToken.ObjectConst)
								if len(value) == 0 {
									continue
								}
								obj, _ := core.TokenToGoValue(value[0])
								if core.InterfaceIsNil(obj) {
									continue
								}

								var token core.SyntaxToken
								err := mapstructure.Decode(obj, &token)
								if err != nil {
									return core.ConfigContainer{}, errors.Wrap(err, "error decoding input")
								}

								labels := make([]string, 0)
								labelsTokens := core.GetObjectKeyValues("Labels", databagToken.ObjectConst)
								for _, labelsToken := range labelsTokens {
									if labelsToken.Type != core.TokenTypeArrayConst {
										continue
									}
									for _, labelToken := range labelsToken.ArrayConst {
										str, err := core.ExtractAsStringValue(labelToken)
										if err != nil {
											continue
										}
										labels = append(labels, str)
									}
								}
								bag := core.DataBag{
									Name:   namePair.Key,
									Type:   typePair.Key,
									Labels: labels,
									Value:  value[0],
								}
								err = input.Insert(bag)
								if err != nil {
									return core.ConfigContainer{}, errors.Wrap(err, "error inserting databag")
								}
							}
						}
					}
				}

				if pastBags, ok := t.alreadyImported[databag.Name]; ok {
					for _, pastBag := range pastBags {
						if core.ConfigContainerDeepEqual(pastBag, *input) {
							continue LOOP
						}
					}
				}
				if _, ok := t.alreadyImported[databag.Name]; !ok {
					t.alreadyImported[databag.Name] = []core.ConfigContainer{}
				}
				t.alreadyImported[databag.Name] = append(t.alreadyImported[databag.Name], *input.Clone())

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

				eg.Go(func() error {
					file, err := maker.Fetcher.Fetch(componentUrl)
					if err != nil {
						return errors.Wrap(err, "error fetching component")
					}

					newBags, err := maker.ApplyComponent(ctx, file, *input)
					if err != nil {
						return errors.Wrap(err, "error applying component '"+componentUrl+"'")
					}
					if newBags.IsEmpty() {
						return nil
					}
					err = output.MergeWith(newBags)
					if err != nil {
						return errors.Wrap(err, "error merging databags")
					}
					return nil
				})
			}
		}
	}

	err := eg.Wait()
	if err != nil {
		return core.ConfigContainer{}, err
	}

	return *output.Container(), nil
}
