package core

import (
	"barbe/core/fetcher"
	"context"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"strings"
)

type Manifest struct {
	Message string `json:"message"`
	//files are plain config files that are added to the files to parse
	Files      []string `json:"files"`
	Components []string `json:"components"`
	Manifests  []string `json:"manifests"`
}

func (maker *Maker) GetTemplates(ctx context.Context, container *ConfigContainer) (Executable, error) {
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
		manifest, err := maker.fetchManifest(ctx, link)
		if err != nil {
			return Executable{}, errors.Wrap(err, "error fetching manifest")
		}
		manifests = append(manifests, manifest)
	}

	for {
		noneFound := true
		for i := 0; i < len(manifests); i++ {
			manifest := manifests[i]
			if len(manifest.Manifests) == 0 {
				continue
			}
			noneFound = false
			for _, link := range manifest.Manifests {
				manifest, err := maker.fetchManifest(ctx, link)
				if err != nil {
					return Executable{}, errors.Wrap(err, "error fetching manifest")
				}
				manifests = append(manifests, manifest)
			}
			manifests[i].Manifests = nil
		}
		if noneFound {
			break
		}
	}

	//prefetch everything in parallel, it'll be in the Fetcher's cache
	fetcherGroup := errgroup.Group{}
	fetcherGroup.SetLimit(10)
	for _, manifest := range manifests {
		for i := range manifest.Files {
			link := manifest.Files[i]
			fetcherGroup.Go(func() error {
				_, err := maker.Fetcher.Fetch(link)
				return err
			})
		}
		for i := range manifest.Components {
			link := manifest.Components[i]
			fetcherGroup.Go(func() error {
				_, err := maker.Fetcher.Fetch(link)
				return err
			})
		}
	}
	err = fetcherGroup.Wait()
	if err != nil {
		return Executable{}, errors.Wrap(err, "error fetching files")
	}

	executable := Executable{
		Message:    "",
		Files:      []fetcher.FileDescription{},
		Components: []fetcher.FileDescription{},
	}
	for _, manifest := range manifests {
		manifest.Message = strings.TrimSpace(manifest.Message)
		if manifest.Message != "" {
			if executable.Message != "" {
				executable.Message += "\n"
			}
			executable.Message += manifest.Message
		}
		for _, file := range manifest.Files {
			fileDesc, err := maker.Fetcher.Fetch(file)
			if err != nil {
				return Executable{}, errors.Wrap(err, "error fetching file")
			}
			executable.Files = append(executable.Files, fileDesc)
		}
		for _, component := range manifest.Components {
			componentDesc, err := maker.Fetcher.Fetch(component)
			if err != nil {
				return Executable{}, errors.Wrap(err, "error fetching component")
			}
			executable.Components = append(executable.Components, componentDesc)
		}
	}
	return executable, nil
}

func (maker *Maker) fetchManifest(ctx context.Context, manifestUrl string) (Manifest, error) {
	manifestFile, err := maker.Fetcher.Fetch(manifestUrl)
	if err != nil {
		return Manifest{}, errors.Wrap(err, "error fetching manifest")
	}

	container := NewConfigContainer()
	err = maker.ParseFiles(ctx, []fetcher.FileDescription{manifestFile}, container)
	if err != nil {
		return Manifest{}, errors.Wrap(err, "error parsing manifest")
	}

	manifest := Manifest{}
	message := container.GetDataBagGroup("message", "")
	for i, msg := range message {
		str, err := ExtractAsStringValue(msg.Value)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("error extracting 'messages[%d]' from manifest", i)
			continue
		}
		if str != "" {
			if manifest.Message != "" {
				manifest.Message += "\n"
			}
			manifest.Message += str
		}
	}

	files := container.GetDataBagGroup("files", "")
	for i, file := range files {
		str, err := interpretAsStrArray(file.Value)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("error extracting 'files[%d]' from manifest", i)
			continue
		}
		manifest.Files = append(manifest.Files, str...)
	}

	components := container.GetDataBagGroup("components", "")
	for i, component := range components {
		str, err := interpretAsStrArray(component.Value)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("error extracting 'components[%d]' from manifest", i)
			continue
		}
		manifest.Components = append(manifest.Components, str...)
	}

	manifests := container.GetDataBagGroup("manifests", "")
	for i, m := range manifests {
		str, err := interpretAsStrArray(m.Value)
		if err != nil {
			log.Ctx(ctx).Warn().Err(err).Msgf("error extracting 'manifests[%d]' from manifest", i)
			continue
		}
		manifest.Manifests = append(manifest.Components, str...)
	}
	return manifest, nil
}
