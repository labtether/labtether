package persistence

import (
	"context"
	"strings"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

func (m *MemoryIncidentStore) CreateAutoIncidentForAlertInstanceContext(ctx context.Context, instanceID string, req incidents.CreateIncidentRequest) (incidents.Incident, bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return incidents.Incident{}, false, incidents.ErrAlertReferenceRequired
	}
	if err := lockAlertEvaluation(ctx, &m.mu, false); err != nil {
		return incidents.Incident{}, false, err
	}
	defer m.mu.Unlock()
	for incidentID, links := range m.links {
		if err := ctx.Err(); err != nil {
			return incidents.Incident{}, false, err
		}
		if m.incidents[incidentID].Source != incidents.SourceAlertAuto {
			continue
		}
		for _, link := range links {
			if err := ctx.Err(); err != nil {
				return incidents.Incident{}, false, err
			}
			if link.AlertInstanceID == instanceID {
				return incidents.Incident{}, false, nil
			}
		}
	}
	req.Source = incidents.SourceAlertAuto
	incident := newMemoryIncident(req)
	link := incidents.AlertLink{
		ID: idgen.New("inclink"), IncidentID: incident.ID, AlertInstanceID: instanceID,
		LinkType: incidents.LinkTypeTrigger, CreatedBy: incident.CreatedBy, CreatedAt: incident.CreatedAt,
	}
	if err := ctx.Err(); err != nil {
		return incidents.Incident{}, false, err
	}
	m.incidents[incident.ID] = cloneIncident(incident)
	m.links[incident.ID] = []incidents.AlertLink{link}
	return cloneIncident(incident), true, nil
}
