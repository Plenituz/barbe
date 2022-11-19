package core

import (
	"barbe/core/fetcher"
	"barbe/core/state_display"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"time"
)

type MakeCommand = string

const (
	MakeCommandGenerate = "generate"
	MakeCommandApply    = "apply"
	MakeCommandDestroy  = "destroy"
)

type Maker struct {
	Command         MakeCommand
	OutputDir       string
	Parsers         []Parser
	PreTransformers []Transformer
	Templaters      []TemplateEngine
	Transformers    []Transformer
	Formatters      []Formatter

	Fetcher      *fetcher.Fetcher
	stateHandler *StateHandler
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
	maker.stateHandler = stateHandler
	return maker
}

func (maker *Maker) Make(ctx context.Context, inputFiles []fetcher.FileDescription) (*ConfigContainer, error) {
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

	if executable.Message != "" {
		log.Ctx(ctx).Info().Msg(executable.Message)
	}

	err = maker.ParseFiles(ctx, executable.Files, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing files from manifest")
	}

	state_display.StartMajorStep("Pre-transform")
	err = maker.PreTransform(ctx, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Pre-transform")

	state_display.StartMajorStep("Applying components for " + MakeCommandGenerate)
	maker.Command = MakeCommandGenerate
	err = maker.ApplyComponents(ctx, executable, container)
	if err != nil {
		return container, err
	}
	state_display.EndMajorStep("Applying components " + MakeCommandGenerate)

	state_display.StartMajorStep("Formatters")
	for _, formatter := range maker.Formatters {
		log.Ctx(ctx).Debug().Msgf("formatting %s", formatter.Name())
		err := formatter.Format(ctx, container)
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
	err = maker.ApplyComponents(ctx, executable, container)
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
	err := maker.stateHandler.HandleStateDatabags(ctx, container)
	if err != nil {
		return errors.Wrap(err, "error creating persisters")
	}
	return nil
}

func (maker *Maker) PreTransform(ctx context.Context, container *ConfigContainer) error {
	for _, transformer := range maker.PreTransformers {
		log.Ctx(ctx).Debug().Msgf("applying pre-transformer '%s'", transformer.Name())
		t := time.Now()
		err := transformer.Transform(ctx, container)
		log.Ctx(ctx).Debug().Msgf("pre-transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return err
		}
	}
	err := maker.stateHandler.HandleStateDatabags(ctx, container)
	if err != nil {
		return errors.Wrap(err, "error creating persisters")
	}
	return nil
}

func (maker *Maker) Transform(ctx context.Context, container *ConfigContainer) error {
	for _, transformer := range maker.Transformers {
		//log.Ctx(ctx).Debug().Msgf("applying transformer '%s'", transformer.Name())
		//t := time.Now()
		err := transformer.Transform(ctx, container)
		//log.Ctx(ctx).Debug().Msgf("transformer '%s' took: %s", transformer.Name(), time.Since(t))
		if err != nil {
			return err
		}
	}
	err := maker.stateHandler.HandleStateDatabags(ctx, container)
	if err != nil {
		return errors.Wrap(err, "error creating persisters")
	}
	return nil
}
