package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func validateInsecureVNCTransport(allowInsecure bool) error {
	if !allowInsecure {
		return errors.New("plain VNC requires allow_insecure_vnc=true")
	}
	if !securityruntime.InsecureTransportAllowed() {
		return errors.New("plain VNC requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
	}
	return nil
}

func requireInsecureVNCTransport(w http.ResponseWriter, allowInsecure bool) bool {
	if err := validateInsecureVNCTransport(allowInsecure); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func protocolConfigAllowsInsecureVNC(protocol string, raw json.RawMessage) (bool, error) {
	switch protocol {
	case protocols.ProtocolVNC:
		cfg, err := protocols.DecodeVNCConfig(raw)
		return cfg.AllowInsecureTransport, err
	case protocols.ProtocolARD:
		cfg, err := protocols.DecodeARDConfig(raw)
		return cfg.AllowInsecureTransport, err
	default:
		return false, fmt.Errorf("protocol %q does not use VNC transport", protocol)
	}
}
