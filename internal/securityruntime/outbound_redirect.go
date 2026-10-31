package securityruntime

import (
	"fmt"
	"net/http"
	"strings"
)

// crossHostRedirectHeaders are the only request headers kept when a redirect
// leaves the original host. net/http drops Authorization and Cookie there, but
// it forwards custom credential headers such as Portainer's X-API-Key or a
// remote collector's configured key header. A forward-auth or login redirect
// would otherwise hand that key to another server.
var crossHostRedirectHeaders = map[string]struct{}{
	"Accept":          {},
	"Accept-Language": {},
	"Content-Type":    {},
	"User-Agent":      {},
}

// limitCrossHostRedirect strips credentials from a redirect that leaves the
// original host, or moves from HTTPS to HTTP, and refuses one that would send
// the request body there again.
func limitCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || via[0] == nil || via[0].URL == nil {
		return nil
	}
	origin := via[0].URL
	sameHost := strings.EqualFold(origin.Hostname(), req.URL.Hostname())
	downgrade := strings.EqualFold(origin.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https")
	if sameHost && !downgrade {
		return nil
	}
	if req.Body != nil && req.Body != http.NoBody {
		return fmt.Errorf("redirect to %q would resend the request body to another host", req.URL.Hostname())
	}
	for name := range req.Header {
		if _, keep := crossHostRedirectHeaders[http.CanonicalHeaderKey(name)]; !keep {
			req.Header.Del(name)
		}
	}
	return nil
}
