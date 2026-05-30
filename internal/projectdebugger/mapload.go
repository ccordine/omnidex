package projectdebugger

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

func LoadMapPayload(location string) map[string]any {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil
	}
	mapPath := omni.DefaultCodebaseMapPath(location)
	if _, err := os.Stat(mapPath); err != nil {
		if os.IsNotExist(err) {
			return map[string]any{"exists": false}
		}
		return nil
	}
	cm, err := omni.ReadCodebaseMap(mapPath)
	if err != nil {
		return map[string]any{"exists": false}
	}
	return mapPayloadFromCodebaseMap(cm, true)
}

func mapPayloadFromCodebaseMap(cm omni.CodebaseMap, exists bool) map[string]any {
	modules := make([]map[string]any, 0, minInt(8, len(cm.Modules)))
	for i, mod := range cm.Modules {
		if i >= 8 {
			break
		}
		modules = append(modules, map[string]any{
			"path":    mod.Path,
			"purpose": mod.Purpose,
		})
	}
	risks := make([]map[string]any, 0, minInt(6, len(cm.Risks)))
	for i, risk := range cm.Risks {
		if i >= 6 {
			break
		}
		risks = append(risks, map[string]any{
			"area": risk.Area,
			"risk": risk.Risk,
		})
	}
	tests := make([]string, 0, len(cm.Tests))
	for _, test := range cm.Tests {
		tests = append(tests, test.Path)
	}
	treePaths := make([]string, 0, minInt(48, len(cm.Files)))
	for i, file := range cm.Files {
		if i >= 48 {
			break
		}
		if path := strings.TrimSpace(file.Path); path != "" {
			treePaths = append(treePaths, path)
		}
	}
	return map[string]any{
		"exists":          exists,
		"root":            cm.Root,
		"file_count":      len(cm.Files),
		"modules":         toAnySlice(modules),
		"risks":           toAnySlice(risks),
		"tests":           toAnySliceStrings(tests),
		"open_questions":  toAnySliceStrings(cm.OpenQuestions),
		"tree_preview":    strings.Join(treePaths, "\n"),
	}
}

func toAnySlice(items []map[string]any) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func toAnySliceStrings(items []string) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
