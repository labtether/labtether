package promexport

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHandler returns an http.Handler that serves the Prometheus metrics
// exposition format for the given SnapshotSource.
//
// The handler uses a dedicated prometheus.Registry (not the default global
// one) so LabTether metrics are fully isolated from any library-level metrics
// that might otherwise leak into the scrape output.
func NewHandler(source SnapshotSource) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewCollector(source))
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
