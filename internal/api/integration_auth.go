package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

func (s *Server) requireIntegrationAuthentication(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if s == nil || s.integrationAPIToken == "" {
			writeError(w, http.StatusServiceUnavailable, "integration API is not configured")
			return
		}
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 || !exactIntegrationBearerToken(headers[0], s.integrationAPIToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="omnidex-integration"`)
			writeError(w, http.StatusUnauthorized, "integration API authentication failed")
			return
		}
		next(w, r)
	}
}

func exactIntegrationBearerToken(header, configured string) bool {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return false
	}
	providedDigest := sha256.Sum256([]byte(header[len(prefix):]))
	configuredDigest := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(providedDigest[:], configuredDigest[:]) == 1
}
