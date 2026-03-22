package freebsd

import "github.com/labtether/labtether/internal/agentplatform/generic"

func New(assetID, source string) *generic.Provider {
	return generic.New(assetID, source, "rc-helper")
}
