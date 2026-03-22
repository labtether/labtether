package alerting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/incidents"
)

var (
	errIncidentGroupStoreUnavailable = errors.New("group store unavailable")
	errIncidentAssetStoreUnavailable = errors.New("asset store unavailable")
)

func normalizeCreateIncidentRequest(req *incidents.CreateIncidentRequest) {
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Severity = strings.TrimSpace(req.Severity)
	req.Source = strings.TrimSpace(req.Source)
	req.GroupID = strings.TrimSpace(req.GroupID)
	req.PrimaryAssetID = strings.TrimSpace(req.PrimaryAssetID)
	req.Assignee = strings.TrimSpace(req.Assignee)
	req.CreatedBy = strings.TrimSpace(req.CreatedBy)
	req.Metadata = cloneMetadata(req.Metadata)
}

func validateCreateIncidentRequest(req incidents.CreateIncidentRequest) error {
	if req.Title == "" {
		return errors.New("title is required")
	}
	if err := validateMaxLen("title", req.Title, MaxIncidentTitleLength); err != nil {
		return err
	}
	if err := validateMaxLen("summary", req.Summary, MaxIncidentSummaryLen); err != nil {
		return err
	}
	if req.Severity == "" {
		return errors.New("severity is required")
	}
	if incidents.NormalizeSeverity(req.Severity) == "" {
		return errors.New("severity must be critical, high, medium, or low")
	}
	if req.Source != "" && incidents.NormalizeSource(req.Source) == "" {
		return errors.New("source must be manual or alert_auto")
	}
	if err := validateMaxLen("group_id", req.GroupID, MaxIncidentLinkIDLength); err != nil {
		return err
	}
	if err := validateMaxLen("primary_asset_id", req.PrimaryAssetID, MaxIncidentLinkIDLength); err != nil {
		return err
	}
	if err := validateMaxLen("assignee", req.Assignee, MaxActorIDLength); err != nil {
		return err
	}
	if err := validateMaxLen("created_by", req.CreatedBy, MaxActorIDLength); err != nil {
		return err
	}
	return nil
}

func normalizeUpdateIncidentRequest(req *incidents.UpdateIncidentRequest) {
	req.Title = trimStringPtr(req.Title)
	req.Summary = trimStringPtr(req.Summary)
	req.Status = trimStringPtr(req.Status)
	req.Severity = trimStringPtr(req.Severity)
	req.GroupID = trimStringPtr(req.GroupID)
	req.PrimaryAssetID = trimStringPtr(req.PrimaryAssetID)
	req.Assignee = trimStringPtr(req.Assignee)
	if req.Metadata != nil {
		normalized := cloneMetadata(*req.Metadata)
		req.Metadata = &normalized
	}
}

func validateUpdateIncidentRequest(req incidents.UpdateIncidentRequest) error {
	if req.Title != nil {
		if *req.Title == "" {
			return errors.New("title cannot be empty")
		}
		if err := validateMaxLen("title", *req.Title, MaxIncidentTitleLength); err != nil {
			return err
		}
	}
	if req.Summary != nil {
		if err := validateMaxLen("summary", *req.Summary, MaxIncidentSummaryLen); err != nil {
			return err
		}
	}
	if req.Status != nil {
		if incidents.NormalizeStatus(*req.Status) == "" {
			return errors.New("invalid incident status")
		}
	}
	if req.Severity != nil {
		if incidents.NormalizeSeverity(*req.Severity) == "" {
			return errors.New("invalid incident severity")
		}
	}
	if req.GroupID != nil {
		if err := validateMaxLen("group_id", *req.GroupID, MaxIncidentLinkIDLength); err != nil {
			return err
		}
	}
	if req.PrimaryAssetID != nil {
		if err := validateMaxLen("primary_asset_id", *req.PrimaryAssetID, MaxIncidentLinkIDLength); err != nil {
			return err
		}
	}
	if req.Assignee != nil {
		if err := validateMaxLen("assignee", *req.Assignee, MaxActorIDLength); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deps) validateIncidentReferences(groupID, assetID string) error {
	groupID = strings.TrimSpace(groupID)
	assetID = strings.TrimSpace(assetID)
	if groupID != "" {
		if d.GroupStore == nil {
			return errIncidentGroupStoreUnavailable
		}
		if _, ok, err := d.GroupStore.GetGroup(groupID); err != nil {
			return err
		} else if !ok {
			return groups.ErrGroupNotFound
		}
	}
	if assetID != "" {
		if d.AssetStore == nil {
			return errIncidentAssetStoreUnavailable
		}
		if _, ok, err := d.AssetStore.GetAsset(assetID); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("asset not found: %s", assetID)
		}
	}
	return nil
}

func normalizeLinkAlertRequest(req *incidents.LinkAlertRequest) {
	req.AlertRuleID = strings.TrimSpace(req.AlertRuleID)
	req.AlertInstanceID = strings.TrimSpace(req.AlertInstanceID)
	req.AlertFingerprint = strings.TrimSpace(req.AlertFingerprint)
	req.LinkType = strings.TrimSpace(req.LinkType)
	req.CreatedBy = strings.TrimSpace(req.CreatedBy)
}

func validateLinkAlertRequest(req incidents.LinkAlertRequest) error {
	if req.LinkType == "" {
		return errors.New("link_type is required")
	}
	if incidents.NormalizeLinkType(req.LinkType) == "" {
		return errors.New("link_type must be trigger, related, symptom, or cause")
	}
	if req.AlertRuleID == "" && req.AlertInstanceID == "" && req.AlertFingerprint == "" {
		return errors.New("one of alert_rule_id, alert_instance_id, or alert_fingerprint is required")
	}
	if err := validateMaxLen("alert_rule_id", req.AlertRuleID, MaxIncidentLinkIDLength); err != nil {
		return err
	}
	if err := validateMaxLen("alert_instance_id", req.AlertInstanceID, MaxIncidentLinkIDLength); err != nil {
		return err
	}
	if err := validateMaxLen("alert_fingerprint", req.AlertFingerprint, MaxIncidentSummaryLen); err != nil {
		return err
	}
	if err := validateMaxLen("created_by", req.CreatedBy, MaxActorIDLength); err != nil {
		return err
	}
	return nil
}
