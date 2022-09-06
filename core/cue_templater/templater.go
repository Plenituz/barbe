package cue_templater

import (
	"context"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerror "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"barbe/core"
	"strings"
	"testing/fstest"
)

type sugarBag struct {
	Name   string                 `cue:"name"`
	Type   string                 `cue:"type"`
	Labels []string               `cue:"labels"`
	Value  map[string]interface{} `cue:"value"`
}

func applyTemplate(ctx context.Context, container *core.ConfigContainer, templates []core.FileDescription) error {
	cueCtx := cuecontext.New()

	valueCtx := cueCtx.Encode(map[string]interface{}{
		"container": container.DataBags,
		"env":       envMap(),
	})
	if valueCtx.Err() != nil {
		return errors.Wrap(valueCtx.Err(), "failed to encode cue context")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "failed to get working directory")
	}

	abs := func(path string) string {
		return filepath.Join(cwd, path)
	}

	overlays := map[string]fs.FS{
		"": Builtins,
	}

	for _, templateFile := range templates {
		if path.Ext(templateFile.Name) != ".cue" {
			continue
		}
		fakeFs := fstest.MapFS{
			templateFile.Name: &fstest.MapFile{
				Data: templateFile.Content,
			},
		}
		overlays[uuid.New().String()] = fakeFs
	}

	buildConfig := &load.Config{
		Dir: cwd,
		Overlay: map[string]load.Source{
			abs("cue.mod"): load.FromString(`module: "github.com/Plenituz"`),
		},
	}

	for mnt, f := range overlays {
		f := f
		mnt := mnt
		err := fs.WalkDir(f, ".", func(p string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.Type().IsRegular() {
				return nil
			}

			if filepath.Ext(entry.Name()) != ".cue" {
				return nil
			}

			contents, err := fs.ReadFile(f, p)
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}

			overlayPath := path.Join(buildConfig.Dir, mnt, p)
			buildConfig.Overlay[overlayPath] = load.FromBytes(contents)
			return nil
		})
		if err != nil {
			return err
		}
	}

	instances := load.Instances([]string{"./..."}, buildConfig)
	for _, value := range instances {
		if value.Err != nil {
			log.Ctx(ctx).Error().Err(value.Err).Msg("cue template execution failed")
			return value.Err
		}
	}

	//TODO parallelize cue execution?
	instanceResults := make([]cue.Value, 0, len(instances))
	for _, instance := range instances {
		v := cueCtx.BuildInstance(instance, cue.Scope(valueCtx))
		instanceResults = append(instanceResults, v)
	}
	if err != nil {
		return err
	}

	for _, value := range instanceResults {
		if value.Err() != nil {
			e := value.Err()
			details := cueerror.Details(e, nil)
			log.Ctx(ctx).Error().Err(e).Msgf("cue template execution failed: %s", details)
			return value.Err()
		}
	}

	type parsedContainer struct {
		Databags []sugarBag `cue:"databags"`
	}

	parseContainers := make([]parsedContainer, 0, len(instanceResults))
	for _, value := range instanceResults {
		var m parsedContainer
		err = value.Decode(&m)
		if err != nil && err.Error() != "value was rounded down" {
			log.Ctx(ctx).Warn().Err(err).Msg("while decoding cue value")
		}
		parseContainers = append(parseContainers, m)
	}
	/*
		piece of code to profile CUE execution
		func (c *compiler) expr(expr ast.Expr) adt.Expr {
			if expr != nil {
				t := time.Now()
				defer func() {
					fmt.Println("expr:", expr.Pos().String(), expr.End().String(), "took", time.Since(t))
				}()
			}
	*/

	for _, c := range parseContainers {
		for _, v := range c.Databags {
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
					return errors.Wrap(err, "error decoding syntax token from cue template")
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
			err = container.Insert(bag)
			if err != nil {
				return errors.Wrap(err, "error merging databag on cue template")
			}
		}
	}

	return nil
}

func envMap() map[string]string {
	//TODO this may not work as well on windows, see https://github.com/caarlos0/env/blob/main/env_windows.go
	r := map[string]string{}
	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		r[p[0]] = p[1]
	}
	return r
}
