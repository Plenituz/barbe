package traversal_manipulator

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"barbe/core"
	"strings"
)

func transformTraversals(ctx context.Context, data *core.ConfigContainer) error {
	transformMap := map[string]string{}
	for resourceType, m := range data.DataBags {
		if resourceType != "traversal_transform" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					return fmt.Errorf("traversal_transform databag '%s[%d]' is not an object", name, i)
				}
				for _, pair := range databag.Value.ObjectConst {
					strValue, err := core.ExtractAsStringValue(pair.Value)
					if err != nil {
						return errors.Wrap(err, "error extracting string value from traversal_transform value")
					}
					transformMap[pair.Key] = strValue
				}
			}
		}
	}

	for resourceType, m := range data.DataBags {
		if resourceType == "traversal_transform" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				err := transformerLoop(ctx, databag, transformMap)
				if err != nil {
					return errors.Wrapf(err, "error applying traversal_transform to databag '%s'", name)
				}
				data.DataBags[resourceType][name][i] = databag
			}
		}
	}
	return nil
}

func transformerLoop(ctx context.Context, databag *core.DataBag, transformMap map[string]string) error {
	for {
		count := 0
		transformed, err := visitTransformers(ctx, core.TokenPtr(databag.Value), transformMap, func() {
			count++
		})
		if err != nil {
			return err
		}
		databag.Value = *transformed
		if count == 0 {
			break
		}
	}
	return nil
}

func visitTransformers(ctx context.Context, root *core.SyntaxToken, transformMap map[string]string, counter func()) (*core.SyntaxToken, error) {
	return core.Visit(ctx, root, func(token *core.SyntaxToken) (*core.SyntaxToken, error) {
		switch token.Type {
		//TODO maybe need to support relative traversal here?
		// the simplifier makes it unnecessary for now
		case core.TokenTypeScopeTraversal:
			transformed, err := transformTraversal(token.Traversal, transformMap, counter)
			if err != nil {
				return nil, errors.Wrap(err, "error transforming traversal")
			}
			return &core.SyntaxToken{
				Type:      core.TokenTypeScopeTraversal,
				Traversal: transformed,
			}, nil
		}
		return nil, nil
	})
}

func traversalToString(traversal []core.Traverse) (string, error) {
	isAttr := func(t core.Traverse) bool {
		return t.Type == core.TraverseTypeAttr ||
			(t.Type == core.TraverseTypeIndex && reflect.ValueOf(t.Index).Kind() == reflect.String)
	}
	getAttrStr := func(t core.Traverse) string {
		if t.Type == core.TraverseTypeAttr {
			return *t.Name
		}
		return t.Index.(string)
	}
	isIndex := func(t core.Traverse) bool {
		return t.Type == core.TraverseTypeIndex && reflect.ValueOf(t.Index).Kind() != reflect.String
	}

	str := ""
	for i, t := range traversal {
		switch {
		case isAttr(t):
			str += getAttrStr(t)
			if i < len(traversal)-1 && isAttr(traversal[i+1]) {
				str += "."
			}
		case isIndex(t):
			str += fmt.Sprintf("[%d]", t.Index)
		default:
			return "", fmt.Errorf("unknown traversal type %v", t.Type)
		}
	}
	return str, nil
}

func s(s string) *string {
	return &s
}

func stringToTraversal(str string) ([]core.Traverse, error) {
	split := strings.Split(str, ".")
	traversal := make([]core.Traverse, 0, len(split))
	for _, item := range split {
		if strings.HasPrefix(item, "[") && strings.HasSuffix(item, "]") {
			traversal = append(traversal, core.Traverse{
				Type:  core.TraverseTypeIndex,
				Index: item,
			})
		} else {
			traversal = append(traversal, core.Traverse{
				Type: core.TraverseTypeAttr,
				Name: s(item),
			})
		}
	}
	return traversal, nil
}

func transformTraversal(traversal []core.Traverse, transformMap map[string]string, counter func()) ([]core.Traverse, error) {
	for i := len(traversal) - 1; i >= 0; i-- {
		traversalStr, err := traversalToString(traversal[:i+1])
		if err != nil {
			return nil, errors.Wrap(err, "error converting traversal to string")
		}
		if mappedValue, ok := transformMap[traversalStr]; ok {
			//fmt.Println(fmt.Sprintf("transforming traversal %s -> %s", traversalStr, mappedValue))
			counter()
			root, err := stringToTraversal(mappedValue)
			if err != nil {
				return nil, errors.Wrapf(err, "error converting string '%s' to traversal", mappedValue)
			}
			return append(root, traversal[i+1:]...), nil
		}
	}
	return traversal, nil
}
