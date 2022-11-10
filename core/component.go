package core

import (
	"barbe/core/fetcher"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"strings"
	"time"
)

func (maker *Maker) ApplyComponents(ctx context.Context, executable Executable, container *ConfigContainer) error {
	componentInput := container
	for i := 0; i < maxComponentLoops; i++ {
		log.Ctx(ctx).Debug().Msgf("applying components, loop %d", i)
		eg := errgroup.Group{}
		eg.SetLimit(50)
		newDatabags := NewConcurrentConfigContainer()
		for i := range executable.Components {
			component := executable.Components[i]
			eg.Go(func() error {
				input := componentInput.Clone()
				output, err := maker.ApplyComponent(ctx, component, *input)
				if err != nil {
					return err
				}
				if output.IsEmpty() {
					return nil
				}
				err = newDatabags.MergeWith(output)
				if err != nil {
					return errors.Wrap(err, "error merging databags")
				}
				return nil
			})
		}
		err := eg.Wait()
		if err != nil {
			return err
		}

		componentInput = newDatabags.Container()
		err = maker.Transform(ctx, componentInput)
		if err != nil {
			return errors.Wrap(err, "error transforming container in pipeline")
		}

		//remove databags that are in componentInput and already in container, to avoid looping forever
		for typeName, databags := range componentInput.DataBags {
			for databagName, databagGroup := range databags {
				for _, databag := range databagGroup {
					if container.Contains(databag) {
						log.Ctx(ctx).Debug().Msgf("removing databag %s.%s.%s from component input, already in container", typeName, databagName, strings.Join(databag.Labels, "."))
						componentInput.DeleteDataBag(typeName, databagName, databag.Labels)
					}
				}
			}
		}

		comparison := componentInput.Clone()
		//all the state-related databags shouldn't be compared or merged in the main container
		//because they change all the time and would result in the component loop never ending (or just looping too much)
		//we do want them on to be passed along to the next component execution tho, so the component can use them
		comparison.DeleteDataBagsOfType(StateDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateSetDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateDeleteDatabagType)
		if comparison.IsEmpty() {
			break
		}
		err = container.MergeWith(*comparison)
		if err != nil {
			return errors.Wrap(err, "error merging databags")
		}
	}
	return nil
}

func (maker *Maker) ApplyComponent(ctx context.Context, file fetcher.FileDescription, input ConfigContainer) (ConfigContainer, error) {
	output := NewConfigContainer()
	for _, engine := range maker.Templaters {
		log.Ctx(ctx).Debug().Msg("applying template engine: '" + engine.Name() + "'")
		t := time.Now()

		partialOutput, err := engine.Apply(ctx, maker, input, file)
		log.Ctx(ctx).Debug().Msgf("template engine '%s' took: %v", engine.Name(), time.Since(t))
		if err != nil {
			return ConfigContainer{}, errors.Wrap(err, "from template engine '"+engine.Name()+"'")
		}

		err = output.MergeWith(partialOutput)
		if err != nil {
			return ConfigContainer{}, errors.Wrap(err, "merging output")
		}
	}
	err := maker.Transform(ctx, output)
	if err != nil {
		return ConfigContainer{}, err
	}
	return *output, nil
}
