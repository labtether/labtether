package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

func TestRejectAPIKeyIdentityMutation(t *testing.T) {
	apiKeyContext := apiv2.ContextWithAPIKeyID(
		apiv2.ContextWithPrincipal(context.Background(), "apikey:key_test", "admin"),
		"key_test",
	)

	tests := []struct {
		name    string
		context context.Context
		want    bool
	}{
		{name: "API key", context: apiKeyContext, want: true},
		{name: "interactive admin", context: apiv2.ContextWithPrincipal(context.Background(), "usr_admin", "admin"), want: false},
		{name: "owner bearer", context: apiv2.ContextWithPrincipal(context.Background(), "owner", "owner"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/identity", nil).WithContext(tc.context)
			rec := httptest.NewRecorder()
			if got := rejectAPIKeyIdentityMutation(rec, req); got != tc.want {
				t.Fatalf("rejectAPIKeyIdentityMutation() = %v, want %v", got, tc.want)
			}
			if tc.want && rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		})
	}
}
