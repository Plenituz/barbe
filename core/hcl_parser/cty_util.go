package hcl_parser

import (
	"barbe/core"
	"fmt"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
)

func UnmarshalDatabags(v cty.Value) ([]core.DataBag, error) {
	outputs := make([]core.DataBag, 0)
	var err error
	for _, item := range v.AsValueSlice() {
		if item.IsNull() {
			continue
		}
		output := core.DataBag{}
		output.Name, err = extractStringValueCty(item.GetAttr("name"))
		if err != nil {
			return nil, err
		}
		output.Type, err = extractStringValueCty(item.GetAttr("type"))
		if err != nil {
			return nil, err
		}
		output.Value, err = UnmarshalSyntaxToken(item.GetAttr("value"))
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

func UnmarshalSyntaxToken(v cty.Value) (core.SyntaxToken, error) {
	var err error
	output := core.SyntaxToken{}
	switch {
	case v.Type().IsPrimitiveType():
		output.Type = core.TokenTypeLiteralValue
		output.Value, err = convertToPrimitive(v)
		if err != nil {
			return output, err
		}
		return output, nil
	case v.Type().IsListType(), v.Type().IsSetType(), v.Type().IsTupleType():
		output.Type = core.TokenTypeArrayConst
		output.ArrayConst = make([]core.SyntaxToken, 0)
		for _, item := range v.AsValueSlice() {
			if item.IsNull() {
				continue
			}
			c, err := UnmarshalSyntaxToken(item)
			if err != nil {
				return output, err
			}
			output.ArrayConst = append(output.ArrayConst, c)
		}
		return output, nil
	}
	if !v.Type().HasAttribute("Type") && v.Type().IsObjectType() {
		output.Type = core.TokenTypeObjectConst
		output.ObjectConst = make([]core.ObjectConstItem, 0)
		for key, item := range v.AsValueMap() {
			if item.IsNull() {
				continue
			}
			val, err := UnmarshalSyntaxToken(item)
			if err != nil {
				return output, err
			}
			output.ObjectConst = append(output.ObjectConst, core.ObjectConstItem{
				Key:   key,
				Value: val,
			})
		}
		return output, nil
	}

	output.Type, err = extractStringValueCty(v.GetAttr("Type"))
	if err != nil {
		return output, err
	}
	if v.Type().HasAttribute("Meta") {
		meta := v.GetAttr("Meta").AsValueMap()
		output.Meta = map[string]interface{}{}
		for k, v := range meta {
			output.Meta[k], err = convertToPrimitive(v)
			if err != nil {
				return output, err
			}
		}
	}
	if v.Type().HasAttribute("Value") && !v.GetAttr("Value").IsNull() {
		output.Value, err = convertToPrimitive(v.GetAttr("Value"))
		if err != nil {
			return output, err
		}
	}
	if v.Type().HasAttribute("ObjectConst") {
		output.ObjectConst, err = unmarshalObjectConst(v.GetAttr("ObjectConst"))
		if err != nil {
			return output, err
		}
	}
	if v.Type().HasAttribute("ArrayConst") {
		arrayConsts := make([]core.SyntaxToken, 0)
		for _, item := range v.GetAttr("ArrayConst").AsValueSlice() {
			if item.IsNull() {
				continue
			}
			d, err := UnmarshalSyntaxToken(item)
			if err != nil {
				return output, err
			}
			arrayConsts = append(arrayConsts, d)
		}
		output.ArrayConst = arrayConsts
	}
	if v.Type().HasAttribute("Traversal") {
		output.Traversal, err = unmarshalTraversal(v.GetAttr("Traversal"))
		if err != nil {
			return output, err
		}
	}
	if v.Type().HasAttribute("FunctionName") {
		output.FunctionName = core.Ptr(v.GetAttr("FunctionName").AsString())
	}
	if v.Type().HasAttribute("FunctionArgs") {
		for _, item := range v.GetAttr("FunctionArgs").AsValueSlice() {
			d, err := UnmarshalSyntaxToken(item)
			if err != nil {
				return output, err
			}
			output.FunctionArgs = append(output.FunctionArgs, d)
		}
	}
	if v.Type().HasAttribute("Parts") {
		for _, item := range v.GetAttr("Parts").AsValueSlice() {
			d, err := UnmarshalSyntaxToken(item)
			if err != nil {
				return output, err
			}
			output.Parts = append(output.Parts, d)
		}
	}
	if v.Type().HasAttribute("IndexCollection") {
		indexCollection, err := UnmarshalSyntaxToken(v.GetAttr("IndexCollection"))
		if err != nil {
			return output, err
		}
		output.IndexCollection = core.TokenPtr(indexCollection)
	}
	if v.Type().HasAttribute("IndexKey") {
		indexKey, err := UnmarshalSyntaxToken(v.GetAttr("IndexKey"))
		if err != nil {
			return output, err
		}
		output.IndexKey = core.TokenPtr(indexKey)
	}
	if v.Type().HasAttribute("Source") {
		source, err := UnmarshalSyntaxToken(v.GetAttr("Source"))
		if err != nil {
			return output, err
		}
		output.Source = core.TokenPtr(source)
	}
	if v.Type().HasAttribute("ForKeyVar") {
		output.ForKeyVar = core.Ptr(v.GetAttr("ForKeyVar").AsString())
	}
	if v.Type().HasAttribute("ForValVar") {
		output.ForValVar = core.Ptr(v.GetAttr("ForValVar").AsString())
	}
	if v.Type().HasAttribute("ForCollExpr") {
		forCollExpr, err := UnmarshalSyntaxToken(v.GetAttr("ForCollExpr"))
		if err != nil {
			return output, err
		}
		output.ForCollExpr = core.TokenPtr(forCollExpr)
	}
	if v.Type().HasAttribute("ForKeyExpr") {
		forKeyExpr, err := UnmarshalSyntaxToken(v.GetAttr("ForKeyExpr"))
		if err != nil {
			return output, err
		}
		output.ForKeyExpr = core.TokenPtr(forKeyExpr)
	}
	if v.Type().HasAttribute("ForValExpr") {
		forValExpr, err := UnmarshalSyntaxToken(v.GetAttr("ForValExpr"))
		if err != nil {
			return output, err
		}
		output.ForValExpr = core.TokenPtr(forValExpr)
	}
	if v.Type().HasAttribute("ForCondExpr") {
		forCondExpr, err := UnmarshalSyntaxToken(v.GetAttr("ForCondExpr"))
		if err != nil {
			return output, err
		}
		output.ForCondExpr = core.TokenPtr(forCondExpr)
	}
	if v.Type().HasAttribute("Condition") {
		condition, err := UnmarshalSyntaxToken(v.GetAttr("Condition"))
		if err != nil {
			return output, err
		}
		output.Condition = core.TokenPtr(condition)
	}
	if v.Type().HasAttribute("TrueResult") {
		trueResult, err := UnmarshalSyntaxToken(v.GetAttr("TrueResult"))
		if err != nil {
			return output, err
		}
		output.TrueResult = core.TokenPtr(trueResult)
	}
	if v.Type().HasAttribute("FalseResult") {
		falseResult, err := UnmarshalSyntaxToken(v.GetAttr("FalseResult"))
		if err != nil {
			return output, err
		}
		output.FalseResult = core.TokenPtr(falseResult)
	}
	if v.Type().HasAttribute("RightHandSide") {
		rhs, err := UnmarshalSyntaxToken(v.GetAttr("RightHandSide"))
		if err != nil {
			return output, err
		}
		output.RightHandSide = core.TokenPtr(rhs)
	}
	if v.Type().HasAttribute("LeftHandSide") {
		lhs, err := UnmarshalSyntaxToken(v.GetAttr("LeftHandSide"))
		if err != nil {
			return output, err
		}
		output.LeftHandSide = core.TokenPtr(lhs)
	}
	if v.Type().HasAttribute("Operator") {
		output.Operator = core.Ptr(v.GetAttr("Operator").AsString())
	}
	if v.Type().HasAttribute("SplatEach") {
		splateach, err := UnmarshalSyntaxToken(v.GetAttr("SplatEach"))
		if err != nil {
			return output, err
		}
		output.SplatEach = core.TokenPtr(splateach)
	}

	return output, nil
}

func MarshalSyntaxToken(v core.SyntaxToken) (cty.Value, error) {
	var err error
	output := map[string]cty.Value{
		"Type": cty.StringVal(v.Type),
	}
	output["Value"], err = primitiveToCty(v.Value)
	if err != nil {
		return cty.NilVal, err
	}

	if v.ObjectConst != nil {
		output["ObjectConst"], err = marshalObjectConst(v.ObjectConst)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.Meta != nil {
		obj := map[string]cty.Value{}
		for k, v := range v.Meta {
			obj[k], err = primitiveToCty(v)
			if err != nil {
				return cty.NilVal, err
			}
		}
		output["Meta"] = cty.ObjectVal(obj)
	}
	if v.ArrayConst != nil {
		arrayConsts := make([]cty.Value, 0, len(v.ArrayConst))
		for _, item := range v.ArrayConst {
			d, err := MarshalSyntaxToken(item)
			if err != nil {
				return cty.NilVal, err
			}
			arrayConsts = append(arrayConsts, d)
		}
		output["ArrayConst"] = cty.TupleVal(arrayConsts)

	}
	if v.Traversal != nil {
		output["Traversal"], err = marshalTraversal(v.Traversal)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.FunctionName != nil {
		output["FunctionName"] = cty.StringVal(*v.FunctionName)
	}
	if v.FunctionArgs != nil {
		args := make([]cty.Value, 0, len(v.FunctionArgs))
		for _, item := range v.FunctionArgs {
			d, err := MarshalSyntaxToken(item)
			if err != nil {
				return cty.NilVal, err
			}
			args = append(args, d)
		}
		output["FunctionArgs"] = cty.TupleVal(args)
	}
	if v.Parts != nil {
		parts := make([]cty.Value, 0, len(v.Parts))
		for _, item := range v.Parts {
			d, err := MarshalSyntaxToken(item)
			if err != nil {
				return cty.NilVal, err
			}
			parts = append(parts, d)
		}
		output["Parts"] = cty.TupleVal(parts)
	}
	if v.IndexCollection != nil {
		output["IndexCollection"], err = MarshalSyntaxToken(*v.IndexCollection)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.IndexKey != nil {
		output["IndexKey"], err = MarshalSyntaxToken(*v.IndexKey)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.Source != nil {
		output["Source"], err = MarshalSyntaxToken(*v.Source)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.ForKeyVar != nil {
		output["ForKeyVar"] = cty.StringVal(*v.ForKeyVar)
	}
	if v.ForValVar != nil {
		output["ForValVar"] = cty.StringVal(*v.ForValVar)
	}
	if v.ForCollExpr != nil {
		output["ForCollExpr"], err = MarshalSyntaxToken(*v.ForCollExpr)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.ForKeyExpr != nil {
		output["ForKeyExpr"], err = MarshalSyntaxToken(*v.ForKeyExpr)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.ForValExpr != nil {
		output["ForValExpr"], err = MarshalSyntaxToken(*v.ForValExpr)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.ForCondExpr != nil {
		output["ForCondExpr"], err = MarshalSyntaxToken(*v.ForCondExpr)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.Condition != nil {
		output["Condition"], err = MarshalSyntaxToken(*v.Condition)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.TrueResult != nil {
		output["TrueResult"], err = MarshalSyntaxToken(*v.TrueResult)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.FalseResult != nil {
		output["FalseResult"], err = MarshalSyntaxToken(*v.FalseResult)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.RightHandSide != nil {
		output["RightHandSide"], err = MarshalSyntaxToken(*v.RightHandSide)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.LeftHandSide != nil {
		output["LeftHandSide"], err = MarshalSyntaxToken(*v.LeftHandSide)
		if err != nil {
			return cty.NilVal, err
		}
	}
	if v.Operator != nil {
		output["Operator"] = cty.StringVal(*v.Operator)
	}
	if v.SplatEach != nil {
		output["SplatEach"], err = MarshalSyntaxToken(*v.SplatEach)
		if err != nil {
			return cty.NilVal, err
		}
	}

	return cty.ObjectVal(output), nil
}

func extractStringValueCty(attr cty.Value) (string, error) {
	if attr.Type() == cty.String {
		return attr.AsString(), nil
	}
	//TODO avoid doing an unmarshal here and just return the string from the cty value
	read, err := UnmarshalSyntaxToken(attr)
	if err != nil {
		return "", err
	}
	return core.ExtractAsStringValue(read)
}

func unmarshalTraversal(v cty.Value) ([]core.Traverse, error) {
	output := make([]core.Traverse, 0)
	for _, item := range v.AsValueSlice() {
		t := core.Traverse{
			Type: item.GetAttr("Type").AsString(),
		}
		if item.Type().HasAttribute("Name") {
			nameAttr := item.GetAttr("Name")
			if nameAttr.Type() != cty.String {
				return output, errors.New("traversal name must be a string")
			}
			t.Name = core.Ptr(nameAttr.AsString())
		}
		if item.Type().HasAttribute("Index") {
			indexAttr := item.GetAttr("Index")
			switch {
			case indexAttr.Type() == cty.String:
				t.Index = indexAttr.AsString()
			case indexAttr.Type() == cty.Number:
				t.Index, _ = indexAttr.AsBigFloat().Int64()
			default:
				return nil, errors.New("traversal index must be a string or number")
			}
		}
		output = append(output, t)
	}
	return output, nil
}

func marshalTraversal(traversal []core.Traverse) (cty.Value, error) {
	output := make([]cty.Value, 0, len(traversal))
	for _, t := range traversal {
		item := map[string]cty.Value{
			"Type": cty.StringVal(t.Type),
		}
		if t.Name != nil {
			item["Name"] = cty.StringVal(*t.Name)
		}
		if !core.InterfaceIsNil(t.Index) {
			var err error
			item["Index"], err = primitiveToCty(t.Index)
			if err != nil {
				return cty.NilVal, errors.Wrap(err, "failed to marshal traversal index")
			}
		}
		output = append(output, cty.ObjectVal(item))
	}
	return cty.TupleVal(output), nil
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

func unmarshalObjectConst(v cty.Value) ([]core.ObjectConstItem, error) {
	output := make([]core.ObjectConstItem, 0)
	var err error
	for _, item := range v.AsValueSlice() {
		outputItem := core.ObjectConstItem{}
		keyAttr := item.GetAttr("Key")
		switch {
		case keyAttr.Type() == cty.String:
			outputItem.Key = keyAttr.AsString()
		case keyAttr.Type().IsObjectType() || keyAttr.Type().IsMapType():
			keyVal, err := extractStringValueCty(keyAttr)
			if err != nil {
				return nil, err
			}
			outputItem.Key = keyVal
		default:
			return nil, errors.New("key must be string")
		}
		outputItem.Value, err = UnmarshalSyntaxToken(item.GetAttr("Value"))
		if err != nil {
			return nil, err
		}
		output = append(output, outputItem)
	}
	return output, nil
}

func marshalObjectConst(v []core.ObjectConstItem) (cty.Value, error) {
	output := make([]cty.Value, 0, len(v))
	for _, item := range v {
		outputItem := map[string]cty.Value{
			"Key": cty.StringVal(item.Key),
		}
		var err error
		outputItem["Value"], err = MarshalSyntaxToken(item.Value)
		if err != nil {
			return cty.NilVal, err
		}
		output = append(output, cty.ObjectVal(outputItem))
	}
	return cty.TupleVal(output), nil
}

func convertToPrimitive(v cty.Value) (interface{}, error) {
	if v.IsNull() {
		return nil, nil
	}
	switch v.Type() {
	case cty.String:
		return v.AsString(), nil
	case cty.Number:
		f, _ := v.AsBigFloat().Float64()
		return f, nil
	case cty.Bool:
		return v.True(), nil
	default:
		return nil, fmt.Errorf("unsupported type %s", v.Type().FriendlyName())
	}
}
