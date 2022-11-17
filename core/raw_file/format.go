package raw_file

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"github.com/pkg/errors"
	"os"
	"path"
)

type RawFileFormatter struct{}

func (t RawFileFormatter) Name() string {
	return "raw_file"
}

func (t RawFileFormatter) Format(ctx context.Context, data *core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "raw_file" {
			continue
		}

		for name, group := range m {
			for i, databag := range group {
				err := applyRawFile(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying raw_file to '%s[%d]'", name, i)
				}
			}
		}
	}
	return nil
}

func applyRawFile(ctx context.Context, databag core.DataBag) error {
	if databag.Value.Type != core.TokenTypeObjectConst {
		return errors.New("raw_file databag's syntax token must be of type object")
	}

	outputDir := ctx.Value("maker").(*core.Maker).OutputDir
	outputPath := ""
	content := ""
	for _, pair := range databag.Value.ObjectConst {
		switch pair.Key {
		case "path":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_file."+pair.Key+" as string")
			}
			outputPath = path.Join(outputDir, o)
		case "content":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting raw_file."+pair.Key+" as string")
			}
			content = o
		}
	}
	if outputPath == "" {
		return errors.New("raw_file.path must be defined")
	}
	defer chown_util.TryRectifyRootFiles(ctx, []string{
		path.Dir(outputPath),
		outputPath,
	})

	err := os.MkdirAll(path.Dir(outputPath), 0755)
	if err != nil {
		return errors.Wrap(err, "error creating raw_file directory '"+path.Dir(outputPath)+"'")
	}

	err = os.WriteFile(outputPath, []byte(content), 0644)
	if err != nil {
		return errors.Wrap(err, "error writing file at '"+outputPath+"'")
	}
	return nil
}
