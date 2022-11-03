package jsonnet_templater

import (
	"barbe/core"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
)

//https://jsonnet.org/ref/stdlib.html
//https://jsonnet.org/ref/language.html
type parsedContainer struct {
	Databags  []sugarBag
	Pipelines [][]string
}
type sugarBag struct {
	Name   string
	Type   string
	Labels []string
	Value  interface{}
}

/*
{
    pipeline: [
        barbe.step(function(container) [

        ]),
        barbe.step(function(container) [

        ])
    ]
}

- parse ast
- find barbe.step calls, store the content ast in a cache
- replace the ast given to the VM by the cache id
- when the vm runs, it therefore will return an array with the cache ids in order
- create sub vms for each pipelines based on the ids and stored ast
*/

type visitor = func(token ast.Node) (ast.Node, error)

func visitJsonnetAst(ctx context.Context, root ast.Node, visitor visitor) (ast.Node, error) {
	if core.InterfaceIsNil(root) {
		return root, nil
	}
	modifiedToken, err := visitor(root)
	if err != nil {
		return root, err
	}
	if !core.InterfaceIsNil(modifiedToken) {
		return modifiedToken, nil
	}
	switch root := root.(type) {
	case *ast.Array:
		for i, item := range root.Elements {
			v, err := visitJsonnetAst(ctx, item.Expr, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting array item with index '"+fmt.Sprintf("%v", i)+"'")
			}
			item.Expr = v
			root.Elements[i] = item
		}
		return root, nil

	case *ast.Binary:
		root.Right, err = visitJsonnetAst(ctx, root.Right, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting binary op right hand side")
		}
		root.Left, err = visitJsonnetAst(ctx, root.Left, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting binary op left hand side")
		}
		return root, nil

	case *ast.Unary:
		root.Expr, err = visitJsonnetAst(ctx, root.Expr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting unary op")
		}
		return root, nil

	case *ast.Conditional:
		root.Cond, err = visitJsonnetAst(ctx, root.Cond, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting conditional condition")
		}
		root.BranchTrue, err = visitJsonnetAst(ctx, root.BranchTrue, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting conditional true result")
		}
		root.BranchFalse, err = visitJsonnetAst(ctx, root.BranchFalse, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting conditional false result")
		}
		return root, nil

	case *ast.DesugaredObject:
		for i, item := range root.Asserts {
			v, err := visitJsonnetAst(ctx, item, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting desugared object assert with index '"+fmt.Sprintf("%v", i)+"'")
			}
			root.Asserts[i] = v
		}
		for i, pair := range root.Fields {
			name, err := visitJsonnetAst(ctx, pair.Name, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting field key name")
			}
			body, err := visitJsonnetAst(ctx, pair.Body, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting field body")
			}
			pair.Name = name
			pair.Body = body
			root.Fields[i] = pair
		}
		return root, nil

	case *ast.Error:
		root.Expr, err = visitJsonnetAst(ctx, root.Expr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting error token")
		}
		return root, nil

	case *ast.Index:
		root.Target, err = visitJsonnetAst(ctx, root.Target, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting index target")
		}
		root.Index, err = visitJsonnetAst(ctx, root.Index, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting index")
		}
		return root, nil

	case *ast.Import, *ast.ImportStr,
		*ast.LiteralBoolean, *ast.LiteralNull,
		*ast.LiteralNumber, *ast.LiteralString,
		*ast.Self, *ast.Var:
		return root, nil

	case *ast.Local:
		for i, bind := range root.Binds {
			body, err := visitJsonnetAst(ctx, bind.Body, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting local bind body with index '"+fmt.Sprintf("%v", i)+"'")
			}
			fun, err := visitJsonnetAst(ctx, bind.Fun, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting local bind function with index '"+fmt.Sprintf("%v", i)+"'")
			}
			bind.Body = body
			bind.Fun = fun.(*ast.Function)
			root.Binds[i] = bind
		}
		root.Body, err = visitJsonnetAst(ctx, root.Body, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting local body")
		}
		return root, nil

	case *ast.SuperIndex:
		root.Index, err = visitJsonnetAst(ctx, root.Index, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting super index")
		}
		return root, nil

	case *ast.InSuper:
		root.Index, err = visitJsonnetAst(ctx, root.Index, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting super index")
		}
		return root, nil

	case *ast.Function:
		for i, param := range root.Parameters {
			defaultArg, err := visitJsonnetAst(ctx, param.DefaultArg, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting function parameter with index '"+fmt.Sprintf("%v", i)+"'")
			}
			param.DefaultArg = defaultArg
			root.Parameters[i] = param
		}
		root.Body, err = visitJsonnetAst(ctx, root.Body, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting function body")
		}
		return root, nil

	case *ast.Apply:
		root.Target, err = visitJsonnetAst(ctx, root.Target, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error visiting apply target")
		}
		for i, arg := range root.Arguments.Positional {
			arg.Expr, err = visitJsonnetAst(ctx, arg.Expr, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting apply argument with index '"+fmt.Sprintf("%v", i)+"'")
			}
			root.Arguments.Positional[i] = arg
		}
		for i, arg := range root.Arguments.Named {
			arg.Arg, err = visitJsonnetAst(ctx, arg.Arg, visitor)
			if err != nil {
				return root, errors.Wrap(err, "error visiting apply argument with index '"+fmt.Sprintf("%v", i)+"'")
			}
			root.Arguments.Named[i] = arg
		}
		return root, nil

	default:
		return nil, errors.New(fmt.Sprintf("Unknown node type '%v'", reflect.TypeOf(root)))
	}
}

func applyTemplate(ctx context.Context, container *core.ConfigContainer, templates []core.FileDescription) error {
	vm := jsonnet.MakeVM()
	procedureCache := map[string]*ast.Function{}

	ctxObjJson, err := json.Marshal(container.DataBags)
	if err != nil {
		return errors.Wrap(err, "failed to marshal context object")
	}

	env, err := envMap()
	if err != nil {
		return errors.Wrap(err, "failed to marshal env map")
	}
	vm.ExtCode("container", string(ctxObjJson))
	vm.ExtCode("barbe", Builtins)
	vm.ExtVar("barbe_output_dir", ctx.Value("maker").(*core.Maker).OutputDir)
	vm.ExtCode("env", env)
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

	results := make([]string, 0)
	for _, templateFile := range templates {
		if path.Ext(templateFile.Name) != ".jsonnet" {
			continue
		}
		//TODO parallelize
		node, err := jsonnet.SnippetToAST(templateFile.Name, string(templateFile.Content))
		if err != nil {
			return errors.Wrap(err, "failed to parse jsonnet template")
		}
		node, err = extractProcedures(ctx, node, procedureCache)
		if err != nil {
			return errors.Wrap(err, "failed to transform jsonnet template")
		}
		jsonStr, err := vm.Evaluate(node)
		if err != nil {
			return formatJsonnetError(ctx, templateFile.Name, err)
		}
		results = append(results, jsonStr)
	}

	parsedContainers := make([]parsedContainer, 0, len(results))
	for _, value := range results {
		var m parsedContainer
		err = json.Unmarshal([]byte(value), &m)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msg("while decoding jsonnet value")
		}
		parsedContainers = append(parsedContainers, m)
	}

	for _, c := range parsedContainers {
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

		for _, pipeline := range c.Pipelines {
			for _, procedureId := range pipeline {
				procedure, ok := procedureCache[procedureId]
				if !ok {
					return errors.New("procedure '" + procedureId + "' not found")
				}
				callBootstrap, err := jsonnet.SnippetToAST("boostrap", "std.tmp(std.extVar(\"container\"))")
				if err != nil {
					return errors.Wrap(err, "failed to parse jsonnet template")
				}
				funcCall := callBootstrap.(*ast.Apply)
				funcCall.Target = procedure

				jsonStr, err := vm.Evaluate(funcCall)
				if err != nil {
					return formatJsonnetError(ctx, "procedure:"+procedureId, err)
				}
				fmt.Println(jsonStr)

			}
		}
	}
	return nil
}

func extractProcedures(ctx context.Context, node ast.Node, procedureCache map[string]*ast.Function) (ast.Node, error) {
	node, err := visitJsonnetAst(ctx, node, func(token ast.Node) (ast.Node, error) {
		if funcCall, ok := token.(*ast.Apply); ok {
			if ind, ok := funcCall.Target.(*ast.Index); ok {
				if varName, ok := ind.Target.(*ast.Var); ok {
					if varName.Id == "barbe" {
						if ind.Index.(*ast.LiteralString).Value == "procedure" {
							if len(funcCall.Arguments.Positional) == 1 {
								//the first argument is a function
								if funcArg, ok := funcCall.Arguments.Positional[0].Expr.(*ast.Function); ok {
									id := uuid.NewString()
									procedureCache[id] = funcArg
									return &ast.LiteralString{Value: id}, nil
								}
							}
						}
					}
				}
			}
		}
		return nil, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to visit jsonnet ast")
	}
	return node, nil
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
