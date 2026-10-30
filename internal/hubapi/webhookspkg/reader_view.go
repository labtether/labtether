package webhookspkg

import (
	"context"
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/webhooks"
)

// WebhookForReader hides the path and query of a webhook URL from roles that
// cannot edit webhooks. Chat and automation services put their access token in
// the URL, so it is as secret as a notification channel URL. Viewers still see
// which host receives the events.
func WebhookForReader(ctx context.Context, wh webhooks.Webhook) webhooks.Webhook {
	if auth.HasWritePrivileges(apiv2.UserRoleFromContext(ctx)) {
		return wh
	}
	wh.URL = webhookOrigin(wh.URL)
	return wh
}

func webhookOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
