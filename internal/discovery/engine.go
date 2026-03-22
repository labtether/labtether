package discovery

import (
	"fmt"
	"sort"

	"github.com/labtether/labtether/internal/edges"
)

// Engine orchestrates asset discovery: it runs all matchers, deduplicates and
// aggregates candidate confidence, checks for existing edges, then creates or
// suggests edges based on threshold.
type Engine struct {
	store    edges.Store
	matchers []Matcher
}

// NewEngine returns an Engine wired with the default matcher set.
// ParentHintMatcher is created dynamically in Run() because it requires the
// full asset list at execution time.
func NewEngine(store edges.Store) *Engine {
	return &Engine{
		store: store,
		matchers: []Matcher{
			&IPMatcher{},
			&HostnameMatcher{},
			&NameTokenMatcher{},
			&StructuralMatcher{},
		},
	}
}

// pairMeta aggregates all match candidates for a single (source, target) pair.
type pairMeta struct {
	sourceAssetID string
	targetAssetID string
	// For edge candidates we track the EdgeType of the first (highest-priority) candidate.
	edgeType string
	// candType is "edge" or "composite" — set by the first candidate for this pair.
	candType    string
	confidences []float64
	signals     []string
}

// Run executes discovery on the given assets, creating or suggesting edges.
// It returns the number of auto-created edges and suggested edges, or an error.
func (e *Engine) Run(assets []AssetData) (created int, suggested int, err error) {
	if len(assets) == 0 {
		return 0, 0, nil
	}

	// 1. Extract signals from all assets.
	signals := make([]AssetSignals, 0, len(assets))
	for _, a := range assets {
		signals = append(signals, ExtractSignals(a))
	}

	// 2. Build matcher list with ParentHintMatcher prepended.
	allMatchers := make([]Matcher, 0, len(e.matchers)+1)
	allMatchers = append(allMatchers, &ParentHintMatcher{AllAssets: assets})
	allMatchers = append(allMatchers, e.matchers...)

	// 3. Run each matcher, collect all candidates.
	var allCandidates []MatchCandidate
	for _, m := range allMatchers {
		allCandidates = append(allCandidates, m.Match(signals)...)
	}

	if len(allCandidates) == 0 {
		return 0, 0, nil
	}

	// 4. Group candidates by sorted (source, target) pair key.
	pairMap := make(map[[2]string]*pairMeta)
	for _, c := range allCandidates {
		key := pairKey(c.SourceAssetID, c.TargetAssetID)
		pm, exists := pairMap[key]
		if !exists {
			pm = &pairMeta{
				// Preserve the canonical source/target orientation from the first candidate.
				sourceAssetID: c.SourceAssetID,
				targetAssetID: c.TargetAssetID,
				edgeType:      c.EdgeType,
				candType:      c.Type,
			}
			pairMap[key] = pm
		}
		pm.confidences = append(pm.confidences, c.Confidence)
		if c.Signal != "" {
			pm.signals = append(pm.signals, c.Signal)
		}
	}

	// 5. Aggregate confidence per pair.
	type pairResult struct {
		meta       *pairMeta
		confidence float64
	}
	results := make([]pairResult, 0, len(pairMap))
	for _, pm := range pairMap {
		conf := edges.AggregateConfidence(pm.confidences)
		results = append(results, pairResult{meta: pm, confidence: conf})
	}

	// Collect all involved asset IDs for the batch edge lookup.
	assetIDSet := make(map[string]struct{}, len(assets)*2)
	for _, r := range results {
		assetIDSet[r.meta.sourceAssetID] = struct{}{}
		assetIDSet[r.meta.targetAssetID] = struct{}{}
	}
	assetIDs := make([]string, 0, len(assetIDSet))
	for id := range assetIDSet {
		assetIDs = append(assetIDs, id)
	}
	sort.Strings(assetIDs) // deterministic order

	// 6. Check existing edges — fetch all edges for involved assets.
	existingEdges, err := e.store.ListEdgesBatch(assetIDs, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("discovery engine: list edges batch: %w", err)
	}
	// Build a set of existing (source, target) and (target, source) pairs so we
	// can skip any pair that already has an edge regardless of direction.
	existingPairs := make(map[[2]string]struct{}, len(existingEdges)*2)
	for _, edge := range existingEdges {
		k := pairKey(edge.SourceAssetID, edge.TargetAssetID)
		existingPairs[k] = struct{}{}
	}

	// 7. Apply thresholds and create edges.
	for _, r := range results {
		key := pairKey(r.meta.sourceAssetID, r.meta.targetAssetID)
		if _, exists := existingPairs[key]; exists {
			// Already has an edge of any origin — skip.
			continue
		}

		conf := r.confidence

		var origin string
		switch {
		case conf >= 0.90:
			origin = edges.OriginAuto
		case conf >= 0.60:
			origin = edges.OriginSuggested
		default:
			// Below threshold — discard.
			continue
		}

		// For edge candidates we use the stored EdgeType; composites use "contains"
		// as the default relationship when stored as edges.
		relType := r.meta.edgeType
		if relType == "" {
			relType = edges.RelContains
		}

		// Build match signals map for the edge record.
		matchSignals := map[string]any{
			"signals": r.meta.signals,
		}

		req := edges.CreateEdgeRequest{
			SourceAssetID:    r.meta.sourceAssetID,
			TargetAssetID:    r.meta.targetAssetID,
			RelationshipType: relType,
			Direction:        edges.DirDownstream,
			Criticality:      edges.CritMedium,
			Origin:           origin,
			Confidence:       conf,
			MatchSignals:     matchSignals,
		}

		if _, createErr := e.store.CreateEdge(req); createErr != nil {
			return created, suggested, fmt.Errorf("discovery engine: create edge %s→%s: %w",
				r.meta.sourceAssetID, r.meta.targetAssetID, createErr)
		}

		if origin == edges.OriginAuto {
			created++
		} else {
			suggested++
		}
	}

	return created, suggested, nil
}
