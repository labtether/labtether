package collectors_test

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/incidents"
)

// From hub_collector_runner_autolink_test.go (moved from cmd/labtether, where cmd keeps a copy).

type stubDependencyStore struct {
	mu    sync.Mutex
	deps  map[string]dependencies.Dependency
	next  int
	links map[string]incidents.IncidentAsset
}

func newStubDependencyStore() *stubDependencyStore {
	return &stubDependencyStore{
		deps:  make(map[string]dependencies.Dependency),
		links: make(map[string]incidents.IncidentAsset),
	}
}

func (s *stubDependencyStore) CreateAssetDependency(req dependencies.CreateDependencyRequest) (dependencies.Dependency, error) {
	sourceID := strings.TrimSpace(req.SourceAssetID)
	targetID := strings.TrimSpace(req.TargetAssetID)
	relType := strings.TrimSpace(req.RelationshipType)
	if sourceID == targetID {
		return dependencies.Dependency{}, dependencies.ErrSelfReference
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dep := range s.deps {
		if dep.SourceAssetID == sourceID && dep.TargetAssetID == targetID && dep.RelationshipType == relType {
			return dependencies.Dependency{}, dependencies.ErrDuplicateDependency
		}
	}

	s.next++
	now := time.Now().UTC()
	dep := dependencies.Dependency{
		ID:               fmt.Sprintf("dep-%d", s.next),
		SourceAssetID:    sourceID,
		TargetAssetID:    targetID,
		RelationshipType: relType,
		Direction:        dependencies.NormalizeDirection(req.Direction),
		Criticality:      dependencies.NormalizeCriticality(req.Criticality),
		Metadata:         cloneMap(req.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if dep.Direction == "" {
		dep.Direction = dependencies.DirectionDownstream
	}
	if dep.Criticality == "" {
		dep.Criticality = dependencies.CriticalityMedium
	}
	s.deps[dep.ID] = dep
	return dep, nil
}

func (s *stubDependencyStore) ListAssetDependencies(assetID string, limit int) ([]dependencies.Dependency, error) {
	if limit <= 0 {
		limit = 50
	}
	assetID = strings.TrimSpace(assetID)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]dependencies.Dependency, 0, len(s.deps))
	for _, dep := range s.deps {
		if dep.SourceAssetID == assetID || dep.TargetAssetID == assetID {
			copied := dep
			copied.Metadata = cloneMap(dep.Metadata)
			out = append(out, copied)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubDependencyStore) GetAssetDependency(id string) (dependencies.Dependency, bool, error) {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	dep, ok := s.deps[id]
	if !ok {
		return dependencies.Dependency{}, false, nil
	}
	dep.Metadata = cloneMap(dep.Metadata)
	return dep, true, nil
}

func (s *stubDependencyStore) DeleteAssetDependency(id string) error {
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.deps[id]; !ok {
		return dependencies.ErrDependencyNotFound
	}
	delete(s.deps, id)
	return nil
}

func (s *stubDependencyStore) BlastRadius(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *stubDependencyStore) UpstreamCauses(assetID string, maxDepth int) ([]dependencies.ImpactNode, error) {
	return nil, nil
}

func (s *stubDependencyStore) LinkIncidentAsset(incidentID string, req incidents.LinkAssetRequest) (incidents.IncidentAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	link := incidents.IncidentAsset{
		ID:         fmt.Sprintf("ia-%d", s.next),
		IncidentID: strings.TrimSpace(incidentID),
		AssetID:    strings.TrimSpace(req.AssetID),
		Role:       incidents.NormalizeAssetRole(req.Role),
		CreatedAt:  time.Now().UTC(),
	}
	s.links[link.ID] = link
	return link, nil
}

func (s *stubDependencyStore) ListIncidentAssets(incidentID string, limit int) ([]incidents.IncidentAsset, error) {
	if limit <= 0 {
		limit = 50
	}
	incidentID = strings.TrimSpace(incidentID)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]incidents.IncidentAsset, 0, len(s.links))
	for _, link := range s.links {
		if link.IncidentID == incidentID {
			out = append(out, link)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubDependencyStore) UnlinkIncidentAsset(incidentID, linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.links, strings.TrimSpace(linkID))
	return nil
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
