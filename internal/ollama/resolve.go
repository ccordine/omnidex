package ollama

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/dockerhost"
)

const defaultResolveProbeTimeout = 4 * time.Second

// CandidateBaseURLs returns deduplicated Ollama base URLs to try, primary first.
func CandidateBaseURLs(primary string) []string {
	return dockerhost.CandidateHostURLs(primary, "OLLAMA_FALLBACK_URLS", "11434")
}

// NormalizeBaseURL trims, adds a scheme, and strips trailing dots from the host.
func NormalizeBaseURL(raw string) string {
	return dockerhost.NormalizeBaseURL(raw)
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

// LinuxDockerFirewallHint returns remediation when probes time out from inside Docker.
func LinuxDockerFirewallHint(err error) string {
	return dockerhost.FirewallHint(err, []int{11434, 8091})
}
