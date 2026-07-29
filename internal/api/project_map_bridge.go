package api

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) scanProjectMapViaBridge(ctx context.Context, location string, maxFiles int) (omni.CodebaseMap, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return omni.CodebaseMap{}, errHostBridgeUnavailable
	}
	walk, err := client.ScanProjectTree(ctx, location, maxFiles)
	if err != nil {
		return omni.CodebaseMap{}, err
	}
	return omni.BuildCodebaseMapFromIndex(workspaceIndexFromWalk(walk)), nil
}

func workspaceIndexFromWalk(walk hostbridge.ProjectWalkResult) omni.WorkspaceIndex {
	index := omni.WorkspaceIndex{
		Workspace:   walk.Root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Manifests:   append([]string(nil), walk.Manifests...),
		Truncated:   walk.Truncated,
		Files:       make([]omni.WorkspaceIndexFile, 0, len(walk.Files)),
	}
	for _, file := range walk.Files {
		index.Files = append(index.Files, omni.WorkspaceIndexFile{Path: file.Path, Size: file.Size})
	}
	return index
}

var errHostBridgeUnavailable = &hostBridgeUnavailableError{}

type hostBridgeUnavailableError struct{}

func (e *hostBridgeUnavailableError) Error() string { return "host bridge unavailable" }
