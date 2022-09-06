package core

import (
	"fmt"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
)

type TemplateBlock struct {
	Files     []string
	Templates []string
	Manifests []ManifestLink
}

type ManifestLink struct {
	VersionConstraint version.Constraints
	ManifestUrl       string
}

func parseTemplateBlock(templateConfig []*DataBag) (TemplateBlock, error) {
	template := TemplateBlock{
		Files:     []string{},
		Templates: []string{},
		Manifests: []ManifestLink{},
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
			"templates": {},
			"template":  {},
		}
		templateKeyValues := GetObjectKeysValues(templateKeys, attrs)
		for _, templateSyntax := range templateKeyValues {
			templates, err := interpretAsStrArray(templateSyntax)
			if err != nil {
				return template, errors.Wrap(err, fmt.Sprintf("error parsing 'template.%stemplates'", name))
			}
			template.Templates = append(template.Templates, templates...)
		}

		manifestKeyValues := GetObjectKeysValues(map[string]struct{}{"manifest": {}}, attrs)
		for _, manifestSyntax := range manifestKeyValues {
			manifests, err := interpretManifest(attrs, manifestSyntax)
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

func interpretManifest(parentAttrs []ObjectConstItem, token SyntaxToken) ([]ManifestLink, error) {
	if token.Type == TokenTypeObjectConst {
		urlSyntax := GetObjectKeyValues("url", token.ObjectConst)
		if len(urlSyntax) == 0 {
			return nil, errors.New("manifest object must have a 'url' attribute")
		}
		if len(urlSyntax) > 1 {
			return nil, errors.New("manifest object must have only one 'url' attribute")
		}
		urlStr, err := ExtractAsStringValue(urlSyntax[0])
		if err != nil {
			return nil, errors.Wrap(err, "couldn't interpret 'url' attribute as string")
		}

		output := ManifestLink{
			ManifestUrl: urlStr,
		}
		versionSyntax := GetObjectKeyValues("version", token.ObjectConst)
		if len(versionSyntax) != 0 {
			if len(versionSyntax) > 1 {
				return nil, errors.New("multiple 'version' attributes found on manifest object")
			}
			versionStr, err := ExtractAsStringValue(versionSyntax[0])
			if err != nil {
				return nil, errors.Wrap(err, "'version' on manifest object is not interpretable as a string")
			}
			output.VersionConstraint, err = version.NewConstraint(versionStr)
			if err != nil {
				return nil, errors.Wrap(err, "'version' on manifest object is not a valid version constraint")
			}
		}
		return []ManifestLink{output}, nil
	}
	if token.Type == TokenTypeArrayConst {
		output := make([]ManifestLink, 0)
		for i, arrItem := range token.ArrayConst {
			if arrItem.Type != TokenTypeObjectConst {
				return nil, errors.New(fmt.Sprintf("element %d of manifest array is not an object", i))
			}
			manifest, err := interpretManifest([]ObjectConstItem{}, arrItem)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("error interpreting element %d of manifest array", i))
			}
			output = append(output, manifest...)
		}
		return output, nil
	}

	str, err := ExtractAsStringValue(token)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't interpret value as object, block, array or string")
	}

	output := ManifestLink{
		ManifestUrl: str,
	}
	versionSyntax := GetObjectKeysValues(map[string]struct{}{"version": {}}, parentAttrs)
	if len(versionSyntax) != 0 {
		if len(versionSyntax) > 1 {
			return nil, errors.New("multiple 'version' attributes found")
		}
		versionStr, err := ExtractAsStringValue(versionSyntax[0])
		if err != nil {
			return nil, errors.Wrap(err, "'version' is not interpretable as a string")
		}
		output.VersionConstraint, err = version.NewConstraint(versionStr)
		if err != nil {
			return nil, errors.Wrap(err, "'version' is not a valid version constraint")
		}
	}
	return []ManifestLink{output}, nil
}
