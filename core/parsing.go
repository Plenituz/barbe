package core

import (
	"fmt"
	"github.com/pkg/errors"
)

type TemplateBlock struct {
	Files      []string
	Components []string
	Manifests  []string
}

func parseTemplateBlock(templateConfig []DataBag) (TemplateBlock, error) {
	template := TemplateBlock{
		Files:      []string{},
		Components: []string{},
		Manifests:  []string{},
	}
	for _, t := range templateConfig {
		attrs, err := extractBlockAttrs(t.Value)
		if err != nil {
			return template, errors.Wrap(err, "error parsing template block")
		}

		name := ""
		if t.Name != "" {
			name = t.Name + "."
		}

		fileKeys := map[string]struct{}{
			"files": {},
			"file":  {},
		}
		fileKeyValues := GetObjectKeysValues(fileKeys, attrs)
		for _, fileSyntax := range fileKeyValues {
			files, err := interpretAsStrArray(fileSyntax)
			if err != nil {
				return template, errors.Wrap(err, fmt.Sprintf("error parsing 'template.%sfiles'", name))
			}
			template.Files = append(template.Files, files...)
		}

		templateKeys := map[string]struct{}{
			"components": {},
			"component":  {},
		}
		templateKeyValues := GetObjectKeysValues(templateKeys, attrs)
		for _, templateSyntax := range templateKeyValues {
			templates, err := interpretAsStrArray(templateSyntax)
			if err != nil {
				return template, errors.Wrap(err, fmt.Sprintf("error parsing 'template.%stemplates'", name))
			}
			template.Components = append(template.Components, templates...)
		}

		manifestKeys := map[string]struct{}{
			"manifests": {},
			"manifest":  {},
		}
		manifestKeyValues := GetObjectKeysValues(manifestKeys, attrs)
		for _, manifestSyntax := range manifestKeyValues {
			manifests, err := interpretAsStrArray(manifestSyntax)
			if err != nil {
				return template, errors.Wrap(err, fmt.Sprintf("error parsing 'template.%smanifest'", name))
			}
			template.Manifests = append(template.Manifests, manifests...)
		}
	}
	return template, nil
}

func extractBlockAttrs(blockArr SyntaxToken) ([]ObjectConstItem, error) {
	if blockArr.Type == TokenTypeObjectConst {
		return blockArr.ObjectConst, nil
	}
	if blockArr.Type != TokenTypeArrayConst {
		return nil, errors.New("'template' block must be an array or an object")
	}
	items := make([]ObjectConstItem, 0)
	for _, item := range blockArr.ArrayConst {
		if item.Type != TokenTypeObjectConst {
			return nil, errors.New("'template' block must be an array of objects")
		}
		items = append(items, item.ObjectConst...)
	}
	return items, nil
}

func interpretAsStrArray(token SyntaxToken) ([]string, error) {
	output := make([]string, 0)
	if token.Type == TokenTypeArrayConst {
		for i, fileItem := range token.ArrayConst {
			fileStr, err := ExtractAsStringValue(fileItem)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("couldn't interpret element %d of array as string", i))
			}
			output = append(output, fileStr)
		}
	} else {
		fileStr, err := ExtractAsStringValue(token)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't interpret value as string")
		}
		output = append(output, fileStr)
	}
	return output, nil
}
