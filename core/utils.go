package core

import (
	"context"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"reflect"
)

func ExtractAsStringValue(read SyntaxToken) (string, error) {
	switch read.Type {
	case TokenTypeScopeTraversal:
		str := ""
		for i, traverse := range read.Traversal {
			if traverse.Type != TraverseTypeAttr {
				return "", fmt.Errorf("invalid traversal type for key")
			}
			if i != 0 {
				str += "."
			}
			str += *traverse.Name
		}
		return str, nil
	case TokenTypeLiteralValue:
		return fmt.Sprintf("%v", read.Value), nil
	case TokenTypeTemplate:
		str := ""
		for _, part := range read.Parts {
			partStr, err := ExtractAsStringValue(part)
			if err != nil {
				return "", err
			}
			str += partStr
		}
		return str, nil
	default:
		return "", fmt.Errorf("unexpected type for string extraction: %s", read.Type)
	}
}

func GetMetaBool(token SyntaxToken, key string) bool {
	if token.Meta == nil {
		return false
	}
	if maybeBool, ok := token.Meta[key]; ok {
		if definitelyBool, ok := maybeBool.(bool); ok {
			return definitelyBool
		}
	}
	return false
}

func GetMeta[T any](token SyntaxToken, key string) T {
	var noop T
	if token.Meta == nil {
		return noop
	}
	if maybe, ok := token.Meta[key]; ok {
		if definitely, ok := maybe.(T); ok {
			return definitely
		}
	}
	return noop
}

func GetObjectKeyValues(key string, pairs []ObjectConstItem) []SyntaxToken {
	return GetObjectKeysValues(map[string]struct{}{key: {}}, pairs)
}

//GetObjectKeysValues returns the values for all the given keys in the map
func GetObjectKeysValues(keys map[string]struct{}, pairs []ObjectConstItem) []SyntaxToken {
	tokens := make([]SyntaxToken, 0)
	for _, pair := range pairs {
		if _, ok := keys[pair.Key]; !ok {
			continue
		}
		tokens = append(tokens, pair.Value)
	}
	return tokens
}

//TODO make this function return partial result when there is an error
func DecodeValue(v interface{}) (SyntaxToken, error) {
	if InterfaceIsNil(v) {
		return SyntaxToken{
			Type:  TokenTypeLiteralValue,
			Value: nil,
		}, nil
	}
	rVal := reflect.ValueOf(v)
	switch rVal.Type().Kind() {
	default:
		return SyntaxToken{}, errors.New("cannot decode value of type " + rVal.Type().Kind().String())
	case reflect.Interface, reflect.Ptr:
		return DecodeValue(rVal.Elem().Interface())
	case reflect.Bool,
		reflect.String,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128:
		return SyntaxToken{
			Type:  TokenTypeLiteralValue,
			Value: v,
		}, nil

	case reflect.Array, reflect.Slice:
		output := SyntaxToken{
			Type:       TokenTypeArrayConst,
			ArrayConst: make([]SyntaxToken, 0, rVal.Len()),
		}

		for i := 0; i < rVal.Len(); i++ {
			item, err := DecodeValue(rVal.Index(i).Interface())
			if err != nil {
				return SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding index %v of array", i))
			}
			output.ArrayConst = append(output.ArrayConst, item)
		}
		return output, nil

	case reflect.Map:
		if rVal.MapIndex(reflect.ValueOf("Type")).IsValid() {
			break
		}

		output := SyntaxToken{
			Type:        TokenTypeObjectConst,
			ObjectConst: make([]ObjectConstItem, 0, rVal.Len()),
		}
		iter := rVal.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()

			if v.IsNil() || !v.IsValid() {
				continue
			}
			if k.Kind() != reflect.String {
				return SyntaxToken{}, errors.New("map key must be string")
			}
			item, err := DecodeValue(v.Interface())
			if err != nil {
				return SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding map value for key %v", k.String()))
			}
			output.ObjectConst = append(output.ObjectConst, ObjectConstItem{
				Key:   k.String(),
				Value: item,
			})
		}
		return output, nil
	case reflect.Struct:
		if rVal.FieldByName("Type").IsValid() {
			break
		}
		output := SyntaxToken{
			Type:        TokenTypeObjectConst,
			ObjectConst: make([]ObjectConstItem, 0, rVal.Len()),
		}
		fields := make([]string, 0, rVal.NumField())
		rVal.FieldByNameFunc(func(s string) bool {
			fields = append(fields, s)
			return false
		})
		for _, fieldName := range fields {
			field := rVal.FieldByName(fieldName)
			if field.IsNil() || !field.IsValid() {
				continue
			}
			item, err := DecodeValue(field.Interface())
			if err != nil {
				return SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding struct value for field %v", fieldName))
			}
			output.ObjectConst = append(output.ObjectConst, ObjectConstItem{
				Key:   fieldName,
				Value: item,
			})
		}
		return output, nil
	}

	var item SyntaxToken
	err := mapstructure.Decode(v, &item)
	if err != nil {
		return SyntaxToken{}, errors.Wrap(err, "error decoding syntax token from template")
	}
	return item, nil
}

type Visitor = func(token *SyntaxToken) (*SyntaxToken, error)

//Visit traverses the syntax tree and applies the visitor function to each token,
//if the visitor function returns a non-nil syntax token, it will replace the given token and stop
//the traversal for this branch of tokens. This means the default return value of visitor should be (nil, nil)
//the returned token will be the new root of the syntax tree
func Visit(ctx context.Context, root *SyntaxToken, visitor Visitor) (*SyntaxToken, error) {
	if root == nil {
		return nil, nil
	}
	modifiedToken, err := visitor(root)
	if err != nil {
		return root, err
	}
	if modifiedToken != nil {
		return modifiedToken, nil
	}

	switch root.Type {
	default:
		if root.Type != "" {
			log.Ctx(ctx).Error().Msgf("Unknown token type '%s' while simplifying", root.Type)
		}
		return root, nil

	case TokenTypeAnonymous, TokenTypeLiteralValue,
		TokenTypeScopeTraversal:
		return root, nil

	case TokenTypeSplat:
		root.Source, err = Visit(ctx, root.Source, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to splat source")
		}
		root.SplatEach, err = Visit(ctx, root.SplatEach, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to splat each")
		}
		return root, nil

	case TokenTypeObjectConst:
		for i, pair := range root.ObjectConst {
			v, err := Visit(ctx, TokenPtr(pair.Value), visitor)
			if err != nil {
				return root, errors.Wrap(err, "error applying simplifier to object const value with key '"+pair.Key+"'")
			}
			root.ObjectConst[i] = ObjectConstItem{
				Key:   pair.Key,
				Value: *v,
			}
		}
		return root, nil

	case TokenTypeArrayConst:
		for i, item := range root.ArrayConst {
			v, err := Visit(ctx, TokenPtr(item), visitor)
			if err != nil {
				return root, errors.Wrap(err, "error applying simplifier to array const item with index '"+fmt.Sprintf("%v", i)+"'")
			}
			root.ArrayConst[i] = *v
		}
		return root, nil

	case TokenTypeTemplate:
		for i, item := range root.Parts {
			v, err := Visit(ctx, TokenPtr(item), visitor)
			if err != nil {
				return root, errors.Wrap(err, "error applying simplifier to template item with index '"+fmt.Sprintf("%v", i)+"'")
			}
			root.Parts[i] = *v
		}
		return root, nil

	case TokenTypeFunctionCall:
		for i, item := range root.FunctionArgs {
			v, err := Visit(ctx, TokenPtr(item), visitor)
			if err != nil {
				return root, errors.Wrap(err, "error applying simplifier to argument '"+fmt.Sprintf("%v", i)+"' of function '"+*root.FunctionName+"'")
			}
			root.FunctionArgs[i] = *v
		}
		return root, nil

	case TokenTypeIndexAccess:
		root.IndexCollection, err = Visit(ctx, root.IndexCollection, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to index access collection")
		}
		root.IndexKey, err = Visit(ctx, root.IndexKey, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to index access key")
		}
		return root, nil

	case TokenTypeRelativeTraversal:
		root.Source, err = Visit(ctx, root.Source, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to relative traversal source")
		}
		return root, nil

	case TokenTypeConditional:
		root.Condition, err = Visit(ctx, root.Condition, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to conditional condition")
		}
		root.TrueResult, err = Visit(ctx, root.TrueResult, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to conditional true result")
		}
		root.FalseResult, err = Visit(ctx, root.FalseResult, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to conditional false result")
		}
		return root, nil

	case TokenTypeParens:
		root.Source, err = Visit(ctx, root.Source, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to parens source")
		}
		return root, nil

	case TokenTypeBinaryOp:
		root.RightHandSide, err = Visit(ctx, root.RightHandSide, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to binary op '"+*root.Operator+"' right hand side")
		}
		root.LeftHandSide, err = Visit(ctx, root.LeftHandSide, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to binary op '"+*root.Operator+"' left hand side")
		}
		return root, nil

	case TokenTypeUnaryOp:
		root.RightHandSide, err = Visit(ctx, root.RightHandSide, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to unary op '"+*root.Operator+"' right hand side")
		}
		return root, nil

	case TokenTypeFor:
		root.ForCollExpr, err = Visit(ctx, root.ForCollExpr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to for collection")
		}
		root.ForKeyExpr, err = Visit(ctx, root.ForKeyExpr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to for key expression")
		}
		root.ForValExpr, err = Visit(ctx, root.ForValExpr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to for value expression")
		}
		root.ForCondExpr, err = Visit(ctx, root.ForCondExpr, visitor)
		if err != nil {
			return root, errors.Wrap(err, "error applying simplifier to for conditional expression")
		}
		return root, nil
	}
}

func InterfaceIsNil(i interface{}) bool {
	if i == nil {
		return true
	}

	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	}

	return false
}
