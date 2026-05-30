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
	payload, err := client.ProjectGitStatus(ctx, location)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("host bridge returned empty git status")
	}
	if strings.TrimSpace(fmt.Sprint(payload["location"])) == "" {
		payload["location"] = location
	}
	return payload, nil
}
