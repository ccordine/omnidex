package api

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const defaultHostBridgePort = "8091"

func terminalUseDirectBridge(r *http.Request, coreURL string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OMNI_TERMINAL_VIA_CORE")), "true") {
		return false
	}
	if strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_WS_URL")) != "" ||
		strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_URL")) != "" {
		return browserBridgeWSBase(r, coreURL) != ""
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OMNI_TERMINAL_DIRECT")), "true") &&
		browserBridgeWSBase(r, coreURL) != ""
}

// browserBridgeWSBase returns a ws:// URL the browser can reach for direct bridge access.
// Prefer the host the user already used to load the UI (r.Host), not CORE_URL inside Docker.
func browserBridgeWSBase(r *http.Request, coreURL string) string {
	if explicit := strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_WS_URL")); explicit != "" {
		return normalizeWSBase(explicit)
	}
	if explicit := strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_URL")); explicit != "" {
		return httpBaseToWS(explicit)
	}
	host := requestHost(r)
	if host == "" {
		return publicBridgeWSBase(coreURL)
	}
	if host == "host.docker.internal" || host == "localhost" || host == "127.0.0.1" {
		return ""
	}
	port := strings.TrimSpace(os.Getenv("HOST_AGENT_PORT"))
	if port == "" {
		port = defaultHostBridgePort
	}
	return "ws://" + net.JoinHostPort(host, port)
}

func requestHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func publicBridgeWSBase(coreURL string) string {
	if explicit := strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_WS_URL")); explicit != "" {
		return normalizeWSBase(explicit)
	}
	if explicit := strings.TrimSpace(os.Getenv("HOST_BRIDGE_PUBLIC_URL")); explicit != "" {
		return httpBaseToWS(explicit)
	}
	parsed, err := url.Parse(strings.TrimSpace(coreURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := parsed.Hostname()
	if host == "host.docker.internal" || host == "localhost" || host == "127.0.0.1" {
		return ""
	}
	port := strings.TrimSpace(os.Getenv("HOST_AGENT_PORT"))
	if port == "" {
		port = defaultHostBridgePort
	}
	return "ws://" + net.JoinHostPort(host, port)
}

func normalizeWSBase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func httpBaseToWS(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return normalizeWSBase(raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func coreWSBase(r *http.Request, coreURL string) string {
	if forced := strings.TrimSpace(os.Getenv("OMNI_TERMINAL_WS_URL")); forced != "" {
		return strings.TrimRight(forced, "/")
	}
	if forced := strings.TrimSpace(os.Getenv("OMNI_TERMINAL_WS_SCHEME")); forced != "" {
		host := strings.TrimSpace(r.Host)
		if host == "" {
			if parsed, err := url.Parse(coreURL); err == nil {
				host = parsed.Host
			}
		}
		if host != "" {
			return forced + "://" + host
		}
	}
	if r.TLS != nil {
		return "wss://" + r.Host
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return "wss://" + r.Host
	}
	return "ws://" + r.Host
}

func buildDirectTerminalWSURL(base, cwd string, query url.Values, token string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	parsed.Path = "/v1/terminal/ws"
	params := url.Values{}
	params.Set("cwd", cwd)
	if cols := strings.TrimSpace(query.Get("cols")); cols != "" {
		params.Set("cols", cols)
	}
	if rows := strings.TrimSpace(query.Get("rows")); rows != "" {
		params.Set("rows", rows)
	}
	if token != "" {
		params.Set("token", token)
	}
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func buildProxyTerminalWSURL(base string, query url.Values) string {
	return strings.TrimRight(base, "/") + "/v1/host/terminal/ws?" + query.Encode()
}
