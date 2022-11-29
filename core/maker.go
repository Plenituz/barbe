package core

import (
	"barbe/core/fetcher"
	"barbe/core/state_display"
	"context"
	"github.com/lightstep/lightstep-tracer-go"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"time"
)

type MakeCommand = string

const (
	MakeCommandGenerate = "generate"
	MakeCommandApply    = "apply"
	MakeCommandDestroy  = "destroy"
)

type Maker struct {
	Command      MakeCommand
	OutputDir    string
	Parsers      []Parser
	Templaters   []TemplateEngine
	Transformers []Transformer
	Formatters   []Formatter

	Fetcher      *fetcher.Fetcher
	StateHandler *StateHandler
	//we keep this here so it can be modified on the fly if needed
	Executable Executable
}

func NewMaker(command MakeCommand) *Maker {
	maker := &Maker{
		Command: command,
		Fetcher: fetcher.NewFetcher(),
	}
	stateHandler := NewStateHandler(maker)
	//we always add a memory persister in case some templates rely on the state "API" to pass values between steps
	err := stateHandler.AddPersister(NewMemoryStatePersister())
	if err != nil {
		panic(err)
	}
	maker.StateHandler = stateHandler
	return maker
}

func (maker *Maker) Make(ctx context.Context, inputFiles []fetcher.FileDescription) (*ConfigContainer, error) {
	if os.Getenv("BARBE_TRACE") != "" {
		//f, err := os.Create(path.Join(maker.OutputDir, "trace.out"))
		//if err != nil {
		//	return nil, errors.Wrap(err, "error creating trace file")
		//}
		//defer f.Close()
		//err = trace.Start(f)
		//if err != nil {
		//	return nil, errors.Wrap(err, "error starting trace")
		//}
		//defer trace.Stop()

		lightstepTracer := lightstep.NewTracer(lightstep.Options{
			AccessToken:    os.Getenv("BARBE_TRACE"),
			MaxLogValueLen: 50000,
		})

		opentracing.SetGlobalTracer(lightstepTracer)
		defer lightstepTracer.Close(ctx)
		defer lightstepTracer.Flush(ctx)

		span := opentracing.GlobalTracer().StartSpan("Make")
		span.LogKV("command", maker.Command)
		defer span.Finish()
		ctx = opentracing.ContextWithSpan(ctx, span)
	}

	desiredCommand := maker.Command
	container := NewConfigContainer()
	err := maker.ParseFiles(ctx, inputFiles, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing input files")
	}

	t := time.Now()
	state_display.StartMajorStep("Fetch templates")
	executable, err := maker.GetTemplates(ctx, container)
	state_display.EndMajorStep("Fetch templates")
	log.Ctx(ctx).Debug().Msgf("getting templates took: %s", time.Since(t))
	if err != nil {
		return container, errors.Wrap(err, "error getting templates")
	}
	maker.Executable = executable

	if executable.Message != "" {
		log.Ctx(ctx).Info().Msg(executable.Message)
	}

	err = maker.ParseFiles(ctx, executable.Files, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing files from manifest")
	}

	state_display.StartMajorStep("Pre-transform")
	err = maker.TransformInPlace(ctx, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Pre-transform")

	state_display.StartMajorStep("Applying components for " + MakeCommandGenerate)
	maker.Command = MakeCommandGenerate
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Applying components " + MakeCommandGenerate)

	state_display.StartMajorStep("Formatters")
	for _, formatter := range maker.Formatters {
		log.Ctx(ctx).Debug().Msgf("formatting %s", formatter.Name())
		err := formatter.Format(ctx, *container)
		if err != nil {
			return container, err
		}
	}
	state_display.EndMajorStep("Formatters")
	if desiredCommand == MakeCommandGenerate {
		return container, nil
	}

	state_display.StartMajorStep("Applying components for " + desiredCommand)
	maker.Command = desiredCommand
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Applying components for " + desiredCommand)

	return container, nil
}

func (maker *Maker) ParseFiles(ctx context.Context, files []fetcher.FileDescription, container *ConfigContainer) error {
	for _, file := range files {
		for _, parser := range maker.Parsers {
			canParse, err := parser.CanParse(ctx, file)
			if err != nil {
				return err
			}
			if !canParse {
				continue
			}
			log.Ctx(ctx).Debug().Msgf("parsing '%s' with '%s'", file.Name, parser.Name())
			err = parser.Parse(ctx, file, container)
			if err != nil {
				return err
			}
		}
	}
	err := maker.StateHandler.HandleStateDatabags(ctx, container)
	if err != nil {
		return errors.Wrap(err, "error creating persisters")
	}
	return nil
}

//Transform returns the new or modified databags produced by the transformers
func (maker *Maker) Transform(ctx context.Context, container ConfigContainer) (newOrModifiedBags ConfigContainer, e error) {
	err := maker.StateHandler.HandleStateDatabags(ctx, &container)
	if err != nil {
		return ConfigContainer{}, errors.Wrap(err, "error creating persisters")
	}
	output := NewConfigContainer()
	for _, transformer := range maker.Transformers {
		//log.Ctx(ctx).Debug().Msgf("applying transformer '%s'", transformer.Name())
		//t := time.Now()
		newBags, err := transformer.Transform(ctx, container)
		//log.Ctx(ctx).Debug().Msgf("transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return ConfigContainer{}, err
		}
		err = output.MergeWith(newBags)
		if err != nil {
			return ConfigContainer{}, err
		}
	}
	return *output, nil
}

//TransformInPlace applied the transformers and merge the databags they produce into the given container directly
func (maker *Maker) TransformInPlace(ctx context.Context, container *ConfigContainer) error {
	err := maker.StateHandler.HandleStateDatabags(ctx, container)
	if err != nil {
		return errors.Wrap(err, "error creating persisters")
	}
	for _, transformer := range maker.Transformers {
		//log.Ctx(ctx).Debug().Msgf("applying transformer '%s'", transformer.Name())
		//t := time.Now()
		newBags, err := transformer.Transform(ctx, *container)
		//log.Ctx(ctx).Debug().Msgf("transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return err
		}
		err = container.MergeWith(newBags)
		if err != nil {
			return err
		}
	}
	return nil
}
