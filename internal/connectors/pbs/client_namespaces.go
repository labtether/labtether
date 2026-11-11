package pbs

import (
	"context"
	"fmt"
	neturl "net/url"
	"sort"
	"strings"
)

// maxNamespaces bounds how many namespaces of one datastore are read per call.
const maxNamespaces = 256

// Namespace is one namespace of a datastore; "" is the root namespace.
type Namespace struct {
	NS      string `json:"ns"`
	Comment string `json:"comment,omitempty"`
}

// ListNamespaces lists the namespaces of a datastore that the token may see.
func (c *Client) ListNamespaces(ctx context.Context, store string) ([]Namespace, error) {
	path := fmt.Sprintf("/api2/json/admin/datastore/%s/namespace", neturl.PathEscape(strings.TrimSpace(store)))
	var out []Namespace
	if err := c.getData(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListDatastoreGroups lists the backup groups of every namespace of a
// datastore that the token can read. Each group carries its namespace.
func (c *Client) ListDatastoreGroups(ctx context.Context, store string) ([]BackupGroup, error) {
	return listAcrossNamespaces(ctx, c, store, "groups", func(group *BackupGroup, ns string) { group.NS = ns })
}

// ListDatastoreSnapshots lists the snapshots of every namespace of a
// datastore that the token can read. Each snapshot carries its namespace.
func (c *Client) ListDatastoreSnapshots(ctx context.Context, store string) ([]BackupSnapshot, error) {
	return listAcrossNamespaces(ctx, c, store, "snapshots", func(snapshot *BackupSnapshot, ns string) { snapshot.NS = ns })
}

// namespacesToRead returns the root namespace plus every namespace PBS lists,
// root first. A PBS without namespaces (before 2.2), or one that refuses the
// listing, is read at the root alone.
func (c *Client) namespacesToRead(ctx context.Context, store string) []string {
	out := []string{""}
	listed, err := c.ListNamespaces(ctx, store)
	if err != nil {
		return out
	}
	seen := map[string]bool{"": true}
	for _, namespace := range listed {
		ns := strings.Trim(strings.TrimSpace(namespace.NS), "/")
		if seen[ns] {
			continue
		}
		seen[ns] = true
		out = append(out, ns)
	}
	sort.Strings(out[1:])
	if len(out) > maxNamespaces {
		out = out[:maxNamespaces]
	}
	return out
}

// namespaceQuery is the "&ns=" query suffix for a namespace; none for the root.
func namespaceQuery(ns string) string {
	ns = strings.Trim(strings.TrimSpace(ns), "/")
	if ns == "" {
		return ""
	}
	return "&ns=" + neturl.QueryEscape(ns)
}

// listAcrossNamespaces reads one datastore list (groups or snapshots) in each
// namespace. A token may read only some namespaces, so a namespace it may not
// read is skipped; the call fails only when no namespace could be read.
func listAcrossNamespaces[T any](ctx context.Context, c *Client, store, kind string, tag func(*T, string)) ([]T, error) {
	base := fmt.Sprintf("/api2/json/admin/datastore/%s/%s", neturl.PathEscape(strings.TrimSpace(store)), kind)
	var (
		out      []T
		firstErr error
		readOne  bool
	)
	for _, ns := range c.namespacesToRead(ctx, store) {
		path := base
		if ns != "" {
			path += "?ns=" + neturl.QueryEscape(ns)
		}
		var items []T
		if err := c.getData(ctx, path, &items); err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, err
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		readOne = true
		for i := range items {
			tag(&items[i], ns)
		}
		out = append(out, items...)
	}
	if !readOne && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}
