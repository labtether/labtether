package remoteaccess

import (
	"time"

	"github.com/labtether/labtether/internal/agentcore/sysconfig"
)

const (
	DefaultCommandTimeout   = 30 * time.Second
	MaxRemoteCommandTimeout = 5 * time.Minute
	MaxCommandOutputBytes   = sysconfig.MaxCommandOutputBytes
)

// TruncateCommandOutput delegates to sysconfig.TruncateCommandOutput.
var TruncateCommandOutput = sysconfig.TruncateCommandOutput
