package resources

import (
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

// Before 1.7.0 a WebDAV connection used plain HTTP unless its port was 443 or
// webdav_tls was true. 1.7.0 defaults to HTTPS and allows plain HTTP only with
// an explicit per-connection opt-in plus LABTETHER_ALLOW_INSECURE_TRANSPORT.
// Such saved connections must say why they stopped working instead of failing
// with a TLS handshake error or a generic message.
const legacyPlainWebDAVNotice = "This WebDAV connection uses plain HTTP, which LabTether 1.7.0 no longer allows by default. " +
	"Edit the connection to use HTTPS, or keep plain HTTP by setting webdav_allow_cleartext=true on the connection " +
	"and LABTETHER_ALLOW_INSECURE_TRANSPORT=true on the Hub."

var errLegacyPlainWebDAV = errors.New(legacyPlainWebDAVNotice)

// legacyPlainWebDAV reports a saved WebDAV connection that relies on plain
// HTTP without the 1.7.0 opt-in.
func legacyPlainWebDAV(fc *persistence.FileConnection) bool {
	if fc == nil || !strings.EqualFold(strings.TrimSpace(fc.Protocol), "webdav") {
		return false
	}
	rawTLS, stored := fc.ExtraConfig["webdav_tls"]
	if !stored {
		port := 0
		if fc.Port != nil {
			port = *fc.Port
		}
		if port == 0 {
			port = fileproto.DefaultPort("webdav")
		}
		return port != 443
	}
	if useTLS, ok := rawTLS.(bool); !ok || useTLS {
		return false
	}
	allowCleartext, _ := fc.ExtraConfig["webdav_allow_cleartext"].(bool)
	return !allowCleartext || !securityruntime.InsecureTransportAllowed()
}

// fileConnectionView adds operator-facing upgrade notices to a listed
// connection without changing its stored fields.
type fileConnectionView struct {
	persistence.FileConnection
	TransportNotice string `json:"transport_notice,omitempty"`
}

func fileConnectionViews(connections []persistence.FileConnection) []fileConnectionView {
	views := make([]fileConnectionView, 0, len(connections))
	for i := range connections {
		view := fileConnectionView{FileConnection: connections[i]}
		if legacyPlainWebDAV(&connections[i]) {
			view.TransportNotice = legacyPlainWebDAVNotice
		}
		view.ExtraConfig = sanitizeLegacyFileConnectionExtraConfig(view.Protocol, view.ExtraConfig)
		views = append(views, view)
	}
	return views
}
