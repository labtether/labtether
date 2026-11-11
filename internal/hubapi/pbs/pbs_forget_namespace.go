package pbs

import (
	"context"
	"net/http"
	"strings"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/servicehttp"
)

const forgetNeedsNamespaceMessage = "This backup group is in a namespace. Name the namespace with ns (empty for the root)."

// forgetNamespace returns the namespace a forget request names, and false
// after it has answered the request itself. A request without "ns" means the
// root, as before namespaces were listed. It is refused when the same group
// exists in another namespace: listings now include namespaced groups, and a
// caller that leaves ns out must not delete a root group by mistake.
func forgetNamespace(ctx context.Context, w http.ResponseWriter, r *http.Request, client *pbsconnector.Client, store, backupType, backupID string) (string, bool) {
	if values, named := r.URL.Query()["ns"]; named {
		return strings.Trim(strings.TrimSpace(values[0]), "/"), true
	}
	groups, err := client.ListDatastoreGroups(ctx, store)
	if err != nil {
		writePBSError(w, http.StatusBadGateway, "could not check which namespace holds this backup group", err)
		return "", false
	}
	for _, group := range groups {
		if group.NS != "" && strings.EqualFold(strings.TrimSpace(group.BackupType), backupType) && strings.TrimSpace(group.BackupID) == backupID {
			servicehttp.WriteError(w, http.StatusBadRequest, forgetNeedsNamespaceMessage)
			return "", false
		}
	}
	return "", true
}
