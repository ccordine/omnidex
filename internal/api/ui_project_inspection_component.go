package api

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/projectgit"
)

type uiProjectMapInspection struct {
	TreePreview string
	FileCount   int64
	ModuleCount int64
}

func renderUIProjectMap(projectID int64, payload map[string]any) (string, error) {
	view, err := decodeUIProjectMapInspection(payload)
	if err != nil {
		return "", err
	}
	return `<div data-project-tab-panel="map" class="scrollbar space-y-4"><section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><div class="flex items-start justify-between gap-3"><div><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Codebase map</h3><p class="mt-1 text-xs text-zinc-500">Current server-inspected repository context.</p></div><button type="button" data-action="projects#scanProjectMap" data-project-id="` + uiInt(projectID) + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Refresh map</button></div><div class="mt-4 grid gap-3 sm:grid-cols-2"><div class="rounded-md border border-white/10 p-3"><span class="text-xs text-zinc-500">Files</span><div class="font-mono text-lg text-cyan-200">` + fmt.Sprint(view.FileCount) + `</div></div><div class="rounded-md border border-white/10 p-3"><span class="text-xs text-zinc-500">Modules</span><div class="font-mono text-lg text-cyan-200">` + fmt.Sprint(view.ModuleCount) + `</div></div></div><pre class="scrollbar mt-4 max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-white/10 bg-black/40 p-3 font-mono text-[11px]">` + uiEscape(view.TreePreview) + `</pre></section></div>`, nil
}

func renderUIProjectGit(projectID int64, status projectgit.Status) (string, error) {
	if err := status.Validate(); err != nil {
		return "", fmt.Errorf("render project git inspection: %w", err)
	}
	if !status.IsRepo {
		return `<div data-project-tab-panel="git"><p class="rounded-md border border-amber-300/20 p-4 text-sm text-amber-200">This project is not a Git repository.</p></div>`, nil
	}
	return `<div data-project-tab-panel="git" class="scrollbar space-y-4"><section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><div class="flex items-center justify-between gap-3"><div><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Git</h3><p class="mt-1 font-mono text-sm text-cyan-200">` + uiEscape(status.Branch) + ` · ` + uiEscape(status.HeadShort) + `</p></div><button type="button" data-action="projects#refreshProjectGit" data-project-id="` + uiInt(projectID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Refresh</button></div><div class="mt-4 grid gap-2 sm:grid-cols-4">` + uiGitMetric("Staged", int64(status.StagedCount)) + uiGitMetric("Modified", int64(status.ModifiedCount)) + uiGitMetric("Untracked", int64(status.UntrackedCount)) + uiGitMetric("Deleted", int64(status.DeletedCount)) + `</div></section></div>`, nil
}

func uiGitMetric(label string, value int64) string {
	return `<div class="rounded-md border border-white/10 p-3"><div class="text-[11px] uppercase text-zinc-500">` + label + `</div><div class="mt-1 font-mono text-lg">` + fmt.Sprint(value) + `</div></div>`
}

func decodeUIProjectMapInspection(payload map[string]any) (uiProjectMapInspection, error) {
	tree, err := uiRequiredInspectionString(payload, "tree_preview", 256*1024, true)
	if err != nil {
		return uiProjectMapInspection{}, fmt.Errorf("decode project map inspection: %w", err)
	}
	files, err := uiRequiredInspectionInteger(payload, "file_count")
	if err != nil {
		return uiProjectMapInspection{}, fmt.Errorf("decode project map inspection: %w", err)
	}
	modules, err := uiRequiredInspectionInteger(payload, "module_count")
	if err != nil {
		return uiProjectMapInspection{}, fmt.Errorf("decode project map inspection: %w", err)
	}
	return uiProjectMapInspection{TreePreview: tree, FileCount: files, ModuleCount: modules}, nil
}

func uiRequiredInspectionString(payload map[string]any, key string, maxBytes int, allowEmpty bool) (string, error) {
	value, ok := payload[key].(string)
	if !ok || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || len(value) > maxBytes || (!allowEmpty && value == "") {
		return "", fmt.Errorf("%s must be one bounded exact string", key)
	}
	return value, nil
}

func uiRequiredInspectionInteger(payload map[string]any, key string) (int64, error) {
	value, ok := uiExactInspectionInteger(payload[key])
	if !ok {
		return 0, fmt.Errorf("%s must be one non-negative exact integer", key)
	}
	return value, nil
}

func uiExactInspectionInteger(value any) (int64, bool) {
	switch current := value.(type) {
	case int:
		if current < 0 {
			return 0, false
		}
		return int64(current), true
	case int64:
		if current < 0 {
			return 0, false
		}
		return current, true
	case float64:
		if current < 0 || current > 9_007_199_254_740_991 || math.Trunc(current) != current {
			return 0, false
		}
		return int64(current), true
	default:
		return 0, false
	}
}
