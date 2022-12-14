package traversal_manipulator

import (
	"barbe/core"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"reflect"
)

type tokenMap struct {
	Match     core.SyntaxToken
	ReplaceBy core.SyntaxToken
}

func mapTokens(ctx context.Context, data *core.ConfigContainer) error {
	transformMaps := make([]tokenMap, 0)
	for resourceType, m := range data.DataBags {
		if resourceType != "token_map" {
			continue
		}
		for name, group := range m {
			for i, databag := range group {
				if databag.Value.Type != core.TokenTypeArrayConst {
					return fmt.Errorf("token_map databag '%s[%d]' is not an array", name, i)
				}
				for j, item := range databag.Value.ArrayConst {
					parsed, err := parseMatchObj(ctx, item)
					if err != nil {
						return errors.Wrap(err, fmt.Sprintf("error parsing token_map databag at '%s[%d][%d]'", name, i, j))
					}
					transformMaps = append(transformMaps, parsed)
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
				err := tokenMapperLoop(ctx, databag, transformMaps)
				if err != nil {
					return errors.Wrapf(err, "error applying token_map to databag '%s[%d]'", name, i)
				}
				data.DataBags[resourceType][name][i] = databag
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

func tokenMapperLoop(ctx context.Context, databag *core.DataBag, transformMaps []tokenMap) error {
	for i := 0; i < 100; i++ {
		count := 0
		transformed, err := visitTokenMappers(ctx, core.TokenPtr(databag.Value), transformMaps, func() {
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

func visitTokenMappers(ctx context.Context, root *core.SyntaxToken, transformMap []tokenMap, counter func()) (*core.SyntaxToken, error) {
	return core.Visit(ctx, root, func(token *core.SyntaxToken) (*core.SyntaxToken, error) {
		for _, transform := range transformMap {
			if !reflect.DeepEqual(*token, transform.Match) {
				continue
			}
			counter()
			return core.TokenPtr(transform.ReplaceBy), nil
		}
		return nil, nil
	})
}
