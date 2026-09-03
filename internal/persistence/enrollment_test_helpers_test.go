package persistence

import (
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

func testEnrollmentTokenParams(tokenHash, label string, expiresAt time.Time, maxUses int) CreateEnrollmentTokenParams {
	return CreateEnrollmentTokenParams{
		TokenHash: tokenHash,
		Label:     label,
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		Scope:     enrollment.TokenScopeUnrestricted,
		CreatedBy: "test",
	}
}

func testAssetEnrollmentTokenParams(tokenHash, assetID string, expiresAt time.Time) CreateEnrollmentTokenParams {
	return CreateEnrollmentTokenParams{
		TokenHash: tokenHash,
		Label:     "asset",
		ExpiresAt: expiresAt,
		MaxUses:   1,
		Scope:     enrollment.TokenScopeAsset,
		AssetID:   assetID,
		CreatedBy: "test",
	}
}

func testGroupEnrollmentTokenParams(tokenHash, groupID string, expiresAt time.Time, maxUses int) CreateEnrollmentTokenParams {
	return CreateEnrollmentTokenParams{
		TokenHash:      tokenHash,
		Label:          "group",
		ExpiresAt:      expiresAt,
		MaxUses:        maxUses,
		Scope:          enrollment.TokenScopeGroup,
		AllowedGroupID: groupID,
		CreatedBy:      "test",
	}
}

func testUnplacedEnrollmentTokenParams(tokenHash string, expiresAt time.Time, maxUses int) CreateEnrollmentTokenParams {
	return CreateEnrollmentTokenParams{
		TokenHash: tokenHash,
		Label:     "unplaced",
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		Scope:     enrollment.TokenScopeUnplaced,
		CreatedBy: "test",
	}
}
