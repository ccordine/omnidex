package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

const defaultProjectMapMaxFiles = 1200

func (s *Server) handleProjectMap(w http.ResponseWriter, r *http.Request, id int64, action string) {
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
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, err := loadProjectCodebaseMapPayload(location)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case "map/scan":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, err := scanProjectCodebaseMap(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		writeError(w, http.StatusNotFound, "project map action not found")
	}
}

func loadProjectCodebaseMapPayload(location string) (map[string]any, error) {
	mapPath := omni.DefaultCodebaseMapPath(location)
	if _, err := os.Stat(mapPath); err != nil {
		if os.IsNotExist(err) {
			return codebaseMapPayload(omni.CodebaseMap{}, mapPath, false), nil
		}
		return nil, err
	}
	cm, err := omni.ReadCodebaseMap(mapPath)
	if err != nil {
		return nil, err
	}
	return codebaseMapPayload(cm, mapPath, true), nil
}

func scanProjectCodebaseMap(ctx context.Context, project model.Project) (map[string]any, error) {
	_ = ctx
	location := strings.TrimSpace(project.Location)
	if location == "" {
		return nil, errProjectLocationMissing
	}
	mapPath := omni.DefaultCodebaseMapPath(location)
	cm, err := omni.UpdateCodebaseMap(location, mapPath, omni.CodebaseMapConfig{MaxFiles: defaultProjectMapMaxFiles})
	if err != nil {
		return nil, err
	}
	if err := omni.WriteCodebaseMap(cm, mapPath); err != nil {
		return nil, err
	}
	payload := codebaseMapPayload(cm, mapPath, true)
	payload["message"] = "codebase map updated"
	return payload, nil
}

var errProjectLocationMissing = &projectLocationError{}

type projectLocationError struct{}

func (e *projectLocationError) Error() string { return "project location is not set" }

func codebaseMapPayload(cm omni.CodebaseMap, mapPath string, exists bool) map[string]any {
	staleCount := 0
	for _, file := range cm.Files {
		if file.Stale {
			staleCount++
		}
	}
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
	sort.Slice(moduleItems, func(i, j int) bool {
		return moduleItems[i].Path < moduleItems[j].Path
	})
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
			"stale":            mod.Stale,
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
		"exists":           exists,
		"map_path":         mapPath,
		"relative_map_path": relativeProjectPath(cm.Root, mapPath),
		"generated_at":     cm.GeneratedAt,
		"revision":         cm.Revision,
		"workspace_id":     cm.WorkspaceID,
		"root":             cm.Root,
		"file_count":       len(cm.Files),
		"module_count":     len(cm.Modules),
		"stale_file_count": staleCount,
		"languages":        languages,
		"modules":          modules,
		"entrypoints":      entrypoints,
		"commands":         commands,
		"tests":            tests,
		"risks":            risks,
		"manifests":        manifests,
		"open_questions":   cm.OpenQuestions,
		"files_preview":    codebaseMapFilesPreview(cm.Files, 48),
		"tree_preview":     codebaseMapTreePreview(cm.Files, 48),
	}
}

func relativeProjectPath(root, target string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	target = filepath.Clean(strings.TrimSpace(target))
	if root == "" || target == "" {
		return target
	}
	if rel, err := filepath.Rel(root, target); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(target)
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
			"stale":    file.Stale,
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
