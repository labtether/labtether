package collectors

import "sync"

// WebServiceGroupingSuggestion represents a below-threshold-but-plausible URL
// grouping match (70-84% confidence). The frontend displays these as toast
// notifications so the operator can accept or deny them.
type WebServiceGroupingSuggestion struct {
	ID              string `json:"id"`
	BaseServiceURL  string `json:"base_service_url"`
	BaseServiceName string `json:"base_service_name"`
	BaseIconKey     string `json:"base_icon_key"`
	SuggestedURL    string `json:"suggested_url"`
	Confidence      int    `json:"confidence"`
}

// suggestionMinConfidence is the lower bound for generating a suggestion.
// Matches below this threshold are ignored entirely.
const suggestionMinConfidence = 70

// removeSuggestion removes a suggestion by ID from the slice in-place and
// returns the removed suggestion (if found) plus a boolean indicating success.
func removeSuggestion(suggestions []WebServiceGroupingSuggestion, id string) ([]WebServiceGroupingSuggestion, WebServiceGroupingSuggestion, bool) {
	for i, s := range suggestions {
		if s.ID == id {
			found := suggestions[i]
			suggestions = append(suggestions[:i], suggestions[i+1:]...)
			return suggestions, found, true
		}
	}
	return suggestions, WebServiceGroupingSuggestion{}, false
}

// Compile-time assertion that sync.Mutex is usable as a field type.
var _ sync.Locker = (*sync.Mutex)(nil)
