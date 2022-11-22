package traversal_manipulator

import (
	"barbe/core"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

func (t *TraversalManipulator) transformTraversals(ctx context.Context, data core.ConfigContainer, output *core.ConfigContainer) error {
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
					t.traversalTransforms[pair.Key] = strValue
				}
			}
		}
	}

	for resourceType, m := range data.DataBags {
		if resourceType == "traversal_transform" {
			continue
		}
		for name, group := range m {
			for _, databag := range group {
				changed, changedBag, err := transformerLoop(ctx, databag, t.traversalTransforms)
				if err != nil {
					return errors.Wrapf(err, "error applying traversal_transform to databag '%s'", name)
				}
				if !changed {
					continue
				}
				err = output.Insert(changedBag)
				if err != nil {
					return errors.Wrapf(err, "error inserting transformed databag '%s'", name)
				}
			}
		}
	}
	return nil
}

func transformerLoop(ctx context.Context, databag core.DataBag, transformMap map[string]string) (changed bool, changedBag core.DataBag, e error) {
	for i := 0; i < 100; i++ {
		shouldStop := true
		transformed, err := visitTransformers(ctx, core.TokenPtr(databag.Value), transformMap, func() {
			changed = true
			shouldStop = false
		})
		if err != nil {
			return false, core.DataBag{}, err
		}
		databag.Value = *transformed
		if shouldStop {
			break
		}
	}
	return changed, databag, nil
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
				Name: core.Ptr(item),
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
