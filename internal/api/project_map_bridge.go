package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) scanProjectMapViaBridge(ctx context.Context, location string, maxFiles int) (omni.CodebaseMap, string, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return omni.CodebaseMap{}, "", errHostBridgeUnavailable
	}

	walk, err := client.ScanProjectTree(ctx, location, maxFiles)
	if err != nil {
		return omni.CodebaseMap{}, "", err
	}

	mapPath := omni.DefaultCodebaseMapPath(walk.Root)
	previous, _ := omni.ReadCodebaseMap(mapPath)

	index := workspaceIndexFromWalk(walk)
	cm := omni.BuildCodebaseMapFromIndex(index, previous)

	indexJSON, err := json.Marshal(index)
	if err != nil {
		return omni.CodebaseMap{}, "", err
	}
	mapJSON, err := json.Marshal(cm)
	if err != nil {
		return omni.CodebaseMap{}, "", err
	}

	_, writtenMapPath, err := client.PersistProjectMap(ctx, location, indexJSON, mapJSON)
	if err != nil {
		return omni.CodebaseMap{}, "", err
	}
	if writtenMapPath != "" {
		mapPath = writtenMapPath
	}
	return cm, mapPath, nil
}

func (s *Server) loadProjectMapViaBridge(ctx context.Context, location string) (omni.CodebaseMap, string, bool, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return omni.CodebaseMap{}, "", false, errHostBridgeUnavailable
	}
	raw, mapPath, exists, err := client.ReadProjectMap(ctx, location)
	if err != nil {
		return omni.CodebaseMap{}, "", false, err
	}
	if !exists || len(raw) == 0 {
		return omni.CodebaseMap{}, mapPath, false, nil
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return omni.CodebaseMap{}, "", false, err
	}
	var cm omni.CodebaseMap
	if err := json.Unmarshal(blob, &cm); err != nil {
		return omni.CodebaseMap{}, "", false, err
	}
	return cm, mapPath, true, nil
}

func workspaceIndexFromWalk(walk hostbridge.ProjectWalkResult) omni.WorkspaceIndex {
	index := omni.WorkspaceIndex{
		Version:     "1.0",
		Workspace:   walk.Root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Manifests:   walk.Manifests,
		Files:       make([]omni.WorkspaceIndexFile, 0, len(walk.Files)),
	}
	if index.Manifests == nil {
		index.Manifests = map[string]string{}
	}
	for _, file := range walk.Files {
		index.Files = append(index.Files, omni.WorkspaceIndexFile{
			Path:    file.Path,
			Size:    file.Size,
			ModTime: file.ModTime,
			SHA256:  file.SHA256,
		})
	}
	return index
}

var errHostBridgeUnavailable = &hostBridgeUnavailableError{}

type hostBridgeUnavailableError struct{}

func (e *hostBridgeUnavailableError) Error() string {
	return "host bridge unavailable"
}
