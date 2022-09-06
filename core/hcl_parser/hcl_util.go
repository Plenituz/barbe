package hcl_parser

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"reflect"
	"barbe/core"
	"strings"
)

func parseTraversal(traversal hcl.Traversal) []cty.Value {
	output := make([]cty.Value, 0)
	for _, traverse := range traversal {
		switch mTraverse := traverse.(type) {
		case hcl.TraverseAttr:
			output = append(output, cty.ObjectVal(map[string]cty.Value{
				"Type": cty.StringVal(core.TraverseTypeAttr),
				"Name": cty.StringVal(mTraverse.Name),
			}))
		case hcl.TraverseRoot:
			output = append(output, cty.ObjectVal(map[string]cty.Value{
				"Type": cty.StringVal(core.TraverseTypeAttr),
				"Name": cty.StringVal(mTraverse.Name),
			}))
		case hcl.TraverseIndex:
			output = append(output, cty.ObjectVal(map[string]cty.Value{
				"Type":  cty.StringVal(core.TraverseTypeIndex),
				"Index": mTraverse.Key,
			}))
		case hcl.TraverseSplat:
			//TODO
			panic("traverse splat not supported yet")
		}
	}
	return output
}

func expressionToSyntaxToken(attr *hclsyntax.Attribute, expr hclsyntax.Expression) (v cty.Value, e error) {
	switch mExpr := expr.(type) {
	default:
		return cty.NilVal, fmt.Errorf("unexpected expression type %T", mExpr)

	case *hclsyntax.ObjectConsKeyExpr:
		return expressionToSyntaxToken(nil, mExpr.Wrapped)
	case *hclsyntax.TemplateWrapExpr:
		return expressionToSyntaxToken(nil, mExpr.Wrapped)

	case *hclsyntax.AnonSymbolExpr:
		return cty.ObjectVal(map[string]cty.Value{
			"Type": cty.StringVal(core.TokenTypeAnonymous),
		}), nil

	case *hclsyntax.SplatExpr:
		source, err := expressionToSyntaxToken(nil, mExpr.Source)
		if err != nil {
			return cty.NilVal, err
		}
		each, err := expressionToSyntaxToken(nil, mExpr.Each)
		if err != nil {
			return cty.NilVal, err
		}
		output := map[string]cty.Value{
			"Type":      cty.StringVal(core.TokenTypeSplat),
			"Source":    source,
			"SplatEach": each,
		}
		return cty.ObjectVal(output), nil

	case *hclsyntax.ParenthesesExpr:
		source, err := expressionToSyntaxToken(nil, mExpr.Expression)
		if err != nil {
			return cty.NilVal, err
		}
		output := map[string]cty.Value{
			"Type":   cty.StringVal(core.TokenTypeParens),
			"Source": source,
		}
		return cty.ObjectVal(output), nil

	case *hclsyntax.BinaryOpExpr:
		output := map[string]cty.Value{
			"Type": cty.StringVal(core.TokenTypeBinaryOp),
		}
		left, err := expressionToSyntaxToken(nil, mExpr.LHS)
		if err != nil {
			return cty.NilVal, err
		}
		output["LeftHandSide"] = left
		right, err := expressionToSyntaxToken(nil, mExpr.RHS)
		if err != nil {
			return cty.NilVal, err
		}
		output["RightHandSide"] = right
		op, err := operationToStringOperator(mExpr.Op)
		if err != nil {
			return cty.NilVal, err
		}
		output["Operator"] = cty.StringVal(op)
		return cty.ObjectVal(output), nil

	case *hclsyntax.UnaryOpExpr:
		output := map[string]cty.Value{
			"Type": cty.StringVal(core.TokenTypeUnaryOp),
		}
		right, err := expressionToSyntaxToken(nil, mExpr.Val)
		if err != nil {
			return cty.NilVal, err
		}
		output["RightHandSide"] = right
		op, err := operationToStringOperator(mExpr.Op)
		if err != nil {
			return cty.NilVal, err
		}
		output["Operator"] = cty.StringVal(op)
		return cty.ObjectVal(output), nil

	case *hclsyntax.ConditionalExpr:
		output := map[string]cty.Value{
			"Type": cty.StringVal(core.TokenTypeConditional),
		}
		condition, err := expressionToSyntaxToken(nil, mExpr.Condition)
		if err != nil {
			return cty.NilVal, err
		}
		output["Condition"] = condition
		trueResult, err := expressionToSyntaxToken(nil, mExpr.TrueResult)
		if err != nil {
			return cty.NilVal, err
		}
		output["TrueResult"] = trueResult
		falseResult, err := expressionToSyntaxToken(nil, mExpr.FalseResult)
		if err != nil {
			return cty.NilVal, err
		}
		output["FalseResult"] = falseResult
		return cty.ObjectVal(output), nil
	case *hclsyntax.ForExpr:
		output := map[string]cty.Value{
			"Type":      cty.StringVal(core.TokenTypeFor),
			"ForValVar": cty.StringVal(mExpr.ValVar),
		}
		collExpr, err := expressionToSyntaxToken(nil, mExpr.CollExpr)
		if err != nil {
			return cty.NilVal, err
		}
		output["ForCollExpr"] = collExpr
		valExpr, err := expressionToSyntaxToken(nil, mExpr.ValExpr)
		if err != nil {
			return cty.NilVal, err
		}
		output["ForValExpr"] = valExpr
		if mExpr.KeyVar != "" {
			output["ForKeyVar"] = cty.StringVal(mExpr.KeyVar)
		}
		if mExpr.KeyExpr != nil {
			keyExpr, err := expressionToSyntaxToken(nil, mExpr.KeyExpr)
			if err != nil {

				return cty.NilVal, err
			}
			output["ForKeyExpr"] = keyExpr
		}
		if mExpr.CondExpr != nil {
			condExpr, err := expressionToSyntaxToken(nil, mExpr.CondExpr)
			if err != nil {
				return cty.NilVal, err
			}
			output["ForCondExpr"] = condExpr
		}
		return cty.ObjectVal(output), nil

	case *hclsyntax.FunctionCallExpr:
		args := make([]cty.Value, 0)
		for _, arg := range mExpr.Args {
			argItem, err := expressionToSyntaxToken(nil, arg)
			if err != nil {
				return cty.NilVal, err
			}
			args = append(args, argItem)
		}
		output := map[string]cty.Value{
			"Type":         cty.StringVal(core.TokenTypeFunctionCall),
			"FunctionName": cty.StringVal(mExpr.Name),
			"FunctionArgs": cty.TupleVal(args),
		}
		return cty.ObjectVal(output), nil

	case *hclsyntax.RelativeTraversalExpr:
		source, err := expressionToSyntaxToken(nil, mExpr.Source)
		if err != nil {
			return cty.NilVal, err
		}
		return cty.ObjectVal(map[string]cty.Value{
			"Type":      cty.StringVal(core.TokenTypeRelativeTraversal),
			"Source":    source,
			"Traversal": cty.TupleVal(parseTraversal(mExpr.Traversal)),
		}), nil

	case *hclsyntax.ScopeTraversalExpr:
		return cty.ObjectVal(map[string]cty.Value{
			"Type":      cty.StringVal(core.TokenTypeScopeTraversal),
			"Traversal": cty.TupleVal(parseTraversal(mExpr.Traversal)),
		}), nil

	case *hclsyntax.IndexExpr:
		collection, err := expressionToSyntaxToken(attr, mExpr.Collection)
		if err != nil {
			return cty.NilVal, errors.Wrap(err, "indexing(collection)")
		}
		key, err := expressionToSyntaxToken(attr, mExpr.Key)
		if err != nil {
			return cty.NilVal, errors.Wrap(err, "indexing(key)")
		}
		return cty.ObjectVal(map[string]cty.Value{
			"Type":            cty.StringVal(core.TokenTypeIndexAccess),
			"IndexKey":        key,
			"IndexCollection": collection,
		}), nil

	case *hclsyntax.TemplateExpr:
		parts := make([]cty.Value, 0)
		for i, part := range mExpr.Parts {
			partItem, err := expressionToSyntaxToken(nil, part)
			if err != nil {
				return cty.NilVal, errors.Wrap(err, fmt.Sprintf("template[%d]", i))
			}
			parts = append(parts, partItem)
		}
		return cty.ObjectVal(map[string]cty.Value{
			"Type":  cty.StringVal(core.TokenTypeTemplate),
			"Parts": cty.TupleVal(parts),
		}), nil

	case *hclsyntax.TupleConsExpr:
		parts := make([]cty.Value, 0)
		for i, part := range mExpr.Exprs {
			partItem, err := expressionToSyntaxToken(nil, part)
			if err != nil {
				return cty.NilVal, errors.Wrap(err, fmt.Sprintf("array[%d]", i))
			}
			parts = append(parts, partItem)
		}
		output := map[string]cty.Value{
			"Type": cty.StringVal(core.TokenTypeArrayConst),
		}
		if len(parts) != 0 {
			output["ArrayConst"] = cty.TupleVal(parts)
		}
		return cty.ObjectVal(output), nil

	case *hclsyntax.ObjectConsExpr:
		objConsts := make([]cty.Value, 0)
		for i, pair := range mExpr.Items {
			keyOutput, err := expressionToSyntaxToken(nil, pair.KeyExpr)
			if err != nil {
				return cty.NilVal, errors.Wrap(err, fmt.Sprintf("object(key)[%d]", i))
			}
			valueOutput, err := expressionToSyntaxToken(nil, pair.ValueExpr)
			if err != nil {
				return cty.NilVal, errors.Wrap(err, fmt.Sprintf("object(value)[%d]", i))
			}

			objConsts = append(objConsts, cty.ObjectVal(map[string]cty.Value{
				"Key":   keyOutput,
				"Value": valueOutput,
			}))
		}
		return cty.ObjectVal(map[string]cty.Value{
			"Type":        cty.StringVal(core.TokenTypeObjectConst),
			"ObjectConst": cty.TupleVal(objConsts),
		}), nil
	case *hclsyntax.LiteralValueExpr:
		return cty.ObjectVal(map[string]cty.Value{
			"Type":  cty.StringVal(core.TokenTypeLiteralValue),
			"Value": mExpr.Val,
		}), nil
	}
}

func blockToCtyValue(block *hclsyntax.Block, includeLabels bool) (cty.Value, error) {
	body := block.Body
	m := map[string]cty.Value{}
	for _, attr := range body.Attributes {
		syntaxToken, err := expressionToSyntaxToken(attr, attr.Expr)
		if err != nil {
			return cty.NilVal, err
		}
		m[attr.Name] = syntaxToken
	}
	if includeLabels {
		labels := make([]cty.Value, 0)
		for _, l := range block.Labels {
			labels = append(labels, cty.StringVal(l))
		}
		m["labels"] = cty.TupleVal(labels)
	}

	for _, subBlock := range body.Blocks {
		subBlockVal, err := blockToCtyValue(subBlock, true)
		if err != nil {
			return cty.NilVal, err
		}
		if v, ok := m[subBlock.Type]; ok {
			arr := append(v.AsValueSlice(), subBlockVal)
			m[subBlock.Type] = cty.TupleVal(arr)
		} else {
			m[subBlock.Type] = cty.TupleVal([]cty.Value{subBlockVal})
		}
	}

	return cty.ObjectVal(m), nil
}

func appendIfMissing(slice []cty.Value, element cty.Value) ([]cty.Value, error) {
	for _, ele := range slice {
		eq, err := stdlib.Equal(ele, element)
		if err != nil {
			return slice, err
		}
		if eq.True() {
			return slice, nil
		}
	}
	return append(slice, element), nil
}

func DataBagToEvalContext(databag *core.DataBag, others []*core.DataBag) (*hcl.EvalContext, error) {
	ctxt := map[string]cty.Value{}
	if databag != nil {
		ctyValue, err := MarshalSyntaxToken(databag.Value)
		if err != nil {
			return nil, err
		}
		ctxt["block"] = ctyValue
	}

	ctyOthers := make([]cty.Value, 0, len(others))
	for _, other := range others {
		obj := map[string]cty.Value{
			"type": cty.StringVal(other.Type),
		}
		body, err := MarshalSyntaxToken(other.Value)
		if err != nil {
			return nil, errors.Wrap(err, "error converting block '"+other.Type+"' to cty.Value")
		}
		obj["body"] = body

		//labels := make([]cty.Value, 0)
		//for _, l := range other.Labels {
		//	labels = append(labels, cty.StringVal(l))
		//}
		//obj["labels"] = cty.TupleVal(labels)

		ctyOthers = append(ctyOthers, cty.ObjectVal(obj))
	}
	ctxt["others"] = cty.TupleVal(ctyOthers)

	return &hcl.EvalContext{
		Variables: ctxt,
	}, nil
}

func BlockToEvalContext(block *hclsyntax.Block, others []*hclsyntax.Block) (*hcl.EvalContext, error) {
	ctxt := map[string]cty.Value{}

	if block != nil {
		ctyValue, err := blockToCtyValue(block, true)
		if err != nil {
			return nil, err
		}
		ctxt["block"] = ctyValue
	}

	ctyOthers := make([]cty.Value, 0, len(others))
	for _, other := range others {
		obj := map[string]cty.Value{
			"type": cty.StringVal(other.Type),
		}
		body, err := blockToCtyValue(other, false)
		if err != nil {
			return nil, errors.Wrap(err, "error converting block '"+other.Type+"' to cty.Value")
		}
		obj["body"] = body

		labels := make([]cty.Value, 0)
		for _, l := range other.Labels {
			labels = append(labels, cty.StringVal(l))
		}
		obj["labels"] = cty.TupleVal(labels)

		ctyOthers = append(ctyOthers, cty.ObjectVal(obj))
	}
	ctxt["others"] = cty.TupleVal(ctyOthers)

	return &hcl.EvalContext{
		Variables: ctxt,
		//https://github.com/hashicorp/terraform/blob/d35bc05312/internal/lang/functions.go
		Functions: map[string]function.Function{
			"debug": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "arr",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					fmt.Println("hello")
					return args[0], nil
				},
			}),
			"concatarr": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "arr",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					out := make([]cty.Value, 0)
					for _, list := range args[0].AsValueSlice() {
						if list.IsNull() {
							continue
						}
						if list.Type().IsTupleType() {
							for _, item := range list.AsValueSlice() {
								if item.IsNull() {
									continue
								}
								out = append(out, item)
							}
						} else {
							out = append(out, list)
						}
					}
					return cty.TupleVal(out), nil
				},
			}),
			"to_template": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					values := args[0].AsValueSlice()
					parts := make([]core.SyntaxToken, 0, len(values))
					for _, val := range values {
						item, err := UnmarshalSyntaxToken(val)
						if err != nil {
							return cty.NilVal, errors.Wrap(err, "error unmarshaling array item as data item")
						}
						parts = append(parts, item)
					}
					syntaxToken := core.SyntaxToken{
						Type:  core.TokenTypeTemplate,
						Parts: parts,
					}
					return MarshalSyntaxToken(syntaxToken)
				},
			}),
			"append_to_traversal": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "source",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
					{
						Name:             "toAdd",
						Type:             cty.String,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					source, err := UnmarshalSyntaxToken(args[0])
					if err != nil {
						return cty.NilVal, errors.Wrap(err, "error parsing first argument of append_to_traversal as a traversal")
					}
					if source.Type != core.TokenTypeScopeTraversal && source.Type != core.TokenTypeRelativeTraversal {
						return cty.NilVal, errors.New("first argument of append_to_traversal must be a traversal")
					}

					templateStr := args[1].AsString()
					split := strings.Split(templateStr, ".")
					for _, str := range split {
						//TODO add support for indexing
						source.Traversal = append(source.Traversal, core.Traverse{
							Type: core.TraverseTypeAttr,
							Name: s(str),
						})
					}
					return MarshalSyntaxToken(source)
				},
			}),
			"to_traversal": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.String,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					templateStr := args[0].AsString()
					split := strings.Split(templateStr, ".")
					traverse := make([]core.Traverse, 0, len(split))
					for _, str := range split {
						//TODO add support for indexing
						traverse = append(traverse, core.Traverse{
							Type: core.TraverseTypeAttr,
							Name: s(str),
						})
					}
					syntaxToken := core.SyntaxToken{
						Type:      core.TokenTypeScopeTraversal,
						Traversal: traverse,
					}
					return MarshalSyntaxToken(syntaxToken)
				},
			}),
			"to_func_call": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "funcName",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				VarParam: &function.Parameter{
					Name:             "args",
					Type:             cty.DynamicPseudoType,
					AllowDynamicType: true,
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					funcNameI, err := UnmarshalSyntaxToken(args[0])
					if err != nil {
						return cty.NilVal, errors.Wrap(err, "error parsing funcName as data item")
					}
					funcName, err := core.ExtractAsStringValue(funcNameI)
					if err != nil {
						return cty.NilVal, errors.Wrap(err, "error reading funcName as string value")
					}

					leftOverArgs := args[1:]
					funcArgs := make([]core.SyntaxToken, 0, len(leftOverArgs))
					for _, val := range leftOverArgs {
						item, err := UnmarshalSyntaxToken(val)
						if err != nil {
							return cty.NilVal, errors.Wrap(err, "error unmarshaling funcArg as data item")
						}
						funcArgs = append(funcArgs, item)
					}

					syntaxToken := core.SyntaxToken{
						Type:         core.TokenTypeFunctionCall,
						FunctionName: &funcName,
						FunctionArgs: funcArgs,
					}
					return MarshalSyntaxToken(syntaxToken)
				},
			}),
			"type": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					item, err := UnmarshalSyntaxToken(args[0])
					if err != nil {
						return cty.NilVal, err
					}
					switch item.Type {
					case core.TokenTypeLiteralValue:
						return cty.StringVal(reflect.TypeOf(item.Value).String()), nil
					case core.TokenTypeArrayConst:
						return cty.StringVal("array"), nil
					case core.TokenTypeObjectConst:
						return cty.StringVal("object"), nil
					default:
						return cty.StringVal("unknown"), nil
					}
				},
			}),
			"pass": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
						AllowNull:        true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					return args[0], nil
				},
			}),
			"val": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					item, err := UnmarshalSyntaxToken(args[0])
					if err != nil {
						return cty.NilVal, err
					}
					switch item.Type {
					case core.TokenTypeTemplate:
						str, err := core.ExtractAsStringValue(item)
						if err != nil {
							return cty.NilVal, errors.Wrap(err, "error extracting value from template literal")
						}
						return cty.StringVal(str), nil
					case core.TokenTypeLiteralValue:
						return args[0].AsValueMap()["Value"], nil
					case core.TokenTypeArrayConst:
						return args[0].AsValueMap()["ArrayConst"], nil
					case core.TokenTypeObjectConst:
						v := map[string]cty.Value{}
						for _, objConst := range item.ObjectConst {
							v[objConst.Key], err = MarshalSyntaxToken(objConst.Value)
							if err != nil {
								return cty.NilVal, errors.Wrap(err, "error converting object const value for key '"+objConst.Key+"'")
							}
						}
						return cty.ObjectVal(v), nil
					}
					return cty.NilVal, errors.New("not a known value")
				},
			}),
			"length": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "value",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
						AllowUnknown:     true,
						AllowMarked:      true,
					},
				},
				Type: func(args []cty.Value) (cty.Type, error) {
					collTy := args[0].Type()
					switch {
					case collTy == cty.String || collTy.IsTupleType() || collTy.IsObjectType() || collTy.IsListType() || collTy.IsMapType() || collTy.IsSetType() || collTy == cty.DynamicPseudoType:
						return cty.Number, nil
					default:
						return cty.Number, errors.New("argument must be a string, a collection type, or a structural type")
					}
				},
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					coll := args[0]
					collTy := args[0].Type()
					marks := coll.Marks()
					switch {
					case collTy == cty.DynamicPseudoType:
						return cty.UnknownVal(cty.Number).WithMarks(marks), nil
					case collTy.IsTupleType():
						l := len(collTy.TupleElementTypes())
						return cty.NumberIntVal(int64(l)).WithMarks(marks), nil
					case collTy.IsObjectType():
						l := len(collTy.AttributeTypes())
						return cty.NumberIntVal(int64(l)).WithMarks(marks), nil
					case collTy == cty.String:
						// We'll delegate to the cty stdlib strlen function here, because
						// it deals with all of the complexities of tokenizing unicode
						// grapheme clusters.
						return stdlib.Strlen(coll)
					case collTy.IsListType() || collTy.IsSetType() || collTy.IsMapType():
						return coll.Length(), nil
					default:
						// Should never happen, because of the checks in our Type func above
						return cty.UnknownVal(cty.Number), errors.New("impossible value type for length(...)")
					}
				},
			}),
			"as_block": function.New(&function.Spec{
				VarParam: &function.Parameter{
					Name:             "block",
					Type:             cty.DynamicPseudoType,
					AllowDynamicType: true,
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					output := make([]core.SyntaxToken, 0, len(args))
					for _, arg := range args {
						item, err := UnmarshalSyntaxToken(arg)
						if err != nil {
							return cty.NilVal, err
						}
						if len(item.Meta) == 0 {
							item.Meta = map[string]interface{}{
								"IsBlock": true,
							}
						} else {
							item.Meta["IsBlock"] = true
						}

						output = append(output, item)
					}
					if len(output) == 1 {
						return MarshalSyntaxToken(output[0])
					}
					return MarshalSyntaxToken(core.SyntaxToken{
						Type: core.TokenTypeArrayConst,
						Meta: map[string]interface{}{
							"IsBlock": true,
						},
						ArrayConst: output,
					})
				},
			}),
			"traversal_as_str": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					item, err := UnmarshalSyntaxToken(args[0])
					if err != nil {
						return cty.NilVal, err
					}
					if item.Type != core.TokenTypeRelativeTraversal && item.Type != core.TokenTypeScopeTraversal {
						return cty.NilVal, errors.New("input of traversal_as_str is not a traversal")
					}

					str := ""
					for i, t := range item.Traversal {
						if i > 0 {
							str += "."
						}
						switch t.Type {
						case core.TraverseTypeAttr:
							str += *t.Name
						case core.TraverseTypeIndex:
							str += fmt.Sprintf("[%v]", t.Index)
						default:
							return cty.NilVal, fmt.Errorf("unknown traversal type %v", t.Type)
						}
					}
					return cty.StringVal(str), nil
				},
			}),
			"as_str": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "val",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					str, err := extractStringValueCty(args[0])
					if err != nil {
						return cty.NilVal, errors.Wrap(err, "[as_str]")
					}
					return cty.StringVal(str), nil
				},
			}),
			"first_not_null": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "arr",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					in := args[0]
					for _, item := range in.AsValueSlice() {
						if item.IsNull() {
							continue
						}
						return item, nil
					}
					return cty.NilVal, nil
				},
			}),
			"has_attr": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name:             "obj",
						Type:             cty.DynamicPseudoType,
						AllowDynamicType: true,
					},
					{
						Name: "attrName",
						Type: cty.String,
					},
				},
				Type: function.StaticReturnType(cty.Bool),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					in := args[0]
					attrName := args[1]
					if in.IsNull() {
						return cty.BoolVal(false), nil
					}
					if !in.Type().IsMapType() && !in.Type().IsObjectType() {
						return cty.BoolVal(false), nil
					}
					return cty.BoolVal(in.Type().HasAttribute(attrName.AsString())), nil
				},
			}),
			//TODO
			//"abspath":      funcs.AbsPathFunc,
			//"alltrue":      funcs.AllTrueFunc,
			//"anytrue":      funcs.AnyTrueFunc,
			//"basename":     funcs.BasenameFunc,
			//"base64decode": funcs.Base64DecodeFunc,
			//"base64encode": funcs.Base64EncodeFunc,
			//"base64gzip":   funcs.Base64GzipFunc,
			//"base64sha256": funcs.Base64Sha256Func,
			//"base64sha512": funcs.Base64Sha512Func,
			//"bcrypt":       funcs.BcryptFunc,
			//"cidrhost":     funcs.CidrHostFunc,
			//"cidrnetmask":  funcs.CidrNetmaskFunc,
			//"cidrsubnet":   funcs.CidrSubnetFunc,
			//"cidrsubnets":  funcs.CidrSubnetsFunc,
			//"coalesce":     funcs.CoalesceFunc,
			////"defaults":         s.experimentalFunction(experiments.ModuleVariableOptionalAttrs, funcs.DefaultsFunc),
			//"dirname":          funcs.DirnameFunc,
			//"file":             funcs.MakeFileFunc(s.BaseDir, false),
			//"fileexists":       funcs.MakeFileExistsFunc(s.BaseDir),
			//"fileset":          funcs.MakeFileSetFunc(s.BaseDir),
			//"filebase64":       funcs.MakeFileFunc(s.BaseDir, true),
			//"filebase64sha256": funcs.MakeFileBase64Sha256Func(s.BaseDir),
			//"filebase64sha512": funcs.MakeFileBase64Sha512Func(s.BaseDir),
			//"filemd5":          funcs.MakeFileMd5Func(s.BaseDir),
			//"filesha1":         funcs.MakeFileSha1Func(s.BaseDir),
			//"filesha256":       funcs.MakeFileSha256Func(s.BaseDir),
			//"filesha512":       funcs.MakeFileSha512Func(s.BaseDir),
			//"lookup":           funcs.LookupFunc,
			//"length":           funcs.LengthFunc,
			//"list":             funcs.ListFunc,
			//"map":              funcs.MapFunc,
			//"matchkeys":        funcs.MatchkeysFunc,
			//"index":            funcs.IndexFunc, // stdlib.IndexFunc is not compatible
			//"md5":              funcs.Md5Func,
			//"one":              funcs.OneFunc,
			//"pathexpand":       funcs.PathExpandFunc,
			//"replace":          funcs.ReplaceFunc,
			//"rsadecrypt":       funcs.RsaDecryptFunc,
			//"sensitive":        funcs.SensitiveFunc,
			//"nonsensitive":     funcs.NonsensitiveFunc,
			//"sha1":             funcs.Sha1Func,
			//"sha256":           funcs.Sha256Func,
			//"sha512":           funcs.Sha512Func,
			//"sum":              funcs.SumFunc,
			//"textdecodebase64": funcs.TextDecodeBase64Func,
			//"textencodebase64": funcs.TextEncodeBase64Func,
			//"timestamp":        funcs.TimestampFunc,
			//"tostring":         funcs.MakeToFunc(cty.String),
			//"tonumber":         funcs.MakeToFunc(cty.Number),
			//"tobool":           funcs.MakeToFunc(cty.Bool),
			//"toset":            funcs.MakeToFunc(cty.Set(cty.DynamicPseudoType)),
			//"tolist":           funcs.MakeToFunc(cty.List(cty.DynamicPseudoType)),
			//"tomap":            funcs.MakeToFunc(cty.Map(cty.DynamicPseudoType)),
			//"transpose":        funcs.TransposeFunc,
			//"urlencode":        funcs.URLEncodeFunc,
			//"uuid":             funcs.UUIDFunc,
			//"uuidv5":           funcs.UUIDV5Func,
			//"yamldecode":       ctyyaml.YAMLDecodeFunc,
			//"yamlencode":       ctyyaml.YAMLEncodeFunc,
			"abs":          stdlib.AbsoluteFunc,
			"can":          tryfunc.CanFunc,
			"ceil":         stdlib.CeilFunc,
			"chomp":        stdlib.ChompFunc,
			"coalescelist": stdlib.CoalesceListFunc,
			"compact":      stdlib.CompactFunc,
			"concat":       stdlib.ConcatFunc,
			"contains":     stdlib.ContainsFunc,
			"csvdecode":    stdlib.CSVDecodeFunc,
			"distinct":     stdlib.DistinctFunc,
			"distinctarr": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name: "list",
						Type: cty.DynamicPseudoType,
					},
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (ret cty.Value, err error) {

					listVal := args[0]

					if !listVal.IsWhollyKnown() {
						return cty.UnknownVal(retType), nil
					}
					var list []cty.Value

					for it := listVal.ElementIterator(); it.Next(); {
						_, v := it.Element()
						list, err = appendIfMissing(list, v)
						if err != nil {
							return cty.NilVal, err
						}
					}

					if len(list) == 0 {
						return cty.ListValEmpty(retType.ElementType()), nil
					}
					return cty.TupleVal(list), nil
				},
			}),
			"element":         stdlib.ElementFunc,
			"chunklist":       stdlib.ChunklistFunc,
			"flatten":         stdlib.FlattenFunc,
			"floor":           stdlib.FloorFunc,
			"format":          stdlib.FormatFunc,
			"formatdate":      stdlib.FormatDateFunc,
			"formatlist":      stdlib.FormatListFunc,
			"indent":          stdlib.IndentFunc,
			"join":            stdlib.JoinFunc,
			"jsondecode":      stdlib.JSONDecodeFunc,
			"jsonencode":      stdlib.JSONEncodeFunc,
			"keys":            stdlib.KeysFunc,
			"log":             stdlib.LogFunc,
			"lower":           stdlib.LowerFunc,
			"max":             stdlib.MaxFunc,
			"merge":           stdlib.MergeFunc,
			"min":             stdlib.MinFunc,
			"parseint":        stdlib.ParseIntFunc,
			"pow":             stdlib.PowFunc,
			"range":           stdlib.RangeFunc,
			"regex":           stdlib.RegexFunc,
			"regexall":        stdlib.RegexAllFunc,
			"reverse":         stdlib.ReverseListFunc,
			"setintersection": stdlib.SetIntersectionFunc,
			"setproduct":      stdlib.SetProductFunc,
			"setsubtract":     stdlib.SetSubtractFunc,
			"setunion":        stdlib.SetUnionFunc,
			"signum":          stdlib.SignumFunc,
			"slice":           stdlib.SliceFunc,
			"sort":            stdlib.SortFunc,
			"split":           stdlib.SplitFunc,
			"strrev":          stdlib.ReverseFunc,
			"substr":          stdlib.SubstrFunc,
			"timeadd":         stdlib.TimeAddFunc,
			"title":           stdlib.TitleFunc,
			"trim":            stdlib.TrimFunc,
			"trimprefix":      stdlib.TrimPrefixFunc,
			"trimspace":       stdlib.TrimSpaceFunc,
			"trimsuffix":      stdlib.TrimSuffixFunc,
			"try":             tryfunc.TryFunc,
			"upper":           stdlib.UpperFunc,
			"values":          stdlib.ValuesFunc,
			"zipmap":          stdlib.ZipmapFunc,
		},
	}, nil
}

func operationToStringOperator(op *hclsyntax.Operation) (string, error) {
	switch op {
	case hclsyntax.OpEqual:
		return "==", nil
	case hclsyntax.OpLogicalOr:
		return "||", nil
	case hclsyntax.OpLogicalAnd:
		return "&&", nil
	case hclsyntax.OpLogicalNot:
		return "!", nil
	case hclsyntax.OpNotEqual:
		return "!=", nil
	case hclsyntax.OpGreaterThan:
		return ">", nil
	case hclsyntax.OpGreaterThanOrEqual:
		return ">=", nil
	case hclsyntax.OpLessThan:
		return "<", nil
	case hclsyntax.OpLessThanOrEqual:
		return "<=", nil
	case hclsyntax.OpAdd:
		return "+", nil
	case hclsyntax.OpSubtract:
		return "-", nil
	case hclsyntax.OpMultiply:
		return "*", nil
	case hclsyntax.OpDivide:
		return "/", nil
	case hclsyntax.OpModulo:
		return "%", nil
	case hclsyntax.OpNegate:
		return "-", nil
	default:
		return "", errors.New("unknown operator")
	}
}

func mergeAttributeValues(existingValue hclsyntax.Expression, newValue hclsyntax.Expression) (hclsyntax.Expression, error) {
	switch existingExpr := existingValue.(type) {
	default:
		return existingValue, fmt.Errorf("unexpected expression type %T while merging", existingExpr)

	case *hclsyntax.AnonSymbolExpr, *hclsyntax.SplatExpr,
		*hclsyntax.BinaryOpExpr, *hclsyntax.UnaryOpExpr,
		*hclsyntax.ConditionalExpr, *hclsyntax.ForExpr,
		*hclsyntax.FunctionCallExpr, *hclsyntax.ScopeTraversalExpr,
		*hclsyntax.IndexExpr, *hclsyntax.TemplateExpr,
		*hclsyntax.LiteralValueExpr, *hclsyntax.ParenthesesExpr:
		fmt.Println(fmt.Sprintf("cannot merge %T returning existing expr", existingExpr))
		return existingExpr, nil

	case *hclsyntax.ObjectConsKeyExpr:
		return mergeAttributeValues(existingExpr.Wrapped, newValue)

	case *hclsyntax.TupleConsExpr:
		switch newValue.(type) {
		case *hclsyntax.TupleConsExpr:
		default:
			return nil, fmt.Errorf("cannot merge a tuple const with a %T", newValue)
		}
		exprs := existingExpr.Exprs[:]
		for _, newExpr := range newValue.(*hclsyntax.TupleConsExpr).Exprs {
			exprs = append(exprs, newExpr)
		}
		newExpr := *existingExpr
		newExpr.Exprs = exprs
		return &newExpr, nil

	case *hclsyntax.ObjectConsExpr:
		switch newValue.(type) {
		case *hclsyntax.ObjectConsExpr:
		default:
			return nil, fmt.Errorf("cannot merge an object const with a %T", newValue)
		}
		existingMap := map[string]struct{}{}
		for _, pair := range existingExpr.Items {
			key, err := extractStringValueExpr(pair.KeyExpr)
			if err != nil {
				return nil, errors.Wrap(err, "key of map couldnt be interpreted as string")
			}
			existingMap[key] = struct{}{}
		}

		for _, newPair := range newValue.(*hclsyntax.ObjectConsExpr).Items {
			key, err := extractStringValueExpr(newPair.KeyExpr)
			if err != nil {
				return nil, errors.Wrap(err, "key of incoming map couldnt be interpreted as string")
			}
			if _, ok := existingMap[key]; ok {
				continue
			}
			existingExpr.Items = append(existingExpr.Items, newPair)
			existingMap[key] = struct{}{}
		}
	}
	return existingValue, nil
}

func extractStringValueExpr(expr hclsyntax.Expression) (string, error) {
	//TODO avoid doing an unmarshal here and just return the string from the hcl expr
	v, err := expressionToSyntaxToken(nil, expr)
	if err != nil {
		return "", err
	}
	return extractStringValueCty(v)
}
