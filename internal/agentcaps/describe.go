package agentcaps

import "strings"

// Advertisement is what the Hub reports to the console about one agent.
// Offered lists the request types the Hub sends to an explicit agent;
// update.request appears as update.request:self and update.request:packages.
type Advertisement struct {
	Explicit bool     `json:"explicit"`
	Edition  string   `json:"edition,omitempty"`
	Offered  []string `json:"offered,omitempty"`
}

// Describe returns the advertisement of an agent device whose agent is
// explicit or names a known edition. ok is false for any other device, which
// keeps the pages and actions it has today.
func Describe(source string, metadata map[string]string) (Advertisement, bool) {
	if !strings.EqualFold(strings.TrimSpace(source), "agent") {
		return Advertisement{}, false
	}
	ad := Advertisement{Explicit: Explicit(metadata), Edition: Edition(metadata), Offered: Offered(metadata)}
	if !ad.Explicit && ad.Edition == "" {
		return Advertisement{}, false
	}
	if ad.Explicit && ad.Offered == nil {
		ad.Offered = []string{}
	}
	return ad, true
}
