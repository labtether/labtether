package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

func denyAssetRestrictedGlobalAPI(w http.ResponseWriter, r *http.Request, object string) bool {
	if len(allowedAssetsFromContext(r.Context())) == 0 {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "asset-restricted api keys cannot access global "+object)
	return true
}

func guardAssetRestrictedGlobalAPI(object string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if denyAssetRestrictedGlobalAPI(w, r, object) {
			return
		}
		next(w, r)
	}
}
