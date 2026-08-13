package websearch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
)

var exactTrackingParameters = map[string]struct{}{
	"dclid": {}, "fbclid": {}, "gclid": {}, "mc_cid": {}, "mc_eid": {}, "msclkid": {},
}

func CanonicalizeURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse candidate URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("candidate URL requires http or https scheme")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("candidate URL must not contain user information")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("candidate URL must not contain a fragment")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("candidate URL host is empty")
	}
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	parsed.Host = hostname
	if strings.Contains(hostname, ":") {
		parsed.Host = "[" + hostname + "]"
	}
	if port != "" {
		parsed.Host = net.JoinHostPort(hostname, port)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	} else {
		trailingSlash := strings.HasSuffix(parsed.Path, "/")
		parsed.Path = path.Clean(parsed.Path)
		if trailingSlash && parsed.Path != "/" {
			parsed.Path += "/"
		}
	}
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") {
			query.Del(key)
			continue
		}
		if _, tracking := exactTrackingParameters[lower]; tracking {
			query.Del(key)
		}
	}
	parsed.RawQuery = stableQuery(query)
	canonical := parsed.String()
	if _, err := parseOutboundURL(canonical); err != nil {
		return "", err
	}
	return canonical, nil
}

func stableQuery(values url.Values) string {
	for key := range values {
		sort.Strings(values[key])
	}
	return values.Encode()
}

func candidateID(canonicalURL string) CandidateID {
	digest := sha256.Sum256([]byte("web-candidate.v1\x00" + canonicalURL))
	return CandidateID("candidate_" + hex.EncodeToString(digest[:]))
}

func documentID(canonicalURL, contentSHA string) DocumentID {
	digest := sha256.Sum256([]byte("web-document.v1\x00" + canonicalURL + "\x00" + contentSHA))
	return DocumentID("document_" + hex.EncodeToString(digest[:]))
}

// CandidateIDForURL returns the stable opaque identity for an already
// canonical URL. Non-canonical input is rejected instead of being silently
// rewritten at an authority boundary.
func CandidateIDForURL(canonicalURL string) (CandidateID, error) {
	normalized, err := CanonicalizeURL(canonicalURL)
	if err != nil {
		return "", err
	}
	if normalized != canonicalURL {
		return "", fmt.Errorf("candidate URL is not canonical")
	}
	return candidateID(canonicalURL), nil
}
