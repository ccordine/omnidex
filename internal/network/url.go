package network

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultHost = "192.168.1.102"
	DefaultPort = 8090
	WorkspaceKey = "core_url"
)

func DefaultCoreURL() string {
	return fmt.Sprintf("http://%s:%d", DefaultHost, DefaultPort)
}

func NormalizeCoreURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultCoreURL()
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return DefaultCoreURL()
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(DefaultPort)
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

func ParseHostPort(coreURL string) (host string, port int) {
	coreURL = NormalizeCoreURL(coreURL)
	parsed, err := url.Parse(coreURL)
	if err != nil {
		return DefaultHost, DefaultPort
	}
	host = parsed.Hostname()
	if host == "" {
		host = DefaultHost
	}
	port = DefaultPort
	if parsed.Port() != "" {
		if value, err := strconv.Atoi(parsed.Port()); err == nil && value > 0 {
			port = value
		}
	}
	return host, port
}

func BuildCoreURL(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = DefaultHost
	}
	if port <= 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, strconv.Itoa(port)))
}
