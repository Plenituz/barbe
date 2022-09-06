package terraform_fmt

import (
	"context"
	"errors"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/rs/zerolog/log"
	"os"
	"path"
	"barbe/core"
	"strings"
)

type TerraformFormatter struct{}

func (t TerraformFormatter) Name() string {
	return "terraform_fmt"
}

func (t TerraformFormatter) Format(ctx context.Context, data *core.ConfigContainer) error {
	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	hasAnything := false
	for resourceType, m := range data.DataBags {
		if !strings.HasPrefix(resourceType, "cr_") {
			continue
		}
		hasAnything = true

		writtenResourceType := resourceType
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
		if strings.HasPrefix(typeName, "provider") {
			typeName = strings.Split(typeName, "(")[0]
		}
		for name, group := range m {
			if name == "request-log_ddb_replica_auto_scaling_write_pol" {
				log.Debug().Msgf("%#v")
			}
			baseLabels := make([]string, 0)
			if writtenResourceType != "" {
				baseLabels = append(baseLabels, writtenResourceType)
			}
			if name != "" {
				baseLabels = append(baseLabels, name)
			}
			for _, databag := range group {
				labels := append([]string{}, baseLabels...)
				labels = append(labels, databag.Labels...)
				block := rootBody.AppendNewBlock(
					typeName,
					labels,
				)
				err := populateBlock(block, databag)
				if err != nil {
					return err
				}
			}
		}
	}

	if !hasAnything {
		return nil
	}

	outputDir := ctx.Value("maker").(*core.Maker).OutputDir
	outputPath := path.Join(outputDir, "generated.tf")

	log.Ctx(ctx).Debug().Msgf("Terraform formatter writing to %s", outputPath)
	o, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	_, err = f.WriteTo(o)
	if err != nil {
		return err
	}
	return nil
}

func populateBlock(block *hclwrite.Block, databag *core.DataBag) error {
	if databag.Name == "request-log_ddb_replica_auto_scaling_write_pol" {
		log.Debug().Msgf("%#v", databag)
	}
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
