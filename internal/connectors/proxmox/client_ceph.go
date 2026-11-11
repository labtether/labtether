package proxmox

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"
)

// ErrNoReadableNode means the saved login sees no online Proxmox node, so a
// node-level read such as the Ceph pools cannot be made.
var ErrNoReadableNode = errors.New("proxmox: no online node is visible to this login")

// Proxmox's own words when Ceph is not installed or not set up on a node.
var cephNotSetUpPattern = regexp.MustCompile(`(?i)binary not installed|not (yet )?(initialized|installed|configured)|rados_connect`)

// CephNotSetUp reports whether Proxmox refused a Ceph read because Ceph is
// not installed or not set up there.
func CephNotSetUp(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode < 500 {
		return false
	}
	return cephNotSetUpPattern.MatchString(apiErr.Reason) || cephNotSetUpPattern.MatchString(apiErr.Body)
}

// ListCephPools returns the Ceph pools. Proxmox lists them per node
// (/nodes/{node}/ceph/pool); every Ceph node answers for the whole cluster,
// so the first online node that answers is used.
func (c *Client) ListCephPools(ctx context.Context) ([]map[string]any, error) {
	resources, err := c.GetClusterResources(ctx)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, resource := range resources {
		node := strings.TrimSpace(resource.Node)
		if resource.Type != "node" || node == "" || (resource.Status != "" && resource.Status != "online") {
			continue
		}
		var pools []map[string]any
		path := fmt.Sprintf("/api2/json/nodes/%s/ceph/pool", neturl.PathEscape(node))
		if err := c.getData(ctx, path, &pools); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return pools, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrNoReadableNode
}
