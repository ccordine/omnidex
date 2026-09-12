package websearch

import (
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
