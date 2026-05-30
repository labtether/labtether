package main

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func requireScope(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if required != "" && !apiv2.ScopeCheck(scopesFromContext(r.Context()), required) {
			apiv2.WriteScopeForbidden(w, required)
			return
		}
		next(w, r)
	}
}

func requireReadWriteScopes(readScope, writeScope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		required := readScope
		if apiv2.IsMutatingMethod(r.Method) {
			required = writeScope
		}
		if required != "" && !apiv2.ScopeCheck(scopesFromContext(r.Context()), required) {
			apiv2.WriteScopeForbidden(w, required)
			return
		}
		next(w, r)
	}
}

func requireScopeByMethod(methodScopes map[string]string, fallbackScope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		required := methodScopes[r.Method]
		if required == "" {
			required = fallbackScope
		}
		if required != "" && !apiv2.ScopeCheck(scopesFromContext(r.Context()), required) {
			apiv2.WriteScopeForbidden(w, required)
			return
		}
		next(w, r)
	}
}

func requireAssetFromPath(prefix string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		assetID := routeAssetID(prefix, r.URL.Path)
		if assetID != "" && !apiv2.AssetCheck(allowedAssetsFromContext(r.Context()), assetID) {
			apiv2.WriteAssetForbidden(w, assetID)
			return
		}
		next(w, r)
	}
}

func routeAssetID(prefix, path string) string {
	if prefix == "" || !strings.HasPrefix(path, prefix) {
		return ""
	}
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if trimmed == "" {
		return ""
	}
	part := strings.SplitN(trimmed, "/", 2)[0]
	id, err := url.PathUnescape(part)
	if err != nil {
		return strings.TrimSpace(part)
	}
	return strings.TrimSpace(id)
}

func guardV1AssetRoute(prefix, readScope, writeScope string, next http.HandlerFunc) http.HandlerFunc {
	return requireReadWriteScopes(readScope, writeScope, requireAssetFromPath(prefix, next))
}
