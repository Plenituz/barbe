package hcl_parser

import (
	"barbe/core"
	"barbe/core/fetcher"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
)

func parseFromTemplate(ctx context.Context, container *core.ConfigContainer, userGeneratedFile fetcher.FileDescription) error {
	userGenerated, diags := hclsyntax.ParseConfig(userGeneratedFile.Content, userGeneratedFile.Name, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return diags
	}
	userGeneratedBody, ok := userGenerated.Body.(*hclsyntax.Body)
	if !ok {
		return errors.New("user generated file is not a *hclsyntax.Body, it's a " + fmt.Sprintf("%T", userGenerated.Body))
	}

	rootBag := core.DataBag{
		Name: "",
		Type: "",
		Value: core.SyntaxToken{
			Type:        core.TokenTypeObjectConst,
			ObjectConst: []core.ObjectConstItem{},
		},
	}
	for _, attr := range userGeneratedBody.Attributes {
		syntaxToken, err := hclExpressionToSyntaxToken(attr.Expr)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling data item")
		}
		rootBag.Value.ObjectConst = append(rootBag.Value.ObjectConst, core.ObjectConstItem{
			Key:   attr.Name,
			Value: *syntaxToken,
		})
	}
	if len(userGeneratedBody.Attributes) != 0 {
		err := container.Insert(rootBag)
		if err != nil {
			return errors.Wrap(err, "error merging bag")
		}
	}

	for _, block := range userGeneratedBody.Blocks {
		syntaxToken, err := blockToSyntaxToken(block, false)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling data item")
		}

		name := ""
		if len(block.Labels) > 0 {
			name = block.Labels[0]
			block.Labels = block.Labels[1:]
		} else {
			block.Labels = []string{}
		}
		if block.Type == "provider" {
			block.Type = "provider(" + uuid.New().String() + ")"
		}
		bag := core.DataBag{
			Name:   name,
			Type:   block.Type,
			Labels: block.Labels,
			Value:  *syntaxToken,
		}

		err = container.Insert(bag)
		if err != nil {
			return errors.Wrap(err, "error merging bag")
		}
	}
	return nil
}
