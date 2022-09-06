package jsonnet_templater

import (
	"context"
	"encoding/json"
	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"path"
	"regexp"
	"barbe/core"
	"strings"
)

//https://jsonnet.org/ref/stdlib.html
//https://jsonnet.org/ref/language.html
type parsedContainer struct {
	Databags []sugarBag
}
type sugarBag struct {
	Name   string
	Type   string
	Labels []string
	Value  map[string]interface{}
}

func applyTemplate(ctx context.Context, container *core.ConfigContainer, templates []core.FileDescription) error {
	vm := jsonnet.MakeVM()

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

	results := make([]string, 0)
	for _, templateFile := range templates {
		if path.Ext(templateFile.Name) != ".jsonnet" {
			continue
		}
		//TODO parallelize
		jsonStr, err := vm.EvaluateAnonymousSnippet(templateFile.Name, string(templateFile.Content))
		if err != nil {
			log.Ctx(ctx).Debug().Msg(err.Error())
			if strings.Contains(err.Error(), "<showuser>") {
				msg := strings.Split(strings.Split(err.Error(), "<showuser>")[1], "</showuser>")[0]
				return errors.New(msg)
			}
			err = errors.New(strings.ReplaceAll(err.Error(), "<extvar:barbe>", "<extvar:barbe> utils.jsonnet"))
			return errors.Wrap(err, "failed to evaluate template at '"+templateFile.Name+"'")
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

	for i, c := range parsedContainers {
		for j, v := range c.Databags {
			syntaxToken := core.SyntaxToken{
				Type:        core.TokenTypeObjectConst,
				ObjectConst: make([]core.ObjectConstItem, 0, len(v.Value)),
			}
			for attr, value := range v.Value {
				if core.InterfaceIsNil(value) {
					continue
				}
				item, err := core.DecodeValue(value)
				if err != nil {
					return errors.Wrap(err, "error decoding syntax token from jsonnet template")
				}
				syntaxToken.ObjectConst = append(syntaxToken.ObjectConst, core.ObjectConstItem{
					Key:   attr,
					Value: item,
				})
			}

			if v.Labels == nil {
				v.Labels = []string{}
			}
			bag := core.DataBag{
				Name:   v.Name,
				Type:   v.Type,
				Labels: v.Labels,
				Value:  syntaxToken,
			}
			if v.Name == "request-log_64cab7657bddc8663172702f59e27be6_ddb_replica_ind_sclpol_read" {
				log.Ctx(ctx).Debug().Msgf("%#v", bag, i, j)
			}
			err = container.Insert(bag)
			if err != nil {
				return errors.Wrap(err, "error merging databag on jsonnet template")
			}
		}
	}
	return nil
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
