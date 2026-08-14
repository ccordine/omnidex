package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

const defaultProjectMapMaxFiles = 1200

func (s *Server) handleProjectMap(w http.ResponseWriter, r *http.Request, id int64, action string) {
	switch action {
	case "map":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	case "map/scan":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := decodeProjectAutoWorkActionRequest(w, r, "project map scan"); err != nil {
			writeError(w, projectRequestErrorStatus(err), err.Error())
			return
		}
	default:
		writeError(w, http.StatusNotFound, "project map action not found")
		return
	}
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	location := strings.TrimSpace(project.Location)
	if location == "" {
		writeError(w, http.StatusBadRequest, "project location is not set")
		return
	}
	switch action {
	case "map":
		payload, err := s.loadProjectCodebaseMapPayload(r.Context(), location)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case "map/scan":
		payload, err := s.scanProjectCodebaseMap(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func (s *Server) loadProjectCodebaseMapPayload(ctx context.Context, location string) (map[string]any, error) {
	if client := s.hostBridgeClient(); client != nil && !projectPathAccessibleLocally(location) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cm, err := s.scanProjectMapViaBridge(ctx, location, defaultProjectMapMaxFiles)
		if err != nil {
			return nil, err
		}
		return codebaseMapPayload(cm, true), nil
	}
	return loadProjectCodebaseMapPayloadLocal(location)
}

func loadProjectCodebaseMapPayloadLocal(location string) (map[string]any, error) {
	cm, err := omni.BuildCodebaseMap(location, omni.CodebaseMapConfig{MaxFiles: defaultProjectMapMaxFiles})
	if err != nil {
		return nil, err
	}
	return codebaseMapPayload(cm, true), nil
}

type projectMapScanResponse struct {
	ProjectID     int64  `json:"project_id"`
	GeneratedAt   string `json:"generated_at"`
	Source        string `json:"source"`
	FileCount     int    `json:"file_count"`
	ModuleCount   int    `json:"module_count"`
	ScanTruncated bool   `json:"scan_truncated"`
}

func (s *Server) scanProjectCodebaseMap(ctx context.Context, project model.Project) (projectMapScanResponse, error) {
	location := strings.TrimSpace(project.Location)
	if location == "" {
		return projectMapScanResponse{}, errProjectLocationMissing
	}

	if client := s.hostBridgeClient(); client != nil && !projectPathAccessibleLocally(location) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		cm, err := s.scanProjectMapViaBridge(ctx, location, defaultProjectMapMaxFiles)
		if err != nil {
			return projectMapScanResponse{}, err
		}
		return newProjectMapScanResponse(project.ID, "host-bridge", cm)
	}

	cm, err := omni.BuildCodebaseMap(location, omni.CodebaseMapConfig{MaxFiles: defaultProjectMapMaxFiles})
	if err != nil {
		return projectMapScanResponse{}, err
	}
	return newProjectMapScanResponse(project.ID, "core-local", cm)
}

func newProjectMapScanResponse(projectID int64, source string, cm omni.CodebaseMap) (projectMapScanResponse, error) {
	if projectID <= 0 || (source != "host-bridge" && source != "core-local") || strings.TrimSpace(cm.GeneratedAt) == "" {
		return projectMapScanResponse{}, fmt.Errorf("codebase scan did not produce exact response authority")
	}
	return projectMapScanResponse{
		ProjectID: projectID, GeneratedAt: cm.GeneratedAt, Source: source,
		FileCount: len(cm.Files), ModuleCount: len(cm.Modules), ScanTruncated: cm.Truncated,
	}, nil
}

func projectPathAccessibleLocally(location string) bool {
	info, err := os.Stat(strings.TrimSpace(location))
	return err == nil && info.IsDir()
}

var errProjectLocationMissing = &projectLocationError{}

type projectLocationError struct{}

func (e *projectLocationError) Error() string { return "project location is not set" }

func codebaseMapPayload(cm omni.CodebaseMap, exists bool) map[string]any {
	languages := make([]map[string]any, 0, len(cm.Languages))
	for _, lang := range cm.Languages {
		languages = append(languages, map[string]any{
			"language": lang.Language,
			"files":    lang.Files,
			"bytes":    lang.Bytes,
		})
	}
	sort.Slice(languages, func(i, j int) bool {
		li, _ := languages[i]["language"].(string)
		lj, _ := languages[j]["language"].(string)
		return li < lj
	})

	modules := make([]map[string]any, 0, minInt(16, len(cm.Modules)))
	moduleItems := append([]omni.ModuleSummary(nil), cm.Modules...)
	sort.Slice(moduleItems, func(i, j int) bool { return moduleItems[i].Path < moduleItems[j].Path })
	for i, mod := range moduleItems {
		if i >= 16 {
			break
		}
		files := append([]string(nil), mod.ImportantFiles...)
		sort.Strings(files)
		if len(files) > 8 {
			files = files[:8]
		}
		modules = append(modules, map[string]any{
			"path":             mod.Path,
			"purpose":          mod.Purpose,
			"important_files":  files,
			"confidence":       mod.Confidence,
			"responsibilities": mod.Responsibilities,
		})
	}

	entrypoints := make([]map[string]any, 0, len(cm.Entrypoints))
	for _, entry := range cm.Entrypoints {
		entrypoints = append(entrypoints, map[string]any{
			"path":   entry.Path,
			"kind":   entry.Kind,
			"reason": entry.Reason,
		})
	}

	commands := make([]map[string]any, 0, minInt(12, len(cm.Commands)))
	for i, cmd := range cm.Commands {
		if i >= 12 {
			break
		}
		commands = append(commands, map[string]any{
			"name":    cmd.Name,
			"command": cmd.Command,
			"source":  cmd.Source,
		})
	}

	tests := make([]string, 0, minInt(12, len(cm.Tests)))
	for i, test := range cm.Tests {
		if i >= 12 {
			break
		}
		tests = append(tests, test.Path)
	}

	risks := make([]map[string]any, 0, minInt(8, len(cm.Risks)))
	for i, risk := range cm.Risks {
		if i >= 8 {
			break
		}
		risks = append(risks, map[string]any{
			"area":   risk.Area,
			"risk":   risk.Risk,
			"reason": risk.Reason,
		})
	}

	manifests := make([]string, 0, len(cm.Manifests))
	for _, manifest := range cm.Manifests {
		manifests = append(manifests, manifest.Path)
	}
	sort.Strings(manifests)

	return map[string]any{
		"exists":         exists,
		"generated_at":   cm.GeneratedAt,
		"workspace_id":   cm.WorkspaceID,
		"root":           cm.Root,
		"file_count":     len(cm.Files),
		"module_count":   len(cm.Modules),
		"scan_truncated": cm.Truncated,
		"languages":      languages,
		"modules":        modules,
		"entrypoints":    entrypoints,
		"commands":       commands,
		"tests":          tests,
		"risks":          risks,
		"manifests":      manifests,
		"open_questions": cm.OpenQuestions,
		"files_preview":  codebaseMapFilesPreview(cm.Files, 48),
		"tree_preview":   codebaseMapTreePreview(cm.Files, 48),
	}
}

func codebaseMapFilesPreview(files []omni.FileSummary, limit int) []map[string]any {
	if limit <= 0 {
		limit = 40
	}
	items := append([]omni.FileSummary(nil), files...)
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	out := make([]map[string]any, 0, minInt(limit, len(items)))
	for i, file := range items {
		if i >= limit {
			break
		}
		out = append(out, map[string]any{
			"path":     file.Path,
			"language": file.Language,
			"module":   file.Module,
			"purpose":  file.Purpose,
		})
	}
	return out
}

func codebaseMapTreePreview(files []omni.FileSummary, limit int) string {
	if limit <= 0 {
		limit = 40
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if path := strings.TrimSpace(file.Path); path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if len(paths) > limit {
		paths = paths[:limit]
	}
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, "\n")
}
