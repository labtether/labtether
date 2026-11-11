package servicehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONBodyErrorAnswerSaysWhatIsWrong(t *testing.T) {
	decode := func(body string, limit int64) error {
		var dst struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		return json.NewDecoder(http.MaxBytesReader(httptest.NewRecorder(), r.Body, limit)).Decode(&dst)
	}
	cases := []struct {
		body   string
		limit  int64
		status int
		words  string
	}{
		{"", 1 << 20, http.StatusBadRequest, "The request needs a JSON body."},
		{"name=x", 1 << 20, http.StatusBadRequest, "The request body is not a valid JSON object."},
		{`{"count":"7"}`, 1 << 20, http.StatusBadRequest, "count must be a whole number."},
		{`{"name":"` + strings.Repeat("x", 64) + `"}`, 16, http.StatusRequestEntityTooLarge, "The request body is larger than 16 bytes."},
	}
	for _, tc := range cases {
		status, words := JSONBodyErrorAnswer(decode(tc.body, tc.limit))
		if status != tc.status || words != tc.words {
			t.Errorf("%q = %d %q, want %d %q", tc.body, status, words, tc.status, tc.words)
		}
	}
}
