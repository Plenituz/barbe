package traversal_manipulator

import (
	"barbe/core"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"reflect"
)

func (t *TraversalManipulator) mapTokens(ctx context.Context, data core.ConfigContainer, output *core.ConfigContainer) error {
	for resourceType, m := range data.DataBags {
		if resourceType != "token_map" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				if databag.Value.Type != core.TokenTypeArrayConst {
					return fmt.Errorf("token_map databag '%s[%d]' is not an array", name, i)
				}
				t.tokenMapsMutex.Lock()
				for j, item := range databag.Value.ArrayConst {
					parsed, err := parseMatchObj(ctx, item)
					if err != nil {
						t.tokenMapsMutex.Unlock()
						return errors.Wrap(err, fmt.Sprintf("error parsing token_map databag at '%s[%d][%d]'", name, i, j))
					}
					t.tokenMaps = append(t.tokenMaps, parsed)
					t.tokenMapsMutex.Unlock()
				}
			}
		}
	}

	for resourceType, m := range data.DataBags {
		if resourceType == "token_map" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				changed, changedBag, err := t.tokenMapperLoop(ctx, databag)
				if err != nil {
					return errors.Wrapf(err, "error applying token_map to databag '%s[%d]'", name, i)
				}
				if !changed {
					continue
				}
				err = output.Insert(changedBag)
				if err != nil {
					return errors.Wrapf(err, "error inserting changed databag '%s[%d]'", name, i)
				}
			}
		}
	}
	return nil
}

func parseMatchObj(ctx context.Context, token core.SyntaxToken) (tokenMap, error) {
	if token.Type != core.TokenTypeObjectConst {
		return tokenMap{}, fmt.Errorf("token is not an object")
	}
	result := tokenMap{}

	matchToken := core.GetObjectKeyValues("match", token.ObjectConst)
	if len(matchToken) == 0 {
		return tokenMap{}, fmt.Errorf("token has no 'match' key")
	}
	if len(matchToken) > 1 {
		log.Ctx(ctx).Warn().Msg("token has more than one 'match' key, using the first one")
	}
	result.Match = matchToken[0]

	replaceByToken := core.GetObjectKeyValues("replace_by", token.ObjectConst)
	if len(replaceByToken) == 0 {
		return tokenMap{}, fmt.Errorf("token has no 'replace_by' key")
	}
	if len(replaceByToken) > 1 {
		log.Ctx(ctx).Warn().Msg("token has more than one 'replace_by' key, using the first one")
	}
	result.ReplaceBy = replaceByToken[0]

	return result, nil
}

func (t *TraversalManipulator) tokenMapperLoop(ctx context.Context, databag core.DataBag) (changed bool, changedBag core.DataBag, e error) {
	for i := 0; i < 100; i++ {
		shouldStop := true
		transformed, err := t.visitTokenMappers(ctx, core.TokenPtr(databag.Value), func() {
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

func (t *TraversalManipulator) visitTokenMappers(ctx context.Context, root *core.SyntaxToken, counter func()) (*core.SyntaxToken, error) {
	return core.Visit(ctx, root, func(token *core.SyntaxToken) (*core.SyntaxToken, error) {
		t.tokenMapsMutex.RLock()
		defer t.tokenMapsMutex.RUnlock()
		for _, transform := range t.tokenMaps {
			if !reflect.DeepEqual(*token, transform.Match) {
				continue
			}
			counter()
			return core.TokenPtr(transform.ReplaceBy), nil
		}
		return nil, nil
	})
}
