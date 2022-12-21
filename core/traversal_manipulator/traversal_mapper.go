package traversal_manipulator

import (
	"barbe/core"
	"context"
	"fmt"
	"github.com/pkg/errors"
)

func (t *TraversalManipulator) mapTraversals(ctx context.Context, data core.ConfigContainer, output *core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "traversal_map" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				if databag.Value.Type != core.TokenTypeObjectConst {
					return fmt.Errorf("traversal_map databag '%s[%d]' is not an object", name, i)
				}
				t.traversalMapsMutex.Lock()
				for _, pair := range databag.Value.ObjectConst {
					t.traversalMaps[pair.Key] = pair.Value
				}
				t.traversalMapsMutex.Unlock()
			}
		}
	}

	for resourceType, m := range data.DataBags {
		if resourceType == "traversal_map" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				changed, changedBag, err := t.mapperLoop(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying traversal_map to databag '%s[%d]'", name, i)
				}
				if !changed {
					continue
				}
				err = output.Insert(changedBag)
				if err != nil {
					return errors.Wrapf(err, "error inserting databag '%s[%d]'", name, i)
				}
			}
		}
	}
	return nil
}

func (t *TraversalManipulator) mapperLoop(ctx context.Context, databag core.DataBag) (changed bool, changedBag core.DataBag, e error) {
	for i := 0; i < 100; i++ {
		shouldStop := true
		transformed, err := t.visitMappers(ctx, core.TokenPtr(databag.Value), func() {
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

func (t *TraversalManipulator) visitMappers(ctx context.Context, root *core.SyntaxToken, counter func()) (*core.SyntaxToken, error) {
	return core.Visit(ctx, root, func(token *core.SyntaxToken) (*core.SyntaxToken, error) {
		switch token.Type {
		//TODO maybe need to support relative traversal here?
		// the simplifier makes it unnecessary for now
		case core.TokenTypeScopeTraversal:
			return t.mapTraversal(token.Traversal, counter)
		}
		return nil, nil
	})
}

func (t *TraversalManipulator) mapTraversal(traversal []core.Traverse, counter func()) (*core.SyntaxToken, error) {
	traversalStr, err := traversalToString(traversal)
	if err != nil {
		return nil, errors.Wrap(err, "error converting traversal to string")
	}
	t.traversalMapsMutex.RLock()
	defer t.traversalMapsMutex.RUnlock()
	if mappedValue, ok := t.traversalMaps[traversalStr]; ok {
		//fmt.Println(fmt.Sprintf("mapping traversal %s -> %+v", traversalStr, mappedValue))
		counter()
		return &mappedValue, nil
	}
	return nil, nil
}
