package persistence

import (
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/incidents"
)

type dependencyScanner interface {
	Scan(dest ...any) error
}

func scanDependency(row dependencyScanner) (dependencies.Dependency, error) {
	dep := dependencies.Dependency{}
	var metadata []byte
	if err := row.Scan(
		&dep.ID,
		&dep.SourceAssetID,
		&dep.TargetAssetID,
		&dep.RelationshipType,
		&dep.Direction,
		&dep.Criticality,
		&metadata,
		&dep.CreatedAt,
		&dep.UpdatedAt,
	); err != nil {
		return dependencies.Dependency{}, err
	}
	dep.Metadata = unmarshalStringMap(metadata)
	dep.CreatedAt = dep.CreatedAt.UTC()
	dep.UpdatedAt = dep.UpdatedAt.UTC()
	return dep, nil
}

type incidentAssetScanner interface {
	Scan(dest ...any) error
}

func scanIncidentAsset(row incidentAssetScanner) (incidents.IncidentAsset, error) {
	ia := incidents.IncidentAsset{}
	if err := row.Scan(
		&ia.ID,
		&ia.IncidentID,
		&ia.AssetID,
		&ia.Role,
		&ia.CreatedAt,
	); err != nil {
		return incidents.IncidentAsset{}, err
	}
	ia.CreatedAt = ia.CreatedAt.UTC()
	return ia, nil
}
