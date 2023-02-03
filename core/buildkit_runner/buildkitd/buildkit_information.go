package buildkitd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/docker/distribution/reference"
)

// some of this from https://github.com/dagger/dagger/blob/main/util/buildkitd/buildkit_information.go

func GetDockerSocketLocation() (string, error) {
	//docker context ls --format "{{json .}}"
	cmd := exec.Command("docker", "context", "ls", "--format", "{{json .}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	/*
		{"Current":true,"Description":"colima","DockerEndpoint":"unix:///Users/me/.colima/default/docker.sock","KubernetesEndpoint":"","Name":"colima","StackOrchestrator":""}
		{"Current":false,"Description":"Current DOCKER_HOST based configuration","DockerEndpoint":"unix:///var/run/docker.sock","KubernetesEndpoint":"","Name":"default","StackOrchestrator":"swarm"}
	*/
	var currentContext string
	for _, line := range strings.Split(string(output), "\n") {
		var contextInfo struct {
			Current        bool   `json:"Current"`
			DockerEndpoint string `json:"DockerEndpoint"`
		}
		if err := json.Unmarshal([]byte(line), &contextInfo); err != nil {
			return "", err
		}
		if contextInfo.Current {
			currentContext = contextInfo.DockerEndpoint
			break
		}
	}
	if currentContext == "" {
		return "", fmt.Errorf("could not find current context")
	}
	return strings.TrimPrefix(currentContext, "unix://"), nil
}

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
