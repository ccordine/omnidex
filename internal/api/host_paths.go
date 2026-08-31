package api

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/queue"
)

func resolveHostBridgeProjectPath(ctx context.Context, client *hostbridge.Client, raw string) (string, error) {
	location, err := queue.NormalizeProjectLocation(raw)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", fmt.Errorf("host bridge unavailable")
	}
	result, err := client.Browse(ctx, location, hostbridge.BrowseOptions{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("project directory %q is not reachable on the host: %w", location, err)
	}
	if result == nil || strings.TrimSpace(result.Path) == "" {
		return "", fmt.Errorf("project directory %q returned no authoritative host path", location)
	}
	resolved := filepath.Clean(result.Path)
	if resolved != location {
		return "", fmt.Errorf("host bridge returned project root %q for authoritative root %q", resolved, location)
	}
	return resolved, nil
}
