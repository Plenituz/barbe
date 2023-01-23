package terraform_fmt

import (
	"barbe/core"
	"errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"reflect"
)

//this is somewhat based on hclwrite.appendTokensForValue
func syntaxTokenToHclTokens(item core.SyntaxToken, parentName *string) (hclwrite.Tokens, error) {
	switch item.Type {
	default:
		return nil, errors.New("unsupported data item type '" + item.Type + "'")

	case core.TokenTypeAnonymous:
		toks := make(hclwrite.Tokens, 0)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrack,
			Bytes: []byte{'['},
		})
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenStar,
			Bytes: []byte{'*'},
		})
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrack,
			Bytes: []byte{']'},
		})
		return toks, nil

	case core.TokenTypeSplat:
		source, err := syntaxTokenToHclTokens(*item.Source, nil)
		if err != nil {
			return nil, err
		}
		splat, err := syntaxTokenToHclTokens(*item.SplatEach, nil)
		if err != nil {
			return nil, err
		}
		toks := make(hclwrite.Tokens, 0)
		toks = append(toks, source...)
		toks = append(toks, splat...)
		return toks, nil

	case core.TokenTypeLiteralValue:
		tokens, err := primitiveToTokens(item.Value)
		if err != nil {
			return nil, err
		}
		return tokens, nil

	case core.TokenTypeScopeTraversal:
		return traversalToTokens(item.Traversal, false)

	case core.TokenTypeObjectConst:
		toks := make(hclwrite.Tokens, 0)

		if core.GetMeta[bool](item, "IsBlock") {
			labels := make([]string, 0)
			if _, ok := item.Meta["BlockLabels"]; ok {
				labels = core.GetMetaComplexType[[]string](item, "BlockLabels")
			}
			for _, label := range labels {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenOQuote,
					Bytes: []byte{'"'},
				})
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenQuotedLit,
					Bytes: escapeQuotedStringLit(label),
				})
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenCQuote,
					Bytes: []byte{'"'},
				})
			}
		}
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrace,
			Bytes: []byte{'{'},
		})
		if len(item.ObjectConst) > 0 {
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenNewline,
				Bytes: []byte{'\n'},
			})
		}

		for _, objConst := range item.ObjectConst {
			if objConst.Value.Type == "" {
				continue
			}
			eKey := objConst.Key
			if hclsyntax.ValidIdentifier(eKey) {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenIdent,
					Bytes: []byte(eKey),
				})
			} else {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenOQuote,
					Bytes: []byte{'"'},
				})
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenQuotedLit,
					Bytes: escapeQuotedStringLit(eKey),
				})
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenCQuote,
					Bytes: []byte{'"'},
				})
			}
			if !core.GetMetaBool(objConst.Value, "IsBlock") {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenEqual,
					Bytes: []byte{'='},
				})
			}
			valueTokens, err := syntaxTokenToHclTokens(objConst.Value, core.Ptr(eKey))
			if err != nil {
				return nil, err
			}
			toks = append(toks, valueTokens...)
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenNewline,
				Bytes: []byte{'\n'},
			})
		}

		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrace,
			Bytes: []byte{'}'},
		})
		return toks, nil

	case core.TokenTypeArrayConst:
		toks := make(hclwrite.Tokens, 0)
		if core.GetMetaBool(item, "IsBlock") {
			if parentName == nil {
				return nil, errors.New("cannot create block without a parent")
			}
			if !hclsyntax.ValidIdentifier(*parentName) {
				return nil, errors.New("invalid parent name for block '" + *parentName + "'")
			}

			for i, arrayConst := range item.ArrayConst {
				if i > 0 {
					toks = append(toks, &hclwrite.Token{
						Type:  hclsyntax.TokenNewline,
						Bytes: []byte{'\n'},
					})
					//the first item will already have the parent name in front
					toks = append(toks, &hclwrite.Token{
						Type:  hclsyntax.TokenIdent,
						Bytes: []byte(*parentName),
					})
				}
				valueTokens, err := syntaxTokenToHclTokens(arrayConst, nil)
				if err != nil {
					return nil, err
				}
				toks = append(toks, valueTokens...)
			}

		} else {
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenOBrack,
				Bytes: []byte{'['},
			})

			for i, arrayConst := range item.ArrayConst {
				if i > 0 {
					toks = append(toks, &hclwrite.Token{
						Type:  hclsyntax.TokenComma,
						Bytes: []byte{','},
					})
				}
				valueTokens, err := syntaxTokenToHclTokens(arrayConst, nil)
				if err != nil {
					return nil, err
				}
				toks = append(toks, valueTokens...)
			}

			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenCBrack,
				Bytes: []byte{']'},
			})
		}
		return toks, nil

	case core.TokenTypeTemplate:
		toks := make(hclwrite.Tokens, 0)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOQuote,
			Bytes: []byte{'"'},
		})

		for _, part := range item.Parts {
			if part.Type == core.TokenTypeLiteralValue {
				if _, ok := part.Value.(string); !ok {
					return nil, errors.New("template part is a literal value that is not a string, it's a '" + reflect.TypeOf(part.Value).String() + "'")
				}
				src := escapeQuotedStringLit(part.Value.(string))
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenQuotedLit,
					Bytes: src,
				})
			} else {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenTemplateInterp,
					Bytes: []byte(`${`),
				})

				valueTokens, err := syntaxTokenToHclTokens(part, nil)
				if err != nil {
					return nil, err
				}
				toks = append(toks, valueTokens...)

				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenTemplateSeqEnd,
					Bytes: []byte(`}`),
				})
			}
		}
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCQuote,
			Bytes: []byte{'"'},
		})
		return toks, nil

	case core.TokenTypeFunctionCall:
		toks := make(hclwrite.Tokens, 0)
		if !hclsyntax.ValidIdentifier(*item.FunctionName) {
			return nil, errors.New("function name is not a valid identifier")
		}
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenIdent,
			Bytes: []byte(*item.FunctionName),
		})
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOParen,
			Bytes: []byte(`(`),
		})

		for i, arg := range item.FunctionArgs {
			if i > 0 {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenComma,
					Bytes: []byte{','},
				})
			}
			valueTokens, err := syntaxTokenToHclTokens(arg, nil)
			if err != nil {
				return nil, err
			}
			toks = append(toks, valueTokens...)
		}

		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCParen,
			Bytes: []byte(`)`),
		})
		return toks, nil

	case core.TokenTypeIndexAccess:
		toks := make(hclwrite.Tokens, 0)
		collection, err := syntaxTokenToHclTokens(*item.IndexCollection, nil)
		if err != nil {
			return nil, err
		}
		key, err := syntaxTokenToHclTokens(*item.IndexKey, nil)
		if err != nil {
			return nil, err
		}
		toks = append(toks, collection...)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOBrack,
			Bytes: []byte(`[`),
		})
		toks = append(toks, key...)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCBrack,
			Bytes: []byte(`]`),
		})
		return toks, nil

	case core.TokenTypeRelativeTraversal:
		toks := make(hclwrite.Tokens, 0)
		source, err := syntaxTokenToHclTokens(*item.Source, nil)
		if err != nil {
			return nil, err
		}
		traversal, err := traversalToTokens(item.Traversal, true)
		if err != nil {
			return nil, err
		}
		if item.Source.Type == core.TokenTypeAnonymous {
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenOBrack,
				Bytes: []byte{'['},
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenStar,
				Bytes: []byte{'*'},
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenCBrack,
				Bytes: []byte{']'},
			})
		} else {
			toks = append(toks, source...)
		}
		toks = append(toks, traversal...)

		return toks, nil

	case core.TokenTypeConditional:
		toks := make(hclwrite.Tokens, 0)
		condition, err := syntaxTokenToHclTokens(*item.Condition, nil)
		if err != nil {
			return nil, err
		}
		trueResult, err := syntaxTokenToHclTokens(*item.TrueResult, nil)
		if err != nil {
			return nil, err
		}
		falseResult, err := syntaxTokenToHclTokens(*item.FalseResult, nil)
		if err != nil {
			return nil, err
		}
		toks = append(toks, condition...)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenQuestion,
			Bytes: []byte(`?`),
		})
		toks = append(toks, trueResult...)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenColon,
			Bytes: []byte(`:`),
		})
		toks = append(toks, falseResult...)
		return toks, nil

	case core.TokenTypeParens:
		toks := make(hclwrite.Tokens, 0)
		content, err := syntaxTokenToHclTokens(*item.Source, nil)
		if err != nil {
			return nil, err
		}

		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenOParen,
			Bytes: []byte(`(`),
		})
		toks = append(toks, content...)
		toks = append(toks, &hclwrite.Token{
			Type:  hclsyntax.TokenCParen,
			Bytes: []byte(`)`),
		})
		return toks, nil

	case core.TokenTypeBinaryOp:
		toks := make(hclwrite.Tokens, 0)
		left, err := syntaxTokenToHclTokens(*item.LeftHandSide, nil)
		if err != nil {
			return nil, err
		}
		right, err := syntaxTokenToHclTokens(*item.RightHandSide, nil)
		if err != nil {
			return nil, err
		}
		op, err := stringOperatorToToken(*item.Operator)

		toks = append(toks, left...)
		toks = append(toks, &hclwrite.Token{
			Type:  op,
			Bytes: []byte(*item.Operator),
		})
		toks = append(toks, right...)
		return toks, nil

	case core.TokenTypeUnaryOp:
		toks := make(hclwrite.Tokens, 0)
		right, err := syntaxTokenToHclTokens(*item.RightHandSide, nil)
		if err != nil {
			return nil, err
		}
		op, err := stringOperatorToToken(*item.Operator)

		toks = append(toks, &hclwrite.Token{
			Type:  op,
			Bytes: []byte(*item.Operator),
		})
		toks = append(toks, right...)
		return toks, nil

	case core.TokenTypeFor:
		toks := make(hclwrite.Tokens, 0)
		if item.ForKeyVar != nil {
			// {for KeyVar, ValVar in CollExpr : KeyExpr => ValExpr if CondExpr}
			collExpr, err := syntaxTokenToHclTokens(*item.ForCollExpr, nil)
			if err != nil {
				return nil, err
			}
			valExpr, err := syntaxTokenToHclTokens(*item.ForValExpr, nil)
			if err != nil {
				return nil, err
			}
			keyExpr, err := syntaxTokenToHclTokens(*item.ForKeyExpr, nil)
			if err != nil {
				return nil, err
			}

			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenOBrace,
				Bytes: []byte(`{`),
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(`for`),
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(*item.ForKeyVar),
			})
			if item.ForValVar != nil {
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenComma,
					Bytes: []byte(`,`),
				})
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenIdent,
					Bytes: []byte(*item.ForValVar),
				})
			}
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(`in`),
			})
			toks = append(toks, collExpr...)
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenColon,
				Bytes: []byte(`:`),
			})
			toks = append(toks, keyExpr...)
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenFatArrow,
				Bytes: []byte(`=>`),
			})
			toks = append(toks, valExpr...)
			if item.ForCondExpr != nil {
				condExpr, err := syntaxTokenToHclTokens(*item.ForCondExpr, nil)
				if err != nil {
					return nil, err
				}
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenIdent,
					Bytes: []byte(`if`),
				})
				toks = append(toks, condExpr...)
			}
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenCBrace,
				Bytes: []byte(`}`),
			})
		} else {
			//[for ValVar in CollExpr : ValExpr if CondExpr]
			collExpr, err := syntaxTokenToHclTokens(*item.ForCollExpr, nil)
			if err != nil {
				return nil, err
			}
			valExpr, err := syntaxTokenToHclTokens(*item.ForValExpr, nil)
			if err != nil {
				return nil, err
			}

			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenOBrack,
				Bytes: []byte(`[`),
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(`for`),
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(*item.ForValVar),
			})
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenIdent,
				Bytes: []byte(`in`),
			})
			toks = append(toks, collExpr...)
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenColon,
				Bytes: []byte(`:`),
			})
			toks = append(toks, valExpr...)
			if item.ForCondExpr != nil {
				condExpr, err := syntaxTokenToHclTokens(*item.ForCondExpr, nil)
				if err != nil {
					return nil, err
				}
				toks = append(toks, &hclwrite.Token{
					Type:  hclsyntax.TokenIdent,
					Bytes: []byte(`if`),
				})
				toks = append(toks, condExpr...)
			}
			toks = append(toks, &hclwrite.Token{
				Type:  hclsyntax.TokenCBrack,
				Bytes: []byte(`]`),
			})
		}
		return toks, nil
	}
	panic("unreachable code")
}

func primitiveToTokens(val interface{}) (hclwrite.Tokens, error) {
	v, err := primitiveToCty(val)
	if err != nil {
		return nil, err
	}
	return appendTokensForValue(v, nil), nil
}

func primitiveToCty(val interface{}) (cty.Value, error) {
	if core.InterfaceIsNil(val) {
		return cty.NilVal, nil
	}
	switch v := val.(type) {
	case string:
		return cty.StringVal(v), nil
	case int:
		return cty.NumberIntVal(int64(v)), nil
	case int8:
		return cty.NumberIntVal(int64(v)), nil
	case int16:
		return cty.NumberIntVal(int64(v)), nil
	case int32:
		return cty.NumberIntVal(int64(v)), nil
	case int64:
		return cty.NumberIntVal(v), nil
	case float64:
		return cty.NumberFloatVal(v), nil
	case float32:
		return cty.NumberFloatVal(float64(v)), nil
	case bool:
		return cty.BoolVal(v), nil
	default:
		return cty.NilVal, errors.New("unkown primitive type")
	}
}

//if isRelative is true the first element will be a TraverseAttr instead of a TraverseRoot
func traversalToTokens(itemTraversal []core.Traverse, isRelative bool) (hclwrite.Tokens, error) {
	traversal := make(hcl.Traversal, 0, len(itemTraversal))
	for i, traverse := range itemTraversal {
		switch traverse.Type {
		case core.TraverseTypeAttr:
			if i == 0 && !isRelative {
				traversal = append(traversal, hcl.TraverseRoot{
					Name: *traverse.Name,
				})
			} else {
				traversal = append(traversal, hcl.TraverseAttr{
					Name: *traverse.Name,
				})
			}
		case core.TraverseTypeIndex:
			index, err := primitiveToCty(traverse.Index)
			if err != nil {
				return nil, err
			}
			traversal = append(traversal, hcl.TraverseIndex{
				Key: index,
			})
		default:
			return nil, errors.New("unsupported traverse type :'" + traverse.Type + "'")
		}
	}
	return appendTokensForTraversal(traversal, nil), nil
}

func stringOperatorToToken(operator string) (hclsyntax.TokenType, error) {
	switch operator {
	case "==":
		return hclsyntax.TokenEqualOp, nil
	case "||":
		return hclsyntax.TokenOr, nil
	case "&&":
		return hclsyntax.TokenAnd, nil
	case "!":
		return hclsyntax.TokenBang, nil
	case "!=":
		return hclsyntax.TokenNotEqual, nil
	case ">":
		return hclsyntax.TokenGreaterThan, nil
	case ">=":
		return hclsyntax.TokenGreaterThanEq, nil
	case "<":
		return hclsyntax.TokenLessThan, nil
	case "<=":
		return hclsyntax.TokenLessThanEq, nil
	case "+":
		return hclsyntax.TokenPlus, nil
	case "-":
		return hclsyntax.TokenMinus, nil
	case "*":
		return hclsyntax.TokenStar, nil
	case "/":
		return hclsyntax.TokenSlash, nil
	case "%":
		return hclsyntax.TokenPercent, nil
	default:
		return hclsyntax.TokenNil, errors.New("unknown operator")
	}
}
