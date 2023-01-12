package hcl_parser

import (
	"barbe/core"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
)

func hclExpressionToSyntaxToken(expr hclsyntax.Expression) (v *core.SyntaxToken, e error) {
	switch mExpr := expr.(type) {
	default:
		return nil, fmt.Errorf("unexpected expression type %T", mExpr)

	case *hclsyntax.ObjectConsKeyExpr:
		return hclExpressionToSyntaxToken(mExpr.Wrapped)
	case *hclsyntax.TemplateWrapExpr:
		return hclExpressionToSyntaxToken(mExpr.Wrapped)

	case *hclsyntax.AnonSymbolExpr:
		return &core.SyntaxToken{
			Type: core.TokenTypeAnonymous,
		}, nil

	case *hclsyntax.SplatExpr:
		source, err := hclExpressionToSyntaxToken(mExpr.Source)
		if err != nil {
			return nil, err
		}
		each, err := hclExpressionToSyntaxToken(mExpr.Each)
		if err != nil {
			return nil, err
		}
		return &core.SyntaxToken{
			Type:      core.TokenTypeSplat,
			Source:    source,
			SplatEach: each,
		}, nil

	case *hclsyntax.ParenthesesExpr:
		source, err := hclExpressionToSyntaxToken(mExpr.Expression)
		if err != nil {
			return nil, err
		}
		return &core.SyntaxToken{
			Type:   core.TokenTypeParens,
			Source: source,
		}, nil

	case *hclsyntax.BinaryOpExpr:
		output := &core.SyntaxToken{
			Type: core.TokenTypeBinaryOp,
		}
		left, err := hclExpressionToSyntaxToken(mExpr.LHS)
		if err != nil {
			return nil, err
		}
		output.LeftHandSide = left
		right, err := hclExpressionToSyntaxToken(mExpr.RHS)
		if err != nil {
			return nil, err
		}
		output.RightHandSide = right
		op, err := operationToStringOperator(mExpr.Op)
		if err != nil {
			return nil, err
		}
		output.Operator = &op
		return output, nil

	case *hclsyntax.UnaryOpExpr:
		output := &core.SyntaxToken{
			Type: core.TokenTypeUnaryOp,
		}
		right, err := hclExpressionToSyntaxToken(mExpr.Val)
		if err != nil {
			return nil, err
		}
		output.RightHandSide = right
		op, err := operationToStringOperator(mExpr.Op)
		if err != nil {
			return nil, err
		}
		output.Operator = &op
		return output, nil

	case *hclsyntax.ConditionalExpr:
		output := &core.SyntaxToken{
			Type: core.TokenTypeConditional,
		}
		condition, err := hclExpressionToSyntaxToken(mExpr.Condition)
		if err != nil {
			return nil, err
		}
		output.Condition = condition
		trueResult, err := hclExpressionToSyntaxToken(mExpr.TrueResult)
		if err != nil {
			return nil, err
		}
		output.TrueResult = trueResult
		falseResult, err := hclExpressionToSyntaxToken(mExpr.FalseResult)
		if err != nil {
			return nil, err
		}
		output.FalseResult = falseResult
		return output, nil

	case *hclsyntax.ForExpr:
		output := &core.SyntaxToken{
			Type:      core.TokenTypeFor,
			ForValVar: &mExpr.ValVar,
		}
		collExpr, err := hclExpressionToSyntaxToken(mExpr.CollExpr)
		if err != nil {
			return nil, err
		}
		output.ForCollExpr = collExpr
		valExpr, err := hclExpressionToSyntaxToken(mExpr.ValExpr)
		if err != nil {
			return nil, err
		}
		output.ForValExpr = valExpr
		if mExpr.KeyVar != "" {
			output.ForKeyVar = &mExpr.KeyVar
		}
		if mExpr.KeyExpr != nil {
			keyExpr, err := hclExpressionToSyntaxToken(mExpr.KeyExpr)
			if err != nil {
				return nil, err
			}
			output.ForKeyExpr = keyExpr
		}
		if mExpr.CondExpr != nil {
			condExpr, err := hclExpressionToSyntaxToken(mExpr.CondExpr)
			if err != nil {
				return nil, err
			}
			output.ForCondExpr = condExpr
		}
		return output, nil

	case *hclsyntax.FunctionCallExpr:
		args := make([]core.SyntaxToken, 0, len(mExpr.Args))
		for _, arg := range mExpr.Args {
			argItem, err := hclExpressionToSyntaxToken(arg)
			if err != nil {
				return nil, err
			}
			args = append(args, *argItem)
		}
		return &core.SyntaxToken{
			Type:         core.TokenTypeFunctionCall,
			FunctionName: &mExpr.Name,
			FunctionArgs: args,
		}, nil

	case *hclsyntax.RelativeTraversalExpr:
		source, err := hclExpressionToSyntaxToken(mExpr.Source)
		if err != nil {
			return nil, err
		}
		traversal, err := parseHclTraversal(mExpr.Traversal)
		if err != nil {
			return nil, err
		}
		return &core.SyntaxToken{
			Type:      core.TokenTypeRelativeTraversal,
			Source:    source,
			Traversal: traversal,
		}, nil

	case *hclsyntax.ScopeTraversalExpr:
		traversal, err := parseHclTraversal(mExpr.Traversal)
		if err != nil {
			return nil, err
		}
		return &core.SyntaxToken{
			Type:      core.TokenTypeScopeTraversal,
			Traversal: traversal,
		}, nil

	case *hclsyntax.IndexExpr:
		collection, err := hclExpressionToSyntaxToken(mExpr.Collection)
		if err != nil {
			return nil, errors.Wrap(err, "indexing(collection)")
		}
		key, err := hclExpressionToSyntaxToken(mExpr.Key)
		if err != nil {
			return nil, errors.Wrap(err, "indexing(key)")
		}
		return &core.SyntaxToken{
			Type:            core.TokenTypeIndexAccess,
			IndexKey:        key,
			IndexCollection: collection,
		}, nil

	case *hclsyntax.TemplateExpr:
		parts := make([]core.SyntaxToken, 0, len(mExpr.Parts))
		for i, part := range mExpr.Parts {
			partItem, err := hclExpressionToSyntaxToken(part)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("template[%d]", i))
			}
			parts = append(parts, *partItem)
		}
		return &core.SyntaxToken{
			Type:  core.TokenTypeTemplate,
			Parts: parts,
		}, nil

	case *hclsyntax.TupleConsExpr:
		parts := make([]core.SyntaxToken, 0)
		for i, part := range mExpr.Exprs {
			partItem, err := hclExpressionToSyntaxToken(part)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("array[%d]", i))
			}
			parts = append(parts, *partItem)
		}
		output := &core.SyntaxToken{
			Type: core.TokenTypeArrayConst,
		}
		if len(parts) != 0 {
			output.ArrayConst = parts
		}
		return output, nil

	case *hclsyntax.ObjectConsExpr:
		objConsts := make([]core.ObjectConstItem, 0)
		for i, pair := range mExpr.Items {
			keyOutput, err := hclExpressionToSyntaxToken(pair.KeyExpr)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("object(key)[%d]", i))
			}
			valueOutput, err := hclExpressionToSyntaxToken(pair.ValueExpr)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("object(value)[%d]", i))
			}

			keyStr, err := core.ExtractAsStringValue(*keyOutput)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("object(key)[%d] (converting to string)", i))
			}

			objConsts = append(objConsts, core.ObjectConstItem{
				Key:   keyStr,
				Value: *valueOutput,
			})
		}
		return &core.SyntaxToken{
			Type:        core.TokenTypeObjectConst,
			ObjectConst: objConsts,
		}, nil
	case *hclsyntax.LiteralValueExpr:
		value, err := convertToPrimitive(mExpr.Val)
		if err != nil {
			return nil, errors.Wrap(err, "literal value")
		}
		return &core.SyntaxToken{
			Type:  core.TokenTypeLiteralValue,
			Value: value,
		}, nil
	}
}

func parseHclTraversal(traversal hcl.Traversal) ([]core.Traverse, error) {
	output := make([]core.Traverse, 0, len(traversal))
	for _, traverse := range traversal {
		switch mTraverse := traverse.(type) {
		case hcl.TraverseAttr:
			output = append(output, core.Traverse{
				Type: core.TraverseTypeAttr,
				Name: core.Ptr(mTraverse.Name),
			})
		case hcl.TraverseRoot:
			output = append(output, core.Traverse{
				Type: core.TraverseTypeAttr,
				Name: core.Ptr(mTraverse.Name),
			})
		case hcl.TraverseIndex:
			t := core.Traverse{
				Type: core.TraverseTypeIndex,
			}

			indexAttrType := mTraverse.Key.Type()
			switch {
			case indexAttrType == cty.String:
				t.Index = mTraverse.Key.AsString()
			case indexAttrType == cty.Number:
				t.Index, _ = mTraverse.Key.AsBigFloat().Int64()
			default:
				return nil, errors.New("traversal index must be a string or number, it's a '" + indexAttrType.FriendlyName() + "'")
			}
			output = append(output, t)
		case hcl.TraverseSplat:
			//TODO
			panic("traverse splat not supported yet")
		}
	}
	return output, nil
}

//if includeLabels is true, the labels are inserted into the body as an attribute
func blockToSyntaxToken(block *hclsyntax.Block, includeLabels bool) (token *core.SyntaxToken, e error) {
	body := block.Body
	m := core.SyntaxToken{
		Type: core.TokenTypeObjectConst,
		Meta: map[string]interface{}{
			"IsBlock": true,
		},
	}
	for _, attr := range body.Attributes {
		syntaxToken, err := hclExpressionToSyntaxToken(attr.Expr)
		if err != nil {
			return nil, err
		}
		m.ObjectConst = append(m.ObjectConst, core.ObjectConstItem{
			Key:   attr.Name,
			Value: *syntaxToken,
		})
	}
	if includeLabels {
		labelsToken := core.SyntaxToken{
			Type:       core.TokenTypeArrayConst,
			ArrayConst: make([]core.SyntaxToken, 0, len(block.Labels)),
		}
		for _, l := range block.Labels {
			labelsToken.ArrayConst = append(labelsToken.ArrayConst, core.SyntaxToken{
				Type:  core.TokenTypeLiteralValue,
				Value: core.Ptr(l),
			})
		}
		m.ObjectConst = append(m.ObjectConst, core.ObjectConstItem{
			Key:   "labels",
			Value: labelsToken,
		})
	}

	subBlocks := map[string][]core.SyntaxToken{}
	for _, subBlock := range body.Blocks {
		subBlockVal, err := blockToSyntaxToken(subBlock, true)
		if err != nil {
			return nil, err
		}
		if v, ok := subBlocks[subBlock.Type]; ok {
			arr := append(v, *subBlockVal)
			subBlocks[subBlock.Type] = arr
		} else {
			subBlocks[subBlock.Type] = []core.SyntaxToken{*subBlockVal}
		}
	}
	for k, v := range subBlocks {
		m.ObjectConst = append(m.ObjectConst, core.ObjectConstItem{
			Key: k,
			Value: core.SyntaxToken{
				Type: core.TokenTypeArrayConst,
				Meta: map[string]interface{}{
					"IsBlock": true,
				},
				ArrayConst: v,
			},
		})
	}

	return &m, nil
}
