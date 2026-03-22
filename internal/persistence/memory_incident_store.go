package persistence

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

type MemoryIncidentStore struct {
	mu        sync.RWMutex
	incidents map[string]incidents.Incident
	links     map[string][]incidents.AlertLink
}

func NewMemoryIncidentStore() *MemoryIncidentStore {
	return &MemoryIncidentStore{
		incidents: make(map[string]incidents.Incident),
		links:     make(map[string][]incidents.AlertLink),
	}
}

func (m *MemoryIncidentStore) CreateIncident(req incidents.CreateIncidentRequest) (incidents.Incident, error) {
	now := time.Now().UTC()
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	source := incidents.NormalizeSource(req.Source)
	if source == "" {
		source = incidents.SourceManual
	}
	severity := incidents.NormalizeSeverity(req.Severity)
	if severity == "" {
		severity = incidents.SeverityMedium
	}

	incident := incidents.Incident{
		ID:             idgen.New("inc"),
		Title:          strings.TrimSpace(req.Title),
		Summary:        strings.TrimSpace(req.Summary),
		Status:         incidents.StatusOpen,
		Severity:       severity,
		Source:         source,
		GroupID:        strings.TrimSpace(req.GroupID),
		PrimaryAssetID: strings.TrimSpace(req.PrimaryAssetID),
		Assignee:       strings.TrimSpace(req.Assignee),
		CreatedBy:      createdBy,
		OpenedAt:       now,
		Metadata:       cloneMetadata(req.Metadata),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	m.mu.Lock()
	m.incidents[incident.ID] = cloneIncident(incident)
	m.mu.Unlock()
	return cloneIncident(incident), nil
}

func (m *MemoryIncidentStore) GetIncident(id string) (incidents.Incident, bool, error) {
	m.mu.RLock()
	incident, ok := m.incidents[strings.TrimSpace(id)]
	m.mu.RUnlock()
	if !ok {
		return incidents.Incident{}, false, nil
	}
	return cloneIncident(incident), true, nil
}

func (m *MemoryIncidentStore) ListIncidents(filter IncidentFilter) ([]incidents.Incident, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	status := incidents.NormalizeStatus(filter.Status)
	severity := incidents.NormalizeSeverity(filter.Severity)
	source := incidents.NormalizeSource(filter.Source)
	groupID := strings.TrimSpace(filter.GroupID)
	assignee := strings.TrimSpace(filter.Assignee)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]incidents.Incident, 0, len(m.incidents))
	for _, incident := range m.incidents {
		if status != "" && incident.Status != status {
			continue
		}
		if severity != "" && incident.Severity != severity {
			continue
		}
		if source != "" && incident.Source != source {
			continue
		}
		if groupID != "" && incident.GroupID != groupID {
			continue
		}
		if assignee != "" && incident.Assignee != assignee {
			continue
		}
		out = append(out, cloneIncident(incident))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if filter.Offset > 0 {
		if filter.Offset >= len(out) {
			return []incidents.Incident{}, nil
		}
		out = out[filter.Offset:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryIncidentStore) UpdateIncident(id string, req incidents.UpdateIncidentRequest) (incidents.Incident, error) {
	now := time.Now().UTC()
	id = strings.TrimSpace(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	incident, ok := m.incidents[id]
	if !ok {
		return incidents.Incident{}, incidents.ErrIncidentNotFound
	}

	if req.Title != nil {
		incident.Title = strings.TrimSpace(*req.Title)
	}
	if req.Summary != nil {
		incident.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Severity != nil {
		nextSeverity := incidents.NormalizeSeverity(*req.Severity)
		if nextSeverity == "" {
			return incidents.Incident{}, errors.New("invalid incident severity")
		}
		incident.Severity = nextSeverity
	}
	if req.Assignee != nil {
		incident.Assignee = strings.TrimSpace(*req.Assignee)
	}
	if req.GroupID != nil {
		incident.GroupID = strings.TrimSpace(*req.GroupID)
	}
	if req.PrimaryAssetID != nil {
		incident.PrimaryAssetID = strings.TrimSpace(*req.PrimaryAssetID)
	}
	if req.Metadata != nil {
		incident.Metadata = cloneMetadata(*req.Metadata)
	}
	if req.RootCause != nil {
		incident.RootCause = strings.TrimSpace(*req.RootCause)
	}
	if req.ActionItems != nil {
		items := make([]string, len(*req.ActionItems))
		copy(items, *req.ActionItems)
		incident.ActionItems = items
	}
	if req.LessonsLearned != nil {
		incident.LessonsLearned = strings.TrimSpace(*req.LessonsLearned)
	}
	if req.Status != nil {
		nextStatus := incidents.NormalizeStatus(*req.Status)
		if nextStatus == "" || !incidents.CanTransitionStatus(incident.Status, nextStatus) {
			return incidents.Incident{}, incidents.ErrInvalidStatusTransition
		}
		if incident.Status != nextStatus {
			incident.Status = nextStatus
			switch nextStatus {
			case incidents.StatusMitigated:
				if incident.MitigatedAt == nil {
					incident.MitigatedAt = &now
				}
			case incidents.StatusResolved:
				if incident.ResolvedAt == nil {
					incident.ResolvedAt = &now
				}
			case incidents.StatusClosed:
				if incident.ClosedAt == nil {
					incident.ClosedAt = &now
				}
			}
		}
	}

	incident.UpdatedAt = now
	m.incidents[id] = cloneIncident(incident)
	return cloneIncident(incident), nil
}

func (m *MemoryIncidentStore) DeleteIncident(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if _, ok := m.incidents[id]; !ok {
		return incidents.ErrIncidentNotFound
	}
	delete(m.incidents, id)
	delete(m.links, id)
	return nil
}

func (m *MemoryIncidentStore) LinkIncidentAlert(incidentID string, req incidents.LinkAlertRequest) (incidents.AlertLink, error) {
	now := time.Now().UTC()
	incidentID = strings.TrimSpace(incidentID)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.incidents[incidentID]; !ok {
		return incidents.AlertLink{}, incidents.ErrIncidentNotFound
	}

	linkType := incidents.NormalizeLinkType(req.LinkType)
	if linkType == "" {
		return incidents.AlertLink{}, errors.New("invalid incident alert link_type")
	}

	alertRuleID := strings.TrimSpace(req.AlertRuleID)
	alertInstanceID := strings.TrimSpace(req.AlertInstanceID)
	alertFingerprint := strings.TrimSpace(req.AlertFingerprint)
	if alertRuleID == "" && alertInstanceID == "" && alertFingerprint == "" {
		return incidents.AlertLink{}, incidents.ErrAlertReferenceRequired
	}

	existing := m.links[incidentID]
	for _, link := range existing {
		if alertRuleID != "" && link.AlertRuleID == alertRuleID {
			return incidents.AlertLink{}, incidents.ErrIncidentAlertLinkConflict
		}
		if alertInstanceID != "" && link.AlertInstanceID == alertInstanceID {
			return incidents.AlertLink{}, incidents.ErrIncidentAlertLinkConflict
		}
	}

	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	link := incidents.AlertLink{
		ID:               idgen.New("inclink"),
		IncidentID:       incidentID,
		AlertRuleID:      alertRuleID,
		AlertInstanceID:  alertInstanceID,
		AlertFingerprint: alertFingerprint,
		LinkType:         linkType,
		CreatedBy:        createdBy,
		CreatedAt:        now,
	}

	m.links[incidentID] = append(m.links[incidentID], link)
	return cloneIncidentAlertLink(link), nil
}

func (m *MemoryIncidentStore) ListIncidentAlertLinks(incidentID string, limit int) ([]incidents.AlertLink, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	incidentID = strings.TrimSpace(incidentID)

	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.incidents[incidentID]; !ok {
		return nil, incidents.ErrIncidentNotFound
	}

	links := m.links[incidentID]
	out := make([]incidents.AlertLink, 0, len(links))
	for _, link := range links {
		out = append(out, cloneIncidentAlertLink(link))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryIncidentStore) UnlinkIncidentAlert(incidentID, linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	incidentID = strings.TrimSpace(incidentID)
	linkID = strings.TrimSpace(linkID)
	if _, ok := m.incidents[incidentID]; !ok {
		return incidents.ErrIncidentNotFound
	}
	links := m.links[incidentID]
	for i, link := range links {
		if link.ID == linkID {
			m.links[incidentID] = append(links[:i], links[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (m *MemoryIncidentStore) HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error) {
	alertInstanceID = strings.TrimSpace(alertInstanceID)
	if alertInstanceID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for incidentID, links := range m.links {
		incident, ok := m.incidents[incidentID]
		if !ok || incident.Source != incidents.SourceAlertAuto {
			continue
		}
		for _, link := range links {
			if strings.TrimSpace(link.AlertInstanceID) == alertInstanceID {
				return true, nil
			}
		}
	}
	return false, nil
}

func cloneIncident(input incidents.Incident) incidents.Incident {
	out := input
	out.Metadata = cloneMetadata(input.Metadata)
	if len(input.ActionItems) > 0 {
		out.ActionItems = make([]string, len(input.ActionItems))
		copy(out.ActionItems, input.ActionItems)
	}
	if input.MitigatedAt != nil {
		value := input.MitigatedAt.UTC()
		out.MitigatedAt = &value
	}
	if input.ResolvedAt != nil {
		value := input.ResolvedAt.UTC()
		out.ResolvedAt = &value
	}
	if input.ClosedAt != nil {
		value := input.ClosedAt.UTC()
		out.ClosedAt = &value
	}
	return out
}

func cloneIncidentAlertLink(input incidents.AlertLink) incidents.AlertLink {
	out := input
	out.CreatedAt = input.CreatedAt.UTC()
	return out
}
