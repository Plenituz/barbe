package simplifier_transform

import (
	"barbe/core"
	"context"
	"github.com/pkg/errors"
	"reflect"
)

type SimplifierTransformer struct{}

func (t SimplifierTransformer) Name() string {
	return "simplifier_transform"
}

func (t SimplifierTransformer) Transform(ctx context.Context, data core.ConfigContainer) (core.ConfigContainer, error) {
	output := core.NewConfigContainer()
	for resourceType, m := range data.DataBags {
		for name, group := range m {
			for _, databag := range group {
				changed, changedBag, err := simplifyLoop(ctx, databag)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrapf(err, "error simplifying databag '%s.%s'", resourceType, name)
				}
				if !changed {
					continue
				}
				err = output.Insert(changedBag)
				if err != nil {
					return core.ConfigContainer{}, errors.Wrapf(err, "error inserting simplified databag '%s.%s'", resourceType, name)
				}
			}
		}
	}
	return *output, nil
}

func simplifyLoop(ctx context.Context, databag core.DataBag) (changed bool, changedBag core.DataBag, e error) {
	for {
		shouldStop := true
		simplified, err := visit(ctx, core.TokenPtr(databag.Value), func() {
			changed = true
			shouldStop = false
		})
		if err != nil {
			return false, core.DataBag{}, err
		}
		databag.Value = *simplified
		if shouldStop {
			break
		}
	}
	return changed, databag, nil
}

func visit(ctx context.Context, token *core.SyntaxToken, counter func()) (*core.SyntaxToken, error) {
	return core.Visit(ctx, token, func(token *core.SyntaxToken) (*core.SyntaxToken, error) {
		v, err := simplifier(token)
		if err != nil {
			return nil, err
		}
		if v != nil {
			counter()
		}
		return v, nil
	})
}

func simplifier(token *core.SyntaxToken) (*core.SyntaxToken, error) {
	switch token.Type {
	case core.TokenTypeTemplate:
		if IsSimpleTemplate(*token) {
			str, err := core.ExtractAsStringValue(*token)
			if err != nil {
				return nil, errors.Wrap(err, "error extracting string value from template")
			}
			return &core.SyntaxToken{
				Type:  core.TokenTypeLiteralValue,
				Value: str,
			}, nil
		}
		if len(token.Parts) == 1 {
			return &token.Parts[0], nil
		}
		hasTemplate := false
		for _, part := range token.Parts {
			if part.Type == core.TokenTypeTemplate {
				hasTemplate = true
				break
			}
		}
		if hasTemplate {
			flattened := make([]core.SyntaxToken, 0, len(token.Parts))
			for _, part := range token.Parts {
				if part.Type == core.TokenTypeTemplate {
					flattened = append(flattened, part.Parts...)
				} else {
					flattened = append(flattened, part)
				}
			}
			return &core.SyntaxToken{
				Type:  core.TokenTypeTemplate,
				Parts: flattened,
			}, nil
		}
	case core.TokenTypeRelativeTraversal:
		if token.Source.Type == core.TokenTypeScopeTraversal {
			return &core.SyntaxToken{
				Type:      core.TokenTypeScopeTraversal,
				Traversal: append(token.Source.Traversal, token.Traversal...),
			}, nil
		}
	case core.TokenTypeScopeTraversal:
		for i, part := range token.Traversal {
			if part.Type == core.TraverseTypeIndex && reflect.TypeOf(part.Index).Kind() == reflect.String {
				v := part.Index.(string)
				token.Traversal[i] = core.Traverse{
					Type: core.TraverseTypeAttr,
					Name: &v,
				}
			}
		}
	case core.TokenTypeIndexAccess:
		if IsSimpleTemplate(*token.IndexKey) &&
			(token.IndexCollection.Type == core.TokenTypeScopeTraversal || token.IndexCollection.Type == core.TokenTypeRelativeTraversal) {

			str, err := core.ExtractAsStringValue(*token.IndexKey)
			if err != nil {
				return nil, errors.Wrap(err, "error extracting string value from template")
			}
			return &core.SyntaxToken{
				Type:   token.IndexCollection.Type,
				Source: token.IndexCollection.Source,
				Traversal: append(token.IndexCollection.Traversal, core.Traverse{
					Type: core.TraverseTypeAttr,
					Name: &str,
				}),
			}, nil
		}
	}
	return nil, nil
}

//IsSimpleTemplate returns true if the given token is a template that only contains
//string values (no traversal or anything else)
func IsSimpleTemplate(token core.SyntaxToken) bool {
	if token.Type != core.TokenTypeTemplate {
		return false
	}
	for _, part := range token.Parts {
		if part.Type == core.TokenTypeLiteralValue {
			continue
		}
		if part.Type == core.TokenTypeTemplate {
			if !IsSimpleTemplate(part) {
				return false
			}
			continue
		}
		return false
	}
	return true
}
