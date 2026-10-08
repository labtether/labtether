package main

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// version is set at build time via ldflags: -X main.version=...
var version string

const (
	maxActorIDLength          = 64
	maxTargetLength           = 255
	maxCommandLength          = 4096
	maxModeLength             = 32
	maxConnectorIDLength      = 64
	maxActionIDLength         = 64
	maxActionParamCount       = 24
	maxActionParamKeyLength   = 64
	maxActionParamValLength   = 512
	maxPlanNameLength         = 120
	maxPlanTargetCount        = 100
	maxPlanScopeCount         = 24
	maxCredentialNameLength   = 120
	maxCredentialKindLength   = 32
	maxCredentialSecretLen    = 16384
	maxHostKeyLength          = 2048
	maxAlertRuleNameLength    = 120
	maxAlertDescriptionLen    = 2048
	maxAlertTargetCount       = 200
	maxIncidentTitleLength    = 160
	maxIncidentSummaryLen     = 4096
	maxIncidentLinkIDLength   = 255
	maxAssetTagCount          = 32
	maxAssetTagLength         = 64
	streamTicketTTL           = 60 * time.Second
	maxBrowserEventsReadBytes = 64 * 1024
	maxTerminalInputReadBytes = 256 * 1024
	maxDesktopInputReadBytes  = 256 * 1024
)

var terminalWebSocketUpgrader = websocket.Upgrader{
	CheckOrigin:  checkSameOrigin,
	Subprotocols: []string{"binary"},
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runHub(ctx); err != nil {
		logStartupFailure(err)
		os.Exit(1)
	}
}

// handlePolicyCheck evaluates a policy decision request.
func (s *apiServer) handlePolicyCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req policy.CheckRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid policy request payload")
		return
	}

	cfg := s.policyState.Current()
	res := policy.Evaluate(req, cfg)
	servicehttp.WriteJSON(w, http.StatusOK, res)
}
