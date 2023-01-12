package zipper_fmt

import (
	"barbe/core"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"path"
	"strconv"
)

type ZipperFormatter struct{}

func (t ZipperFormatter) Name() string {
	return "zipper_fmt"
}

func (t ZipperFormatter) Format(ctx context.Context, data core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "zipper" {
			continue
		}

		for name, group := range m {
			for i, databag := range group {
				err := applyZipper(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying zipper '%s[%d]'", name, i)
				}
			}
		}
	}
	return nil
}

func applyZipper(ctx context.Context, databag core.DataBag) error {
	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting current working directory")
	}

	if databag.Value.Type != core.TokenTypeObjectConst {
		return errors.New("zipper databag's syntax token must be of type object")
	}

	fileMap := map[string]string{}
	outputFiles := map[string]struct{}{}
	includePatterns := map[string]struct{}{}
	excludePatterns := map[string]struct{}{}
	outputDir := ctx.Value("maker").(*core.Maker).OutputDir

	for _, pair := range databag.Value.ObjectConst {
		switch pair.Key {
		case "file_map":
			if pair.Value.Type != core.TokenTypeObjectConst {
				return errors.New("zipper[" + pair.Key + "].file_map must be of type object")
			}
			for _, innerPair := range pair.Value.ObjectConst {
				value, err := core.ExtractAsStringValue(innerPair.Value)
				if err != nil {
					return errors.Wrap(err, "error extracting zipper["+pair.Key+"].file_map["+innerPair.Key+"] as string")
				}
				fileMap[innerPair.Key] = value
			}
		case "output_file":
			o, err := core.ExtractAsStringValue(pair.Value)
			if err != nil {
				return errors.Wrap(err, "error extracting zipper["+pair.Key+"].output_file as string")
			}
			outputPath := path.Join(outputDir, o)
			outputFiles[outputPath] = struct{}{}
		case "include":
			if pair.Value.Type != core.TokenTypeArrayConst {
				return errors.New("zipper." + pair.Key + " must be an array")
			}
			for i, item := range pair.Value.ArrayConst {
				value, err := core.ExtractAsStringValue(item)
				if err != nil {
					return errors.Wrap(err, "error extracting zipper."+pair.Key+"["+strconv.Itoa(i)+"] as string")
				}
				includePatterns[value] = struct{}{}
			}
		case "exclude":
			if pair.Value.Type != core.TokenTypeArrayConst {
				return errors.New("zipper." + pair.Key + " must be an array")
			}
			for i, item := range pair.Value.ArrayConst {
				value, err := core.ExtractAsStringValue(item)
				if err != nil {
					return errors.Wrap(err, "error extracting zipper."+pair.Key+"["+strconv.Itoa(i)+"] as string")
				}
				excludePatterns[value] = struct{}{}
			}
		}
	}

	includePatternsStr := make([]string, 0, len(includePatterns))
	for pattern := range includePatterns {
		includePatternsStr = append(includePatternsStr, pattern)
	}
	excludePatternsStr := make([]string, 0, len(excludePatterns))
	for pattern := range excludePatterns {
		excludePatternsStr = append(excludePatternsStr, pattern)
	}

	for outputFile := range outputFiles {
		log.Ctx(ctx).Debug().Msgf("zipping: %s", outputFile)
		err := doTheZip(ctx, outputFile, wd, includePatternsStr, excludePatternsStr, fileMap)
		if err != nil {
			return errors.Wrap(err, "error zipping '"+outputFile+"'")
		}
	}
	return nil
}
