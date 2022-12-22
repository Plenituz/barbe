package jsonnet_templater

import (
	"barbe/core"
	"barbe/core/fetcher"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

//https://jsonnet.org/ref/stdlib.html
//https://jsonnet.org/ref/language.html
type parsedContainer struct {
	Databags  []sugarBag
	Pipelines []int
}
type sugarBag struct {
	Name   string
	Type   string
	Labels []string
	Meta   map[string]interface{}
	Value  interface{}
}

type parsedPipelineResult struct {
	Pipelines parsedPipelineItem
}

type parsedPipelineItem struct {
	Databags []sugarBag
}

func createVm(ctx context.Context, maker *core.Maker, input core.ConfigContainer) (*jsonnet.VM, io.Closer, error) {
	env, err := envMap()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to marshal env map")
	}
	vm := jsonnet.MakeVM()
	err = populateStateAndContainerInVm(maker, vm, input)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to populate container in vm")
	}
	vm.MaxStack = 100000
	vm.ExtCode("barbe", Builtins)
	vm.ExtVar("barbe_command", maker.Command)
	vm.ExtVar("barbe_lifecycle_step", maker.CurrentStep)
	vm.ExtVar("barbe_output_dir", ctx.Value("maker").(*core.Maker).OutputDir)
	vm.ExtCode("env", env)
	vm.ExtVar("barbe_selected_pipeline", "")
	vm.ExtVar("barbe_selected_pipeline_step", "")
	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "regexFindAllSubmatch",
		Params: ast.Identifiers{"pattern", "input"},
		Func: func(x []interface{}) (interface{}, error) {
			pattern, ok := x[0].(string)
			if !ok {
				return nil, errors.New("first argument must be a string")
			}
			input, ok := x[1].(string)
			if !ok {
				return nil, errors.New("second argument must be a string")
			}

			expr, err := regexp.Compile(pattern)
			if err != nil {
				return nil, errors.Wrap(err, "failed to compile regex")
			}
			matches := expr.FindAllStringSubmatch(input, -1)

			var result []interface{}
			for _, m := range matches {
				var r []interface{}
				for _, s := range m {
					r = append(r, s)
				}
				result = append(result, r)
			}
			return result, nil
		},
	})

	traceReader, traceWriter := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(traceReader)
		for scanner.Scan() {
			log.Ctx(ctx).Debug().Msg(scanner.Text())
		}
	}()
	vm.SetTraceOut(traceWriter)
	return vm, traceWriter, nil
}

func populateStateAndContainerInVm(maker *core.Maker, vm *jsonnet.VM, container core.ConfigContainer) error {
	ctxObjJson, err := json.Marshal(container.DataBags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal context object")
	}
	vm.ExtCode("container", string(ctxObjJson))

	state := maker.StateHandler.GetState()
	stateJson, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "failed to marshal state object")
	}
	vm.ExtCode("state", string(stateJson))
	return nil
}

func executeJsonnet(ctx context.Context, maker *core.Maker, input core.ConfigContainer, output *core.ConfigContainer, templateFile fetcher.FileDescription) error {
	vm, closer, err := createVm(ctx, maker, input)
	if err != nil {
		return errors.Wrap(err, "failed to create vm")
	}
	defer closer.Close()
	node, err := jsonnet.SnippetToAST(templateFile.Name, string(templateFile.Content))
	if err != nil {
		return errors.Wrap(err, "failed to parse jsonnet template")
	}
	jsonStr, err := vm.Evaluate(node)
	if err != nil {
		return formatJsonnetError(ctx, templateFile.Name, err)
	}

	var c parsedContainer
	err = json.Unmarshal([]byte(jsonStr), &c)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal jsonnet output")
	}
	err = insertDatabags(c.Databags, output)
	if err != nil {
		return errors.Wrap(err, "failed to insert databags")
	}

	err = maker.TransformInPlace(ctx, output)
	if err != nil {
		return errors.Wrap(err, "error transforming container in pipeline")
	}

	if len(c.Pipelines) > 0 {
		for pipelineIndex, pipelineLength := range c.Pipelines {
			for stepIndex := 0; stepIndex < pipelineLength; stepIndex++ {
				err = func() error {
					stepInput := input.Clone()
					err = stepInput.MergeWith(*output)
					if err != nil {
						return errors.Wrap(err, "failed to merge input with container")
					}
					log.Ctx(ctx).Debug().Msgf("executing '%s.%s' pipeline[%d][%d] (%d keys in input)", templateFile.Name, maker.CurrentStep, pipelineIndex, stepIndex, len(stepInput.DataBags))

					//var traceCtx context.Context
					//var task *trace.Task
					var span opentracing.Span
					if os.Getenv("BARBE_TRACE") != "" {
						b, err := json.Marshal(stepInput)
						if err != nil {
							return errors.Wrap(err, "failed to marshal input for trace")
						}
						span = opentracing.GlobalTracer().StartSpan(fmt.Sprintf("%s.pipeline[%d][%d]", path.Base(templateFile.Name), pipelineIndex, stepIndex), opentracing.ChildOf(opentracing.SpanFromContext(ctx).Context()))
						span.LogKV("command", maker.CurrentStep)
						span.LogKV("input", string(b))
						defer span.Finish()
						ctx = opentracing.ContextWithSpan(ctx, span)
						//traceCtx, task = trace.NewTask(ctx, fmt.Sprintf("%s.pipeline[%d][%d]", path.Base(templateFile.Name), pipelineIndex, stepIndex))
						//defer task.End()
						//b, err := json.Marshal(stepInput)
						//if err != nil {
						//	return errors.Wrap(err, "failed to marshal input for trace")
						//}
						//trace.Log(traceCtx, "input", string(b))
						//trace.Log(traceCtx, "command", ctx.Value("maker").(*core.Maker).CurrentStep)
					}

					err = populateStateAndContainerInVm(maker, vm, *stepInput)
					if err != nil {
						return errors.Wrap(err, "failed to populate container in vm")
					}
					vm.ExtVar("barbe_selected_pipeline", fmt.Sprintf("%d", pipelineIndex))
					vm.ExtVar("barbe_selected_pipeline_step", fmt.Sprintf("%d", stepIndex))
					jsonStr, err := vm.Evaluate(node)
					if err != nil {
						return formatJsonnetError(ctx, fmt.Sprintf("%s.pipeline[%d][%d]", templateFile.Name, pipelineIndex, stepIndex), err)
					}

					var parsedResult parsedPipelineResult
					err = json.Unmarshal([]byte(jsonStr), &parsedResult)
					if err != nil {
						return errors.Wrap(err, "failed to unmarshal jsonnet output")
					}
					log.Ctx(ctx).Debug().Msgf("'%s.%s' pipeline[%d][%d] created %d keys", templateFile.Name, maker.CurrentStep, pipelineIndex, stepIndex, len(parsedResult.Pipelines.Databags))

					//transform stepInput + parsedResult.Pipelines.Databags
					//add the result of transformation + parsedResult.Pipelines.Databags to output

					toTransform := stepInput.Clone()
					err = insertDatabags(parsedResult.Pipelines.Databags, toTransform)
					if err != nil {
						return errors.Wrap(err, "failed to insert databags")
					}
					newFromTransform, err := maker.Transform(ctx, *toTransform)
					if err != nil {
						return errors.Wrap(err, "error transforming container in pipeline")
					}

					err = insertDatabags(parsedResult.Pipelines.Databags, output)
					if err != nil {
						return errors.Wrap(err, "failed to insert databags")
					}
					err = output.MergeWith(newFromTransform)
					if err != nil {
						return errors.Wrap(err, "failed to merge container")
					}

					if os.Getenv("BARBE_TRACE") != "" {
						b, err := json.Marshal(output)
						if err != nil {
							return errors.Wrap(err, "failed to marshal output for trace")
						}
						span.LogKV("output", string(b))
						//trace.Log(traceCtx, "output", string(b))
					}

					return nil
				}()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func insertDatabags(newBags []sugarBag, output *core.ConfigContainer) error {
	for _, v := range newBags {
		if v.Name == "" && v.Type == "" {
			continue
		}
		token, err := core.GoValueToToken(v.Value)
		if err != nil {
			return errors.Wrap(err, "error decoding syntax token from jsonnet template")
		}

		if v.Labels == nil {
			v.Labels = []string{}
		}
		bag := core.DataBag{
			Name:   v.Name,
			Type:   v.Type,
			Labels: v.Labels,
			Value:  token,
		}
		err = output.Insert(bag)
		if err != nil {
			return errors.Wrap(err, "error merging databag on jsonnet template")
		}
	}
	return nil
}

func formatJsonnetError(ctx context.Context, templateFileName string, err error) error {
	log.Ctx(ctx).Debug().Msg(err.Error())
	if strings.Contains(err.Error(), "<showuser>") {
		msg := strings.Split(strings.Split(err.Error(), "<showuser>")[1], "</showuser>")[0]
		return errors.New(msg)
	}
	err = errors.New(strings.ReplaceAll(err.Error(), "<extvar:barbe>", "<extvar:barbe> utils.jsonnet"))
	return errors.Wrap(err, "failed to evaluate '"+templateFileName+"'")
}

func envMap() (string, error) {
	//TODO this may not work as well on windows, see https://github.com/caarlos0/env/blob/main/env_windows.go
	r := map[string]string{}
	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		r[p[0]] = p[1]
	}
	str, err := json.Marshal(r)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal env map")
	}
	return string(str), nil
}
