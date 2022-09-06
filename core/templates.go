package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

//used for parsing
type Manifest struct {
	Latest        *string `json:"latest"`
	LatestVersion *version.Version
	Versions      map[string]ManifestVersion `json:"versions"`
}
type ParentManifestLink struct {
	Url     string  `json:"url"`
	Version *string `json:"version"`
}
type ManifestVersion struct {
	InheritFrom []ParentManifestLink `json:"inheritFrom"`
	Message     *string              `json:"message"`
	Files       []string             `json:"files"`
	Steps       []ManifestStep       `json:"steps"`
}
type ManifestStep struct {
	Templates []string `json:"templates"`
}

func prepareTemplates(ctx context.Context, container *ConfigContainer) (Executable, error) {
	templateConfig := container.GetDataBagsOfType("template")
	if templateConfig == nil {
		return Executable{}, nil
	}

	templateBlock, err := parseTemplateBlock(templateConfig)
	if err != nil {
		return Executable{}, errors.Wrap(err, "error parsing template block")
	}

	manifests := make([]Manifest, 0, len(templateBlock.Manifests))
	for _, link := range templateBlock.Manifests {
		manifest, err := fetchManifestUrl(link.ManifestUrl)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error fetching manifest")
		}
		manifests = append(manifests, manifest)
	}

	executable := Executable{
		Message: []string{},
		Files:   []FileDescription{},
		Steps: []ExecutableStep{
			{Templates: []FileDescription{}},
		},
	}
	for _, file := range templateBlock.Files {
		fileContent, err := fetchFile(file)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error fetching file")
		}
		executable.Files = append(executable.Files, FileDescription{
			Name:    path.Base(file),
			Content: fileContent,
		})
	}
	for _, template := range templateBlock.Templates {
		templateContent, err := fetchFile(template)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error fetching template")
		}

		executable.Steps[0].Templates = append(executable.Steps[0].Templates, FileDescription{
			Name:    path.Base(template),
			Content: templateContent,
		})
	}
	for i, manifest := range manifests {
		constraint := templateBlock.Manifests[i].VersionConstraint
		selectedVersion, err := selectVersion(ctx, constraint, manifest)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error selecting version")
		}
		if selectedVersion == "" {
			return Executable{}, errors.New("no version of template satisfied the constraints '" + constraint.String() + "'")
		}
		templateVersion, ok := manifest.Versions[selectedVersion]
		if !ok {
			return Executable{}, errors.New("no version of template '" + selectedVersion + "' found, this should not happen, I told you weird stuff would happen")
		}
		err = applyManifestToExecutable(ctx, &executable, templateVersion)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error applying manifest version '"+selectedVersion+"' to executable")
		}
	}

	return executable, nil
}

// turns a manifest into an executable, ignores any declared inheritFrom
func applyManifestToExecutable(ctx context.Context, executable *Executable, manifestVersion ManifestVersion) error {
	if len(manifestVersion.InheritFrom) != 0 {
		for _, link := range manifestVersion.InheritFrom {
			log.Ctx(ctx).Debug().Interface("version", link.Version).Msgf("inheriting from manifest '%s'", link.Url)
			parentManifest, err := fetchManifestUrl(link.Url)
			if err != nil {
				return errors.Wrap(err, "error fetching parent manifest at '"+link.Url+"'")
			}
			var versionConstraint version.Constraints
			if link.Version != nil {
				versionConstraint, err = version.NewConstraint(*link.Version)
				if err != nil {
					return errors.Wrap(err, "error parsing version constraint '"+*link.Version+"' for parent manifest '"+link.Url+"'")
				}
			} else {
				versionConstraint = version.Constraints{}
			}

			selectedVersion, err := selectVersion(ctx, versionConstraint, parentManifest)
			if err != nil {
				return errors.Wrap(err, "error selecting version of parent manifest '"+link.Url+"'")
			}
			if selectedVersion == "" {
				return errors.New("no version of parent manifest '" + link.Url + "' satisfied the constraints '" + versionConstraint.String() + "'")
			}
			parent, ok := parentManifest.Versions[selectedVersion]
			if !ok {
				return errors.New("no version of parent manifest '" + link.Url + "' found, this should not happen and is most likely a formatting error on the version name")
			}
			err = applyManifestToExecutable(ctx, executable, parent)
			if err != nil {
				return errors.Wrap(err, "error applying parent manifest '"+link.Url+"' to executable")
			}
		}
	}

	if manifestVersion.Message != nil {
		executable.Message = append(executable.Message, *manifestVersion.Message)
	}
	for _, file := range manifestVersion.Files {
		fileContent, err := fetchFile(file)
		if err != nil {
			return errors.Wrap(err, "error fetching file '"+file+"'")
		}
		executable.Files = append(executable.Files, FileDescription{
			Name:    path.Base(file),
			Content: fileContent,
		})
	}
	for stepIndex, step := range manifestVersion.Steps {
		if stepIndex >= len(executable.Steps) {
			executable.Steps = append(executable.Steps, ExecutableStep{})
		}
		for _, template := range step.Templates {
			templateContent, err := fetchFile(template)
			if err != nil {
				return errors.Wrap(err, "error fetching template '"+template+"'")
			}
			executable.Steps[stepIndex].Templates = append(executable.Steps[stepIndex].Templates, FileDescription{
				Name:    path.Base(template),
				Content: templateContent,
			})
		}
	}
	return nil
}

func selectVersion(ctx context.Context, versionConstraint version.Constraints, manifest Manifest) (string, error) {
	if versionConstraint != nil &&
		manifest.LatestVersion != nil &&
		versionConstraint.Check(manifest.LatestVersion) {
		if _, ok := manifest.Versions[*manifest.Latest]; ok {
			return *manifest.Latest, nil
		}
	}

	definedVersions := make([]*version.Version, 0, len(manifest.Versions))
	for vName := range manifest.Versions {
		v, err := version.NewVersion(vName)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msgf("error parsing version '%s', ignoring it", vName)
			continue
		}
		if v.String() != vName {
			log.Ctx(ctx).Error().Msgf("manifest version '%s' does not match its name, it should be '%s'. This can cause some weird stuff to happen", vName, v.String())
		}
		definedVersions = append(definedVersions, v)
	}
	sort.Sort(version.Collection(definedVersions))

	for _, v := range definedVersions {
		if versionConstraint.Check(v) {
			return v.String(), nil
		}
	}
	return "", nil
}

func fetchManifestUrl(manifestUrl string) (Manifest, error) {
	manifestBody, err := fetchFile(manifestUrl)
	if err != nil {
		return Manifest{}, errors.Wrap(err, "error fetching manifest")
	}
	var manifest Manifest
	err = json.NewDecoder(bytes.NewReader(manifestBody)).Decode(&manifest)
	if err != nil {
		return Manifest{}, errors.Wrap(err, "error json decoding manifest body at '"+manifestUrl+"'")
	}
	if manifest.Latest != nil {
		manifest.LatestVersion, err = version.NewVersion(*manifest.Latest)
		if err != nil {
			return Manifest{}, errors.Wrap(err, "error parsing latest version '"+*manifest.Latest+"'")
		}
	}
	return manifest, nil
}

func fetchFile(fileUrl string) ([]byte, error) {
	if strings.HasPrefix(fileUrl, "file://") {
		return fetchLocalFile(strings.TrimPrefix(fileUrl, "file://"))
	} else if strings.HasPrefix(fileUrl, "http://") || strings.HasPrefix(fileUrl, "https://") {
		return fetchRemoteFile(fileUrl)
	} else if strings.HasPrefix(fileUrl, "base64://") {
		return decodeBase64File(strings.TrimPrefix(fileUrl, "base64://"))
	} else {
		return fetchLocalFile(fileUrl)
	}
}

func fetchLocalFile(filePath string) ([]byte, error) {
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading local file at '"+filePath+"'")
	}
	return file, nil
}

func fetchRemoteFile(fileUrl string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, fileUrl, nil)
	if err != nil {
		return nil, err
	}

	agent := fmt.Sprintf("barbe/"+Version+" (%s; %s)", runtime.GOOS, runtime.GOARCH)
	req.Header.Set("User-Agent", agent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func decodeBase64File(fileB64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(fileB64)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding base64 file")
	}
	return data, nil
}

func GetTemplates(ctx context.Context, container *ConfigContainer) (Executable, error) {
	return prepareTemplates(ctx, container)
}
