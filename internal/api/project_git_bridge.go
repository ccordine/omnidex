package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/projectgit"
)

func (s *Server) loadProjectGitStatusViaBridge(ctx context.Context, location string) (projectgit.Status, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return projectgit.Status{}, fmt.Errorf("project directory is not accessible locally")
	}
	resolved, err := resolveHostBridgeProjectPath(ctx, client, location)
	if err != nil {
		return projectgit.Status{}, err
	}
	payload, err := client.ProjectGitStatus(ctx, resolved)
	if err != nil {
		return projectgit.Status{}, projectGitBridgeError(err)
	}
	if payload.Location != resolved || payload.Source != "host-bridge" {
		return projectgit.Status{}, fmt.Errorf("host bridge git status does not attest the resolved project path")
	}
	if payload.RequestedLocation != "" {
		return projectgit.Status{}, fmt.Errorf("host bridge git status requested path attestation is inconsistent")
	}
	if resolved != location {
		payload.RequestedLocation = location
	}
	if err := payload.Validate(); err != nil {
		return projectgit.Status{}, fmt.Errorf("validate mapped host bridge git status: %w", err)
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
