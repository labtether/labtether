package admin

import (
	"net/http"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
)

func writeAdminInternalError(w http.ResponseWriter, status int, clientMessage string, err error) {
	securityruntime.Logf("admin API: %s: %v", clientMessage, err)
	servicehttp.WriteError(w, status, clientMessage)
}
