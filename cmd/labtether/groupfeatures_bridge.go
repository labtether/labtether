package main

import (
	groupfeaturespkg "github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

// buildGroupFeaturesDeps constructs a groupfeatures.Deps from the apiServer's fields.
func (s *apiServer) buildGroupFeaturesDeps() *groupfeaturespkg.Deps {
	return &groupfeaturespkg.Deps{
		GroupStore:            s.groupStore,
		AssetStore:            s.assetStore,
		LogStore:              s.logStore,
		ActionStore:           s.actionStore,
		UpdateStore:           s.updateStore,
		GroupMaintenanceStore: s.groupMaintenanceStore,
		GroupProfileStore:     s.groupProfileStore,
	}
}

// ensureGroupFeaturesDeps returns the cached group features deps, creating on first call.
func (s *apiServer) ensureGroupFeaturesDeps() *groupfeaturespkg.Deps {
	if s.groupFeaturesDeps != nil {
		return s.groupFeaturesDeps
	}
	d := s.buildGroupFeaturesDeps()
	s.groupFeaturesDeps = d
	return d
}
