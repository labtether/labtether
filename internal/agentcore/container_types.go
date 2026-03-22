package agentcore

import (
	dockerpkg "github.com/labtether/labtether/internal/agentcore/docker"
)

// ---------------------------------------------------------------------------
// Docker API response types — canonical definitions live in docker/ subpackage.
// These type aliases keep existing root code working without modification.
// ---------------------------------------------------------------------------

type dockerContainer = dockerpkg.DockerContainer
type dockerPort = dockerpkg.DockerPort
type dockerMount = dockerpkg.DockerMount
type dockerImage = dockerpkg.DockerImage
type dockerNetwork = dockerpkg.DockerNetwork
type dockerVolumesResponse = dockerpkg.DockerVolumesResponse
type dockerVolume = dockerpkg.DockerVolume
type dockerVersionResponse = dockerpkg.DockerVersionResponse
type dockerEvent = dockerpkg.DockerEvent
type dockerStatsResponse = dockerpkg.DockerStatsResponse
type dockerPortBinding = dockerpkg.DockerPortBinding
type dockerContainerCreateRequest = dockerpkg.DockerContainerCreateRequest

// containerName is an alias for docker.ContainerName.
var containerName = dockerpkg.ContainerName
