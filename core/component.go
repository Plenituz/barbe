package core

import (
	"barbe/core/fetcher"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"reflect"
)

func (maker *Maker) ApplyComponents(ctx context.Context, executable Executable, container *ConfigContainer) error {
	for i := 0; i < maxComponentLoops; i++ {
		beforeApply := container.Clone()
		log.Ctx(ctx).Debug().Msgf("master, loop %d", i)
		err := maker.applyComponentsLoop(ctx, executable, container)
		if err != nil {
			return err
		}
		comparison := container.Clone()
		filterOutExistingIdenticalDatabags(ctx, *beforeApply, comparison)
		comparison.DeleteDataBagsOfType(StateDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateSetDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateDeleteDatabagType)
		if comparison.IsEmpty() {
			break
		}
	}
	return nil
}

func (maker *Maker) applyComponentsLoop(ctx context.Context, executable Executable, container *ConfigContainer) error {
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
		//This is not necessary because maker.ApplyComponent already applied the transform?
		err = maker.TransformInPlace(ctx, componentInput)
		if err != nil {
			return errors.Wrap(err, "error transforming container in pipeline")
		}
		err = maker.stateHandler.HandleStateDatabags(ctx, componentInput)
		if err != nil {
			return errors.Wrap(err, "error creating persisters")
		}

		//remove databags that are in componentInput and already in container, to avoid looping forever
		filterOutExistingIdenticalDatabags(ctx, *container, componentInput)
		comparison := componentInput.Clone()
		//all the state-related databags shouldn't be compared or merged in the main container
		//because they change all the time and would result in the component loop never ending (or just looping too much)
		//we do want them on to be passed along to the next component execution tho, so the component can use them
		comparison.DeleteDataBagsOfType(StateDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateSetDatabagType)
		comparison.DeleteDataBagsOfType(BarbeStateDeleteDatabagType)
		if comparison.IsEmpty() {
			//transform again the whole container, and loop again if more databags were produced
			newBags, err := maker.Transform(ctx, *container)
			if err != nil {
				return errors.Wrap(err, "error transforming container in pipeline")
			}

			filterOutExistingIdenticalDatabags(ctx, *container, &newBags)
			comparison := newBags.Clone()
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
			err = componentInput.MergeWith(*comparison)
			if err != nil {
				return errors.Wrap(err, "error merging databags")
			}
			err = maker.stateHandler.HandleStateDatabags(ctx, componentInput)
			if err != nil {
				return errors.Wrap(err, "error creating persisters")
			}
		} else {
			err = container.MergeWith(*comparison)
			if err != nil {
				return errors.Wrap(err, "error merging databags")
			}
		}
	}
	return nil
}

//this removes all the bags that are in `container` from `newDatabags`
func filterOutExistingIdenticalDatabags(ctx context.Context, container ConfigContainer, newDatabags *ConfigContainer) {
	for typeName, databags := range newDatabags.DataBags {
		if typeName == StateDatabagType {
			continue
		}
		for databagName, databagGroup := range databags {
			for _, databag := range databagGroup {
				if container.Contains(databag) {
					for _, existingBag := range container.GetDataBagGroup(typeName, databagName) {
						if reflect.DeepEqual(existingBag, databag) {
							//log.Ctx(ctx).Debug().Msgf("removing databag %s.%s.%s from component input, already in container", typeName, databagName, strings.Join(databag.Labels, "."))
							newDatabags.DeleteDataBag(typeName, databagName, databag.Labels)
						}
					}
				}
			}
		}
	}
}

func (maker *Maker) ApplyComponent(ctx context.Context, file fetcher.FileDescription, input ConfigContainer) (ConfigContainer, error) {
	log.Ctx(ctx).Debug().Msg("applying component '" + file.Name + "'")
	output := NewConfigContainer()
	for _, engine := range maker.Templaters {
		//log.Ctx(ctx).Debug().Msg("applying template engine: '" + engine.Name() + "'")
		//t := time.Now()

		partialOutput, err := engine.Apply(ctx, maker, input, file)
		//log.Ctx(ctx).Debug().Msgf("template engine '%s' took: %v", engine.Name(), time.Since(t))
		if err != nil {
			return ConfigContainer{}, errors.Wrap(err, "from template engine '"+engine.Name()+"'")
		}

		err = output.MergeWith(partialOutput)
		if err != nil {
			return ConfigContainer{}, errors.Wrap(err, "merging output")
		}
	}
	err := maker.TransformInPlace(ctx, output)
	if err != nil {
		return ConfigContainer{}, err
	}
	return *output, nil
}
