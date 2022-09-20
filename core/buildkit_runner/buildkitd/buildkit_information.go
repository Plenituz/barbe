package buildkitd

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/docker/distribution/reference"
)

// almost unchanged from https://github.com/dagger/dagger/blob/main/util/buildkitd/buildkit_information.go

func getBuildkitInformation(ctx context.Context) (*BuildkitInformation, error) {
	formatString := "{{.Config.Image}};{{.State.Running}};{{if index .NetworkSettings.Networks \"host\"}}{{\"true\"}}{{else}}{{\"false\"}}{{end}}"
	cmd := exec.CommandContext(ctx,
		"docker",
		"inspect",
		"--format",
		formatString,
		containerName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	s := strings.Split(string(output), ";")

	// Retrieve the tag
	ref, err := reference.ParseNormalizedNamed(strings.TrimSpace(s[0]))
	if err != nil {
		return nil, err
	}
	tag, ok := ref.(reference.Tagged)
	if !ok {
		return nil, fmt.Errorf("failed to parse image: %s", output)
	}

	// Retrieve the state
	isActive, err := strconv.ParseBool(strings.TrimSpace(s[1]))
	if err != nil {
		return nil, err
	}

	// Retrieve the check on if the host network is configured
	haveHostNetwork, err := strconv.ParseBool(strings.TrimSpace(s[2]))
	if err != nil {
		return nil, err
	}

	return &BuildkitInformation{
		Version:         tag.Tag(),
		IsActive:        isActive,
		HaveHostNetwork: haveHostNetwork,
	}, nil
}

type BuildkitInformation struct {
	Version         string
	IsActive        bool
	HaveHostNetwork bool
}
