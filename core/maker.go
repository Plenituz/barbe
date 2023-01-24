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
type MakeLifecycleStep = string

const (
	MakeCommandGenerate = "generate"
	MakeCommandApply    = "apply"
	MakeCommandDestroy  = "destroy"
)

/*
 Lifecycle steps:
  	1. pre_generate
	2. generate
	3. post_generate
	if command is generate, stop here

	otherwise, if command is apply or destroy
	4. pre_do

	if command is apply
	5a. pre_apply
	6a. apply
	7a. post_apply

	if command is destroy
	5b. pre_destroy
	6b. destroy
	7b. post_destroy

	8. post_do
*/
const (
	MakeLifecycleStepPreGenerate  = "pre_generate"
	MakeLifecycleStepGenerate     = MakeCommandGenerate
	MakeLifecycleStepPostGenerate = "post_generate"
	//runs before the apply or destroy step
	MakeLifecycleStepPreDo = "pre_do"
	//runs before the apply step, after the pre-do step, if the command is apply
	MakeLifecycleStepPreApply    = "pre_apply"
	MakeLifecycleStepApply       = MakeCommandApply
	MakeLifecycleStepPostApply   = "post_apply"
	MakeLifecycleStepPreDestroy  = "pre_destroy"
	MakeLifecycleStepDestroy     = MakeCommandDestroy
	MakeLifecycleStepPostDestroy = "post_destroy"
	//this runs after either post_apply or post_destroy
	MakeLifecycleStepPostDo = "post_do"
)

type Maker struct {
	Command      MakeCommand
	CurrentStep  MakeLifecycleStep
	OutputDir    string
	Parsers      []Parser
	Templaters   []TemplateEngine
	Transformers []Transformer
	Formatters   []Formatter

	Fetcher      *fetcher.Fetcher
	StateHandler *StateHandler
	Executable   Executable
	Env          map[string]string
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

func (maker *Maker) Make(ctx context.Context, inputFiles []fetcher.FileDescription) (c *ConfigContainer, e error) {
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

	maker.CurrentStep = MakeLifecycleStepPreGenerate
	container := NewConfigContainer()
	err := maker.ParseFiles(ctx, inputFiles, container)
	if err != nil {
		return container, errors.Wrap(err, "error parsing input files")
	}

	t := time.Now()
	executable, err := maker.GetTemplates(ctx, container)
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

	err = maker.TransformInPlace(ctx, container)
	if err != nil {
		return container, err
	}

	state_display.GlobalState.StartMajorStep("pre_generate")
	//this is pre_generate
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		state_display.GlobalState.EndMajorStepWith("pre_generate", true)
		return container, err
	}
	state_display.GlobalState.EndMajorStep("pre_generate")

	state_display.GlobalState.StartMajorStep("generate")
	maker.CurrentStep = MakeLifecycleStepGenerate
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		state_display.GlobalState.EndMajorStepWith("generate", true)
		return container, err
	}
	state_display.GlobalState.EndMajorStep("generate")

	state_display.GlobalState.StartMajorStep("post_generate")
	maker.CurrentStep = MakeLifecycleStepPostGenerate
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		state_display.GlobalState.EndMajorStepWith("post_generate", true)
		return container, err
	}
	state_display.GlobalState.EndMajorStep("post_generate")

	for _, formatter := range maker.Formatters {
		log.Ctx(ctx).Debug().Msgf("formatting %s", formatter.Name())
		err := formatter.Format(ctx, *container)
		if err != nil {
			return container, err
		}
	}
	if maker.Command == MakeCommandGenerate {
		return container, nil
	}

	state_display.GlobalState.StartMajorStep("pre_do")
	maker.CurrentStep = MakeLifecycleStepPreDo
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		state_display.GlobalState.EndMajorStepWith("pre_do", true)
		return container, err
	}
	state_display.GlobalState.EndMajorStep("pre_do")

	switch maker.Command {
	case MakeCommandApply:
		state_display.GlobalState.StartMajorStep("pre_apply")
		maker.CurrentStep = MakeLifecycleStepPreApply
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("pre_apply", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("pre_apply")

		state_display.GlobalState.StartMajorStep("apply")
		maker.CurrentStep = MakeLifecycleStepApply
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("apply", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("apply")

		state_display.GlobalState.StartMajorStep("post_apply")
		maker.CurrentStep = MakeLifecycleStepPostApply
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("post_apply", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("post_apply")

	case MakeCommandDestroy:
		state_display.GlobalState.StartMajorStep("pre_destroy")
		maker.CurrentStep = MakeLifecycleStepPreDestroy
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("pre_destroy", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("pre_destroy")

		state_display.GlobalState.StartMajorStep("destroy")
		maker.CurrentStep = MakeLifecycleStepDestroy
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("destroy", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("destroy")

		state_display.GlobalState.StartMajorStep("post_destroy")
		maker.CurrentStep = MakeLifecycleStepPostDestroy
		err = maker.ApplyComponents(ctx, container)
		if err != nil {
			state_display.GlobalState.EndMajorStepWith("post_destroy", true)
			return container, err
		}
		state_display.GlobalState.EndMajorStep("post_destroy")
	default:
		return container, errors.New("unknown command '" + maker.Command + "'")
	}

	state_display.GlobalState.StartMajorStep("post_do")
	maker.CurrentStep = MakeLifecycleStepPostDo
	err = maker.ApplyComponents(ctx, container)
	if err != nil {
		state_display.GlobalState.EndMajorStepWith("post_do", true)
		return container, err
	}
	state_display.GlobalState.EndMajorStep("post_do")

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
