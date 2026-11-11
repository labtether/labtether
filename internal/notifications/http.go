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

// notificationReceiver names who answered and what proves who sends, so a
// failed send says "Gotify did not accept the app token" instead of a bare
// status code.
var notificationReceiver = map[string]struct{ name, credential string }{
	"gotify":  {"Gotify", "app token"},
	"ntfy":    {"ntfy", "access token or password"},
	"slack":   {"Slack", "webhook URL"},
	"webhook": {"The webhook receiver", "credentials"},
}

func notificationResponseError(channel string, status int) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	receiver, ok := notificationReceiver[channel]
	if !ok {
		receiver.name, receiver.credential = channel, "credentials"
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%s did not accept the %s (HTTP %d)", receiver.name, receiver.credential, status)
	case status == http.StatusNotFound || status == http.StatusGone:
		return fmt.Errorf("%s has nothing at this address (HTTP %d)", receiver.name, status)
	case status == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%s refused the message as too large (HTTP %d)", receiver.name, status)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%s is limiting messages right now (HTTP %d)", receiver.name, status)
	case status >= http.StatusInternalServerError:
		return fmt.Errorf("%s had a server error (HTTP %d)", receiver.name, status)
	case status >= http.StatusBadRequest:
		return fmt.Errorf("%s refused the message (HTTP %d)", receiver.name, status)
	default:
		return fmt.Errorf("%s answered HTTP %d instead of taking the message", receiver.name, status)
	}
}
