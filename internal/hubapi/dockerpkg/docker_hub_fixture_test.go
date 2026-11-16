package dockerpkg_test

import (
	"testing"

	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

// newDockerHub stands for cmd's &apiServer{dockerCoordinator: coord}: a Hub
// with no stores and only the given Docker coordinator.
func newDockerHub(t *testing.T, coord *docker.Coordinator) *hubdeps.Hub {
	h := hubdeps.NewEmpty(t)
	h.DockerCoordinator = coord
	return h
}
