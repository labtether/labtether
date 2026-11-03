package main

import (
	"log"
	"strings"
)

// resolveTLSModeSetting accepts the three LABTETHER_TLS_MODE values. Any other
// value used to be served like "auto" while the Hub recorded the unknown text
// as its mode, without a word in the log; it now says so and uses "auto".
func resolveTLSModeSetting(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "auto", "external", "disabled":
		return mode
	case "":
		return "auto"
	}
	log.Printf("labtether warning: LABTETHER_TLS_MODE=%q is not auto, external or disabled; using auto", raw)
	return "auto"
}
