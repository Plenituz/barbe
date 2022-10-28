package terraform_fmt

import (
	"barbe/core"
	"barbe/core/chown_util"
	"context"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"os"
	"path"
	"strings"
)

const (
	TerraformSubdirMetaKey = "sub_dir"
)

type TerraformFormatter struct{}

func (t TerraformFormatter) Name() string {
	return "terraform_fmt"
}

func (t TerraformFormatter) Format(ctx context.Context, data *core.ConfigContainer) error {
	cloudResourcesPerDir := map[string][]*core.DataBag{
		"": {},
	}
	hasAnything := false
	for resourceType, m := range data.DataBags {
		if !strings.HasPrefix(resourceType, "cr_") {
			continue
		}
		for _, group := range m {
			for _, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					continue
				}

				hasAnything = true
				if subdir := core.GetMeta[string](databag.Value, TerraformSubdirMetaKey); subdir != "" {
					if _, ok := cloudResourcesPerDir[subdir]; !ok {
						cloudResourcesPerDir[subdir] = []*core.DataBag{}
					}
					cloudResourcesPerDir[subdir] = append(cloudResourcesPerDir[subdir], databag)
				} else {
					cloudResourcesPerDir[""] = append(cloudResourcesPerDir[""], databag)
				}
			}
		}
	}
	if !hasAnything {
		return nil
	}

	for subdir, bags := range cloudResourcesPerDir {
		err := writeTerraform(ctx, subdir, bags)
		if err != nil {
			return errors.Wrap(err, "failed to write terraform in subdir '"+subdir+"'")
		}
	}

	return nil
}

func writeTerraform(ctx context.Context, subdir string, bags []*core.DataBag) error {
	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()
	for _, databag := range bags {
		writtenResourceType := databag.Type
		typeName := "resource"
		if strings.HasPrefix(writtenResourceType, "cr_[") {
			s := strings.TrimPrefix(writtenResourceType, "cr_[")
			if !strings.Contains(s, "]") {
				return errors.New("invalid resource type '" + writtenResourceType + "'")
			}
			split := strings.SplitN(s, "]", 2)
			typeName = split[0]
			writtenResourceType = strings.TrimPrefix(split[1], "_")
		} else {
			writtenResourceType = strings.TrimPrefix(writtenResourceType, "cr_")
		}
		if strings.Contains(typeName, "(") {
			typeName = strings.Split(typeName, "(")[0]
		}
		labels := make([]string, 0)
		if writtenResourceType != "" {
			labels = append(labels, writtenResourceType)
		}
		if databag.Name != "" {
			labels = append(labels, databag.Name)
		}
		labels = append(labels, databag.Labels...)
		if typeName == "terraform" {
			//terraform blocks never have a label
			labels = []string{}
		}
		block := rootBody.AppendNewBlock(
			typeName,
			labels,
		)
		err := populateBlock(block, databag)
		if err != nil {
			return err
		}
	}

	outputDir := ctx.Value("maker").(*core.Maker).OutputDir
	if subdir != "" {
		outputDir = path.Join(outputDir, subdir)
	}
	outputPath := path.Join(outputDir, "generated.tf")

	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create output dir '"+outputDir+"'")
	}

	log.Ctx(ctx).Debug().Msgf("Terraform formatter writing to %s", outputPath)
	o, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer o.Close()

	_, err = f.WriteTo(o)
	if err != nil {
		return err
	}

	chown_util.TryRectifyRootFiles(ctx, []string{outputDir, outputPath})
	return nil
}

func populateBlock(block *hclwrite.Block, databag *core.DataBag) error {
	val, err := syntaxTokenToHclTokens(databag.Value, nil)
	if err != nil {
		return err
	}
	//we assume here that the databag is an object and so we trim the first and last bracket '{ ... }'
	//because the block comes with its own brackets
	val = val[1:]
	//there is also a new line token here
	val = val[1:]
	if len(val) != 0 {
		val = val[:len(val)-1]
	}
	block.Body().AppendUnstructuredTokens(val)
	return nil
}
