package persistence

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

func TestEnrollmentTokenCreateParamsRejectUnsafeScopeCombinations(t *testing.T) {
	valid := CreateEnrollmentTokenParams{
		TokenHash: "hash", Label: "test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		MaxUses: 1, Scope: enrollment.TokenScopeUnplaced, CreatedBy: "operator",
	}
	tests := []struct {
		name   string
		mutate func(*CreateEnrollmentTokenParams)
	}{
		{name: "missing scope", mutate: func(p *CreateEnrollmentTokenParams) { p.Scope = "" }},
		{name: "missing creator", mutate: func(p *CreateEnrollmentTokenParams) { p.CreatedBy = "" }},
		{name: "asset missing id", mutate: func(p *CreateEnrollmentTokenParams) { p.Scope = enrollment.TokenScopeAsset }},
		{name: "asset multiple uses", mutate: func(p *CreateEnrollmentTokenParams) {
			p.Scope, p.AssetID, p.MaxUses = enrollment.TokenScopeAsset, "node", 2
		}},
		{name: "group missing id", mutate: func(p *CreateEnrollmentTokenParams) { p.Scope = enrollment.TokenScopeGroup }},
		{name: "unplaced with group", mutate: func(p *CreateEnrollmentTokenParams) { p.AllowedGroupID = "group" }},
		{name: "unrestricted with asset", mutate: func(p *CreateEnrollmentTokenParams) {
			p.Scope, p.AssetID = enrollment.TokenScopeUnrestricted, "node"
		}},
		{name: "legacy scope", mutate: func(p *CreateEnrollmentTokenParams) { p.Scope = enrollment.TokenScopeLegacyRevoked }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := valid
			tt.mutate(&params)
			if err := validateEnrollmentTokenCreateParams(params); err == nil {
				t.Fatal("expected unsafe token claims to be rejected")
			}
		})
	}
}
