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

func TraverseDeepEqual(a Traverse, b Traverse) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	default:
		fmt.Println("unhandled traverse type: '" + a.Type + "'")
		return false
	case TraverseTypeAttr:
		if a.Name == nil && b.Name == nil {
			return true
		}
		if a.Name == nil || b.Name == nil {
			return false
		}
		return *a.Name == *b.Name
	case TraverseTypeIndex:
		return reflect.DeepEqual(a.Index, b.Index)
	case TraverseTypeSplat:
		return true
	}
}

func ConfigContainerDeepEqual(a ConfigContainer, b ConfigContainer) bool {
	countA := 0
	countB := 0
	for _, m := range a.DataBags {
		for _, v := range m {
			countA += len(v)
		}
	}
	for _, m := range b.DataBags {
		for _, v := range m {
			countB += len(v)
		}
	}
	if countA != countB {
		return false
	}
	for typeName, databags := range a.DataBags {
		if typeName == StateDatabagType {
			continue
		}
		for databagName, databagGroup := range databags {
			for _, databag := range databagGroup {
				if b.Contains(databag) {
					for _, existingBag := range b.GetDataBagGroup(typeName, databagName) {
						if !reflect.DeepEqual(existingBag.Labels, databag.Labels) {
							return false
						}
						if existingBag.Value.IsSuperSetOf(databag.Value) {
							continue
						}
						if !TokensDeepEqual(existingBag.Value, databag.Value) {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

func TokensDeepEqual(a SyntaxToken, b SyntaxToken) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	default:
		fmt.Println("unhandled token type: '" + a.Type + "'")
		return false
	case TokenTypeLiteralValue:
		return reflect.DeepEqual(a.Value, b.Value)
	case TokenTypeScopeTraversal:
		if len(a.Traversal) != len(b.Traversal) {
			return false
		}
		for i, traverse := range a.Traversal {
			if !TraverseDeepEqual(traverse, b.Traversal[i]) {
				return false
			}
		}
		return true
	case TokenTypeFunctionCall:
		if *a.FunctionName != *b.FunctionName {
			return false
		}
		if len(a.FunctionArgs) != len(b.FunctionArgs) {
			return false
		}
		for i, arg := range a.FunctionArgs {
			if !TokensDeepEqual(arg, b.FunctionArgs[i]) {
				return false
			}
		}
		return true
	case TokenTypeIndexAccess:
		if !TokensDeepEqual(*a.IndexCollection, *b.IndexCollection) {
			return false
		}
		if a.IndexKey == nil && b.IndexKey == nil {
			return true
		}
		if a.IndexKey == nil || b.IndexKey == nil {
			return false
		}
		return TokensDeepEqual(*a.IndexKey, *b.IndexKey)
	case TokenTypeFor:
		if a.ForKeyVar == nil {
			if b.ForKeyVar != nil {
				return false
			}
		} else {
			if b.ForKeyVar == nil {
				return false
			}
			if *a.ForKeyVar != *b.ForKeyVar {
				return false
			}
		}
		if a.ForValVar == nil {
			if b.ForValVar != nil {
				return false
			}
		} else {
			if b.ForValVar == nil {
				return false
			}
			if *a.ForValVar != *b.ForValVar {
				return false
			}
		}
		if a.ForCollExpr == nil {
			if b.ForCollExpr != nil {
				return false
			}
		} else {
			if b.ForCollExpr == nil {
				return false
			}
			if !TokensDeepEqual(*a.ForCollExpr, *b.ForCollExpr) {
				return false
			}
		}
		if a.ForKeyExpr == nil {
			if b.ForKeyExpr != nil {
				return false
			}
		} else {
			if b.ForKeyExpr == nil {
				return false
			}
			if !TokensDeepEqual(*a.ForKeyExpr, *b.ForKeyExpr) {
				return false
			}
		}
		if a.ForValExpr == nil {
			if b.ForValExpr != nil {
				return false
			}
		} else {
			if b.ForValExpr == nil {
				return false
			}
			if !TokensDeepEqual(*a.ForValExpr, *b.ForValExpr) {
				return false
			}
		}
		if a.ForCondExpr == nil {
			if b.ForCondExpr != nil {
				return false
			}
		} else {
			if b.ForCondExpr == nil {
				return false
			}
			if !TokensDeepEqual(*a.ForCondExpr, *b.ForCondExpr) {
				return false
			}
		}
		return true
	case TokenTypeRelativeTraversal:
		if len(a.Traversal) != len(b.Traversal) {
			return false
		}
		for i, traverse := range a.Traversal {
			if !TraverseDeepEqual(traverse, b.Traversal[i]) {
				return false
			}
		}
		return TokensDeepEqual(*a.Source, *b.Source)
	case TokenTypeConditional:
		if !TokensDeepEqual(*a.Condition, *b.Condition) {
			return false
		}
		if !TokensDeepEqual(*a.TrueResult, *b.TrueResult) {
			return false
		}
		return TokensDeepEqual(*a.FalseResult, *b.FalseResult)
	case TokenTypeBinaryOp:
		if *a.Operator != *b.Operator {
			return false
		}
		if !TokensDeepEqual(*a.RightHandSide, *b.RightHandSide) {
			return false
		}
		return TokensDeepEqual(*a.LeftHandSide, *b.LeftHandSide)
	case TokenTypeUnaryOp:
		if *a.Operator != *b.Operator {
			return false
		}
		return TokensDeepEqual(*a.RightHandSide, *b.RightHandSide)
	case TokenTypeSplat:
		return TokensDeepEqual(*a.SplatEach, *b.SplatEach)
	case TokenTypeAnonymous:
		return true
	case TokenTypeTemplate:
		if len(a.Parts) != len(b.Parts) {
			return false
		}
		for i, part := range a.Parts {
			if !TokensDeepEqual(part, b.Parts[i]) {
				return false
			}
		}
		return true
	case TokenTypeParens:
		return TokensDeepEqual(*a.Source, *b.Source)
	case TokenTypeObjectConst:
		if len(a.ObjectConst) != len(b.ObjectConst) {
			return false
		}
		obj := make(map[string]SyntaxToken)
		for _, pair := range a.ObjectConst {
			obj[pair.Key] = pair.Value
		}
		for _, pair := range b.ObjectConst {
			v, ok := obj[pair.Key]
			if !ok {
				return false
			}
			if !TokensDeepEqual(v, pair.Value) {
				return false
			}
		}
		return true
	case TokenTypeArrayConst:
		if len(a.ArrayConst) != len(b.ArrayConst) {
			return false
		}
		for i, v := range a.ArrayConst {
			if !TokensDeepEqual(v, b.ArrayConst[i]) {
				return false
			}
		}
		return true
	}
}

//turn a syntax token into a go value. Returns a partial value if an error occurs when possible
func TokenToGoValue(token SyntaxToken) (interface{}, error) {
	switch token.Type {
	default:
		return nil, fmt.Errorf("unexpected token type: '%s'", token.Type)
	case TokenTypeLiteralValue:
		return token.Value, nil
	case TokenTypeScopeTraversal,
		TokenTypeFunctionCall,
		TokenTypeIndexAccess,
		TokenTypeFor,
		TokenTypeRelativeTraversal,
		TokenTypeConditional,
		TokenTypeBinaryOp,
		TokenTypeUnaryOp,
		TokenTypeSplat,
		TokenTypeAnonymous:
		return nil, fmt.Errorf("cannot convert token type '%s' to go value", token.Type)
	case TokenTypeTemplate:
		v, err := ExtractAsStringValue(token)
		if err != nil {
			return nil, errors.New("cannot convert token type 'template' to go value unless it's resolvable as string")
		}
		return v, nil
	case TokenTypeParens:
		if token.Source != nil {
			return TokenToGoValue(*token.Source)
		}
		return nil, nil
	case TokenTypeObjectConst:
		obj := make(map[string]interface{})
		var hasErr error
		for _, pair := range token.ObjectConst {
			v, err := TokenToGoValue(pair.Value)
			if err != nil {
				hasErr = err
				continue
			}
			obj[pair.Key] = v
		}
		return obj, hasErr
	case TokenTypeArrayConst:
		arr := make([]interface{}, 0, len(token.ArrayConst))
		var hasErr error
		for _, item := range token.ArrayConst {
			v, err := TokenToGoValue(item)
			if err != nil {
				hasErr = err
				continue
			}
			arr = append(arr, v)
		}
		return arr, hasErr
	}
}

//TODO make this function return partial result when there is an error
func GoValueToToken(v interface{}) (SyntaxToken, error) {
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
		return GoValueToToken(rVal.Elem().Interface())
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
			item, err := GoValueToToken(rVal.Index(i).Interface())
			if err != nil {
				return SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding index %v of array", i))
			}
			output.ArrayConst = append(output.ArrayConst, item)
		}
		return output, nil

	case reflect.Map:
		typeField := rVal.MapIndex(reflect.ValueOf("Type"))
		if typeField.IsValid() {
			if typeField.Type().Kind() == reflect.Interface || typeField.Type().Kind() == reflect.Ptr {
				typeField = typeField.Elem()
			}
			if typeField.Kind() == reflect.String && IsTokenType(typeField.String()) {
				break
			}
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
			item, err := GoValueToToken(v.Interface())
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
		typeField := rVal.FieldByName("Type")
		if typeField.IsValid() {
			if typeField.Type().Kind() == reflect.Interface || typeField.Type().Kind() == reflect.Ptr {
				typeField = typeField.Elem()
			}
			if typeField.Kind() == reflect.String && IsTokenType(typeField.String()) {
				break
			}
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
			item, err := GoValueToToken(field.Interface())
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
