package api

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) loadProjectGitStatusViaBridge(ctx context.Context, location string) (map[string]any, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return nil, fmt.Errorf("project directory is not accessible locally")
	}
	resolved, err := resolveHostBridgeProjectPath(ctx, client, location)
	if err != nil {
		return nil, err
	}
	payload, err := client.ProjectGitStatus(ctx, resolved)
	if err != nil {
		return nil, projectGitBridgeError(err)
	}
	if payload == nil {
		return nil, fmt.Errorf("host bridge returned empty git status")
	}
	if strings.TrimSpace(fmt.Sprint(payload["location"])) == "" {
		payload["location"] = resolved
	}
	if strings.TrimSpace(fmt.Sprint(payload["requested_location"])) == "" && resolved != location {
		payload["requested_location"] = location
	}
	return payload, nil
}

func projectGitBridgeError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "host bridge HTTP 404") {
		return fmt.Errorf("host bridge does not expose project git status yet; restart or update omni-host-bridge")
	}
	return err
}
