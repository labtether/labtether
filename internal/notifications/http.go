package notifications

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const notificationHTTPTimeout = 10 * time.Second

// newNotificationHTTPClient prevents notification credentials and custom
// headers from following redirects to a different origin. The security runtime
// independently revalidates every redirect target against the outbound policy.
func newNotificationHTTPClient() *http.Client {
	return &http.Client{
		Timeout: notificationHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req == nil || req.URL == nil || len(via) == 0 || via[0] == nil || via[0].URL == nil {
				return fmt.Errorf("notification redirect target and origin are required")
			}
			if len(via) >= 5 {
				return fmt.Errorf("notification redirect limit exceeded")
			}
			if !sameNotificationOrigin(via[0].URL, req.URL) {
				return fmt.Errorf("notification redirect to a different origin is not allowed")
			}
			return nil
		},
	}
}

func sameNotificationOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectiveNotificationPort(a) == effectiveNotificationPort(b)
}

func effectiveNotificationPort(value *url.URL) string {
	if value == nil {
		return ""
	}
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func notificationResponseError(channel string, status int) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	return fmt.Errorf("%s returned status %d", channel, status)
}
