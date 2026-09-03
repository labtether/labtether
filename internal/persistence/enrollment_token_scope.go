package persistence

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/enrollment"
)

func validateEnrollmentTokenCreateParams(params CreateEnrollmentTokenParams) error {
	if strings.TrimSpace(params.TokenHash) == "" {
		return fmt.Errorf("token hash is required")
	}
	if strings.TrimSpace(params.CreatedBy) == "" {
		return fmt.Errorf("token creator is required")
	}
	if err := enrollment.ValidateStoredTokenMaxUses(params.MaxUses); err != nil {
		return err
	}
	assetID := strings.TrimSpace(params.AssetID)
	groupID := strings.TrimSpace(params.AllowedGroupID)
	switch strings.TrimSpace(params.Scope) {
	case enrollment.TokenScopeAsset:
		if assetID == "" {
			return fmt.Errorf("asset scope requires an asset id")
		}
		if params.MaxUses != 1 {
			return fmt.Errorf("asset-scoped enrollment tokens must be single-use")
		}
	case enrollment.TokenScopeGroup:
		if assetID != "" || groupID == "" {
			return fmt.Errorf("group scope requires one allowed group and no asset id")
		}
	case enrollment.TokenScopeUnplaced:
		if assetID != "" || groupID != "" {
			return fmt.Errorf("unplaced scope cannot bind an asset or group")
		}
	case enrollment.TokenScopeUnrestricted:
		if assetID != "" || groupID != "" {
			return fmt.Errorf("unrestricted scope cannot bind an asset or group")
		}
	default:
		return fmt.Errorf("invalid enrollment token scope")
	}
	return nil
}

func initialEnrollmentGroupForToken(token enrollment.EnrollmentToken, assetID, requestedGroupID string) (string, error) {
	assetID = strings.TrimSpace(assetID)
	requestedGroupID = strings.TrimSpace(requestedGroupID)
	allowedGroupID := strings.TrimSpace(token.AllowedGroupID)

	switch token.Scope {
	case enrollment.TokenScopeAsset:
		if strings.TrimSpace(token.AssetID) != assetID {
			return "", ErrEnrollmentTokenScopeMismatch
		}
		if requestedGroupID != "" && requestedGroupID != allowedGroupID {
			return "", ErrEnrollmentTokenScopeMismatch
		}
		return allowedGroupID, nil
	case enrollment.TokenScopeGroup:
		if requestedGroupID != "" && requestedGroupID != allowedGroupID {
			return "", ErrEnrollmentTokenScopeMismatch
		}
		return allowedGroupID, nil
	case enrollment.TokenScopeUnplaced:
		if requestedGroupID != "" {
			return "", ErrEnrollmentTokenScopeMismatch
		}
		return "", nil
	case enrollment.TokenScopeUnrestricted:
		return requestedGroupID, nil
	default:
		return "", ErrEnrollmentTokenScopeMismatch
	}
}

func validateEnrollmentRecoveryScope(token enrollment.EnrollmentToken, assetID, existingGroupID string) error {
	assetID = strings.TrimSpace(assetID)
	existingGroupID = strings.TrimSpace(existingGroupID)
	switch token.Scope {
	case enrollment.TokenScopeAsset:
		if strings.TrimSpace(token.AssetID) != assetID {
			return ErrEnrollmentTokenScopeMismatch
		}
	case enrollment.TokenScopeGroup:
		if strings.TrimSpace(token.AllowedGroupID) != existingGroupID {
			return ErrEnrollmentTokenScopeMismatch
		}
	case enrollment.TokenScopeUnplaced:
		if existingGroupID != "" {
			return ErrEnrollmentTokenScopeMismatch
		}
	case enrollment.TokenScopeUnrestricted:
		return nil
	default:
		return ErrEnrollmentTokenScopeMismatch
	}
	return nil
}
