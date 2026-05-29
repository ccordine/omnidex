package hostbridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/dockerhost"
)

const defaultHostBridgeResolveTimeout = 4 * time.Second

// CandidateURLs returns host bridge base URLs to try, primary first.
func CandidateURLs(primary string) []string {
	return dockerhost.CandidateHostURLs(primary, "HOST_AGENT_FALLBACK_URLS", "8091")
}

// ResolveReachableURL probes CandidateURLs and returns the first reachable base URL.
func ResolveReachableURL(ctx context.Context, primary, token string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = defaultHostBridgeResolveTimeout
	}
	candidates := CandidateURLs(primary)
	if len(candidates) == 0 {
		return "", fmt.Errorf("no host bridge URL candidates configured")
	}

	var attempts []string
	var lastErr error
	for _, candidate := range candidates {
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		client := NewClient(candidate, token, timeout)
		_, err := client.Health(probeCtx)
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
	return "", fmt.Errorf("no reachable host bridge after %d attempts: %s", len(candidates), strings.Join(attempts, " | "))
}

// LinuxDockerFirewallHint returns remediation when probes time out from inside Docker.
func LinuxDockerFirewallHint(err error) string {
	return dockerhost.FirewallHint(err, []int{8091, 11434})
}
