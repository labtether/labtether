package modelmap

import (
	"context"

	"github.com/labtether/labtether/internal/connectorsdk"
)

type connectorShim struct {
	inner connectorsdk.Connector
}

func WrapConnector(connector connectorsdk.Connector) connectorsdk.Connector {
	if connector == nil {
		return nil
	}
	if wrapped, ok := connector.(*connectorShim); ok {
		return wrapped
	}
	return &connectorShim{inner: connector}
}

func (s *connectorShim) ID() string {
	return s.inner.ID()
}

func (s *connectorShim) DisplayName() string {
	return s.inner.DisplayName()
}

func (s *connectorShim) Capabilities() connectorsdk.Capabilities {
	return s.inner.Capabilities()
}

func (s *connectorShim) Discover(ctx context.Context) ([]connectorsdk.Asset, error) {
	assets, err := s.inner.Discover(ctx)
	if err != nil {
		return nil, err
	}
	return CanonicalizeConnectorAssets(s.inner.ID(), assets), nil
}

func (s *connectorShim) TestConnection(ctx context.Context) (connectorsdk.Health, error) {
	return s.inner.TestConnection(ctx)
}

func (s *connectorShim) Actions() []connectorsdk.ActionDescriptor {
	return CanonicalizeActionDescriptors(s.inner.Actions())
}

func (s *connectorShim) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	resolved := ResolveActionID(actionID, req.TargetID, s.inner.Actions())
	return s.inner.ExecuteAction(ctx, resolved, req)
}
