package apiv2

import (
	"context"
	"testing"
)

func TestIsOwnerPrincipalUsesRoleInsteadOfActorID(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{
			name: "generated owner user ID",
			ctx:  ContextWithPrincipal(context.Background(), "usr_01JOWNER", "owner"),
			want: true,
		},
		{
			name: "legacy owner bearer principal",
			ctx:  ContextWithPrincipal(context.Background(), "owner", "owner"),
			want: true,
		},
		{
			name: "owner actor ID without owner role",
			ctx:  ContextWithPrincipal(context.Background(), "owner", "admin"),
			want: false,
		},
		{
			name: "admin",
			ctx:  ContextWithPrincipal(context.Background(), "usr_01JADMIN", "admin"),
			want: false,
		},
		{
			name: "owner role API key",
			ctx: ContextWithAPIKeyID(
				ContextWithPrincipal(context.Background(), "apikey:key_01JOWNER", "owner"),
				"key_01JOWNER",
			),
			want: false,
		},
		{
			name: "owner role API key actor without marker",
			ctx:  ContextWithPrincipal(context.Background(), "apikey:key_01JOWNER", "owner"),
			want: false,
		},
		{
			name: "missing principal",
			ctx:  context.Background(),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOwnerPrincipal(tc.ctx); got != tc.want {
				t.Fatalf("IsOwnerPrincipal() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsAPIKeyPrincipal(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{
			name: "explicit key marker",
			ctx:  ContextWithAPIKeyID(ContextWithPrincipal(context.Background(), "apikey:key_01", "admin"), "key_01"),
			want: true,
		},
		{
			name: "legacy actor prefix",
			ctx:  ContextWithPrincipal(context.Background(), "apikey:key_legacy", "admin"),
			want: true,
		},
		{
			name: "interactive admin",
			ctx:  ContextWithPrincipal(context.Background(), "usr_01", "admin"),
			want: false,
		},
		{
			name: "owner bearer",
			ctx:  ContextWithPrincipal(context.Background(), "owner", "owner"),
			want: false,
		},
		{
			name: "missing principal",
			ctx:  context.Background(),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAPIKeyPrincipal(tc.ctx); got != tc.want {
				t.Fatalf("IsAPIKeyPrincipal() = %v, want %v", got, tc.want)
			}
		})
	}
}
