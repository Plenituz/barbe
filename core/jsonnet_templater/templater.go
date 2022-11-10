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
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"os"
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
	Value  interface{}
}

type parsedPipelineResult struct {
	Pipelines parsedContainer
}

func createVm(ctx context.Context, container *core.ConfigContainer) (*jsonnet.VM, error) {
	env, err := envMap()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal env map")
	}
	vm := jsonnet.MakeVM()
	err = populateContainerInVm(vm, container)
	if err != nil {
		return nil, errors.Wrap(err, "failed to populate container in vm")
	}
	vm.ExtCode("barbe", Builtins)
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
	return vm, nil
}

func populateContainerInVm(vm *jsonnet.VM, container *core.ConfigContainer) error {
	ctxObjJson, err := json.Marshal(container.DataBags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal context object")
	}
	vm.ExtCode("container", string(ctxObjJson))
	return nil
}

func executeJsonnet(ctx context.Context, container *core.ConfigContainer, templateFile fetcher.FileDescription) error {
	vm, err := createVm(ctx, container)
	if err != nil {
		return errors.Wrap(err, "failed to create vm")
	}
	node, err := jsonnet.SnippetToAST(templateFile.Name, string(templateFile.Content))
	if err != nil {
		return errors.Wrap(err, "failed to parse jsonnet template")
	}
	jsonStr, err := vm.Evaluate(node)
	if err != nil {
		return formatJsonnetError(ctx, templateFile.Name, err)
	}

	err = processExecutionResult(ctx, vm, container, node, jsonStr)
	if err != nil {
		return errors.Wrap(err, "error processing execution result")
	}
	return nil
}

func processExecutionResult(ctx context.Context, vm *jsonnet.VM, container *core.ConfigContainer, template ast.Node, jsonStrOrParsed interface{}) error {
	var c parsedContainer
	if jsonStr, ok := jsonStrOrParsed.(string); ok {
		err := json.Unmarshal([]byte(jsonStr), &c)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal jsonnet output")
		}
	} else if parsed, ok := jsonStrOrParsed.(parsedContainer); ok {
		c = parsed
	} else {
		return errors.New("unexpected jsonnet output type: " + fmt.Sprintf("%T", jsonStrOrParsed))
	}

	for _, v := range c.Databags {
		token, err := core.DecodeValue(v.Value)
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
		err = container.Insert(bag)
		if err != nil {
			return errors.Wrap(err, "error merging databag on jsonnet template")
		}
	}
	if len(c.Pipelines) > 0 {
		err := runPipelines(ctx, vm, container, c.Pipelines, template)
		if err != nil {
			return errors.Wrap(err, "error running pipelines")
		}
	}
	return nil
}

func runPipelines(ctx context.Context, vm *jsonnet.VM, container *core.ConfigContainer, pipelines []int, template ast.Node) error {
	maker := ctx.Value("maker").(*core.Maker)
	for pipelineIndex, pipelineLength := range pipelines {
		for stepIndex := 0; stepIndex < pipelineLength; stepIndex++ {
			err := populateContainerInVm(vm, container)
			if err != nil {
				return errors.Wrap(err, "failed to populate container in vm")
			}
			vm.ExtVar("barbe_selected_pipeline", fmt.Sprintf("%d", pipelineIndex))
			vm.ExtVar("barbe_selected_pipeline_step", fmt.Sprintf("%d", stepIndex))
			jsonStr, err := vm.Evaluate(template)
			if err != nil {
				return formatJsonnetError(ctx, fmt.Sprintf("pipeline[%d][%d]", pipelineIndex, stepIndex), err)
			}

			var parsedResult parsedPipelineResult
			err = json.Unmarshal([]byte(jsonStr), &parsedResult)
			if err != nil {
				return errors.Wrap(err, "failed to unmarshal jsonnet output")
			}

			err = processExecutionResult(ctx, vm, container, template, parsedResult.Pipelines)
			if err != nil {
				return errors.Wrap(err, "error processing execution result")
			}

			err = maker.Transform(ctx, container, fmt.Sprintf("pipeline[%d][%d]", pipelineIndex, stepIndex))
			if err != nil {
				return errors.Wrap(err, "error transforming container in pipeline")
			}
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
