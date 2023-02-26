package import_component

import (
	"barbe/core"
	"context"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"os"
	"sync"
)

const bagName = "barbe_import_component"

type ComponentImporter struct {
	mutex           sync.Mutex
	alreadyImported map[string][]core.ConfigContainer
}

func NewComponentImporter() *ComponentImporter {
	return &ComponentImporter{
		mutex:           sync.Mutex{},
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
			//LOOP:
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
								obj, _ := core.TokenToGoValue(value[0], true)
								if core.InterfaceIsNil(obj) {
									continue
								}

								var token core.SyntaxToken
								err := mapstructure.Decode(obj, &token)
								if err != nil {
									return core.ConfigContainer{}, errors.Wrap(err, "error decoding input")
								}

								labels := core.GetMetaComplexType[[]string](databagToken, "Labels")
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

				//TODO this is kind of getting ignored in the spidermonkey js templater
				//since it creates it's own import_component for each request (on purpose)
				//executeId := core.ContextScopeKey(ctx) + databag.Name
				//t.mutex.Lock()
				//if pastBags, ok := t.alreadyImported[executeId]; ok {
				//	for _, pastBag := range pastBags {
				//		if core.ConfigContainerDeepEqual(pastBag, *input) {
				//			t.mutex.Unlock()
				//			continue LOOP
				//		}
				//	}
				//}
				//if _, ok := t.alreadyImported[executeId]; !ok {
				//	t.alreadyImported[executeId] = []core.ConfigContainer{}
				//}
				//t.alreadyImported[executeId] = append(t.alreadyImported[databag.Name], *input.Clone())
				//t.mutex.Unlock()

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

					if os.Getenv("BARBE_VERBOSE") == "1" {
						log.Ctx(ctx).Debug().Msgf("importing component '%s'", file.Name)
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
