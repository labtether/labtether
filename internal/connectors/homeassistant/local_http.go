package homeassistant

import (
	"context"
	"io"
	"net/http"

	"github.com/labtether/labtether/internal/securityruntime"
)

// newRequest builds a request for this connection: through the local-network
// client when its owner allowed plain http on the local network, otherwise
// through the Hub's outbound policy.
func (c *Connector) newRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	if !c.localHTTP {
		return securityruntime.NewOutboundRequestWithContext(ctx, method, rawURL, body)
	}
	if c.localHTTPErr != nil {
		return nil, c.localHTTPErr
	}
	parsed, err := securityruntime.ValidateLocalHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- host checked by ValidateLocalHTTPURL; the client re-checks every resolved address on connect.
	return http.NewRequestWithContext(ctx, method, parsed.String(), body)
}

func (c *Connector) do(req *http.Request) (*http.Response, error) {
	if !c.localHTTP {
		return securityruntime.DoOutboundRequest(c.httpClient, req)
	}
	if c.localHTTPErr != nil {
		return nil, c.localHTTPErr
	}
	if _, err := securityruntime.ValidateLocalHTTPURL(req.URL.String()); err != nil {
		return nil, err
	}
	// #nosec G704 -- local-network client: one origin, every resolved address checked on connect.
	return c.httpClient.Do(req)
}
