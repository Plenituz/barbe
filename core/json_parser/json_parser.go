package json_parser

import (
	"barbe/core"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"reflect"
	"strings"
)

type JsonParser struct{}

func (j JsonParser) Name() string {
	return "json_parser"
}

func (j JsonParser) CanParse(ctx context.Context, fileDesc core.FileDescription) (bool, error) {
	return strings.HasSuffix(strings.ToLower(fileDesc.Name), ".json"), nil
}

func (j JsonParser) Parse(ctx context.Context, fileDesc core.FileDescription, container *core.ConfigContainer) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(fileDesc.Content, &raw); err != nil {
		return errors.Wrap(err, "failed to parse json")
	}

	for typeName, v := range raw {
		var rawType map[string]interface{}
		if err := mapstructure.Decode(v, &rawType); err != nil {
			log.Ctx(ctx).Warn().Err(err).Msg("key '" + typeName + "' is not a map")
			return errors.Wrap(err, "failed to parse json")
		}

		for name, tokenI := range rawType {
			token, err := parsedJsonToToken(tokenI)
			if err != nil {
				return errors.Wrap(err, "failed to parse json")
			}
			bag := core.DataBag{
				Name:   name,
				Type:   typeName,
				Labels: []string{},
				Value:  token,
			}
			if err := container.Insert(bag); err != nil {
				return errors.Wrap(err, "couldn't insert databag")
			}
		}
	}
	return nil
}

func parsedJsonToToken(v interface{}) (core.SyntaxToken, error) {
	if core.InterfaceIsNil(v) {
		return core.SyntaxToken{
			Type:  core.TokenTypeLiteralValue,
			Value: nil,
		}, nil
	}
	rVal := reflect.ValueOf(v)
	switch rVal.Type().Kind() {
	default:
		return core.SyntaxToken{}, errors.New("cannot decode value of type " + rVal.Type().Kind().String())
	case reflect.Interface, reflect.Ptr:
		return parsedJsonToToken(rVal.Elem().Interface())
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
		return core.DecodeValue(v)

	case reflect.Array, reflect.Slice:
		output := core.SyntaxToken{
			Type:       core.TokenTypeArrayConst,
			ArrayConst: make([]core.SyntaxToken, 0, rVal.Len()),
		}

		for i := 0; i < rVal.Len(); i++ {
			item, err := parsedJsonToToken(rVal.Index(i).Interface())
			if err != nil {
				return core.SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding index %v of array", i))
			}
			output.ArrayConst = append(output.ArrayConst, item)
		}
		return output, nil

	case reflect.Map:
		output := core.SyntaxToken{
			Type:        core.TokenTypeObjectConst,
			ObjectConst: make([]core.ObjectConstItem, 0, rVal.Len()),
		}
		iter := rVal.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()

			if k.Kind() != reflect.String {
				return core.SyntaxToken{}, errors.New("map key must be string")
			}
			item, err := parsedJsonToToken(v.Interface())
			if err != nil {
				return core.SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding map value for key %v", k.String()))
			}
			output.ObjectConst = append(output.ObjectConst, core.ObjectConstItem{
				Key:   k.String(),
				Value: item,
			})
		}
		return output, nil
	case reflect.Struct:
		output := core.SyntaxToken{
			Type:        core.TokenTypeObjectConst,
			ObjectConst: make([]core.ObjectConstItem, 0, rVal.Len()),
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
			item, err := parsedJsonToToken(field.Interface())
			if err != nil {
				return core.SyntaxToken{}, errors.Wrap(err, fmt.Sprintf("error decoding struct value for field %v", fieldName))
			}
			output.ObjectConst = append(output.ObjectConst, core.ObjectConstItem{
				Key:   fieldName,
				Value: item,
			})
		}
		return output, nil
	}
}
