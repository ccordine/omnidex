package ollama

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultResolveProbeTimeout = 4 * time.Second

// CandidateBaseURLs returns deduplicated Ollama base URLs to try, primary first.
func CandidateBaseURLs(primary string) []string {
	port := ollamaPort(primary)
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(raw string) {
		normalized := NormalizeBaseURL(raw)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	add(primary)
	for _, item := range strings.Split(os.Getenv("OLLAMA_FALLBACK_URLS"), ",") {
		add(item)
	}
	add(fmt.Sprintf("http://host.docker.internal:%s", port))
	if gateway := dockerDefaultGatewayIP(); gateway != nil {
		add(fmt.Sprintf("http://%s:%s", gateway.String(), port))
	}
	add(fmt.Sprintf("http://127.0.0.1:%s", port))
	add(fmt.Sprintf("http://localhost:%s", port))
	return out
}

// NormalizeBaseURL trims, adds a scheme, and strips trailing dots from the host.
func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	parsed.Host = normalizeHost(parsed.Host)
	if parsed.Host == "" {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if hostname, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(strings.TrimSuffix(hostname, "."), port)
	}
	return strings.TrimSuffix(host, ".")
}

func ollamaPort(primary string) string {
	normalized := NormalizeBaseURL(primary)
	if normalized == "" {
		return "11434"
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Host == "" {
		return "11434"
	}
	if port := parsed.Port(); port != "" {
		return port
	}
	return "11434"
}

func dockerDefaultGatewayIP() net.IP {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		return parseRouteHexIP(fields[2])
	}
	return nil
}

func parseRouteHexIP(value string) net.IP {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return nil
	}
	raw, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return nil
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(raw))
	return ip
}

// ProbeReachable checks whether Ollama responds at baseURL within timeout.
func ProbeReachable(ctx context.Context, baseURL string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultResolveProbeTimeout
	}
	client := New(baseURL, "", "", timeout)
	_, err := client.ListTags(ctx)
	return err
}

// ResolveReachableBaseURL probes CandidateBaseURLs and returns the first reachable endpoint.
func ResolveReachableBaseURL(ctx context.Context, primary string, probeTimeout time.Duration) (string, error) {
	if probeTimeout <= 0 {
		probeTimeout = defaultResolveProbeTimeout
	}
	candidates := CandidateBaseURLs(primary)
	if len(candidates) == 0 {
		return "", fmt.Errorf("no ollama base URL candidates configured")
	}

	var attempts []string
	var lastErr error
	for _, candidate := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		err := ProbeReachable(probeCtx, candidate, probeTimeout)
		cancel()
		if err == nil {
			return candidate, nil
		}
		lastErr = err
		attempts = append(attempts, fmt.Sprintf("%s: %v", candidate, err))
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidates responded")
	}
	return "", fmt.Errorf("no reachable ollama endpoint after %d attempts: %s", len(candidates), strings.Join(attempts, " | "))
}
