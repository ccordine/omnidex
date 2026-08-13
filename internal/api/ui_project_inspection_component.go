package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

func renderUIProjectMap(projectID int64, payload map[string]any) string {
	files := stringMapValue(payload, "tree_preview")
	fileCount := uiAnyInteger(payload["file_count"])
	moduleCount := uiAnyInteger(payload["module_count"])
	return `<div data-project-tab-panel="map" class="scrollbar space-y-4"><section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><div class="flex items-start justify-between gap-3"><div><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Codebase map</h3><p class="mt-1 text-xs text-zinc-500">Current server-inspected repository context.</p></div><button type="button" data-action="projects#scanProjectMap" data-project-id="` + uiInt(projectID) + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Refresh map</button></div><div class="mt-4 grid gap-3 sm:grid-cols-2"><div class="rounded-md border border-white/10 p-3"><span class="text-xs text-zinc-500">Files</span><div class="font-mono text-lg text-cyan-200">` + fileCount + `</div></div><div class="rounded-md border border-white/10 p-3"><span class="text-xs text-zinc-500">Modules</span><div class="font-mono text-lg text-cyan-200">` + moduleCount + `</div></div></div><pre class="scrollbar mt-4 max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-white/10 bg-black/40 p-3 font-mono text-[11px]">` + uiEscape(files) + `</pre></section></div>`
}

func renderUIProjectGit(projectID int64, payload map[string]any) string {
	if isRepo, _ := payload["is_repo"].(bool); !isRepo {
		return `<div data-project-tab-panel="git"><p class="rounded-md border border-amber-300/20 p-4 text-sm text-amber-200">This project is not a Git repository.</p></div>`
	}
	branch := stringMapValue(payload, "branch")
	head := stringMapValue(payload, "head_short")
	return `<div data-project-tab-panel="git" class="scrollbar space-y-4"><section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><div class="flex items-center justify-between gap-3"><div><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Git</h3><p class="mt-1 font-mono text-sm text-cyan-200">` + uiEscape(branch) + ` · ` + uiEscape(head) + `</p></div><button type="button" data-action="projects#refreshProjectGit" data-project-id="` + uiInt(projectID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Refresh</button></div><div class="mt-4 grid gap-2 sm:grid-cols-4">` + uiGitMetric("Staged", payload["staged_count"]) + uiGitMetric("Modified", payload["modified_count"]) + uiGitMetric("Untracked", payload["untracked_count"]) + uiGitMetric("Deleted", payload["deleted_count"]) + `</div></section></div>`
}

func uiGitMetric(label string, value any) string {
	return `<div class="rounded-md border border-white/10 p-3"><div class="text-[11px] uppercase text-zinc-500">` + label + `</div><div class="mt-1 font-mono text-lg">` + uiAnyInteger(value) + `</div></div>`
}

func uiAnyInteger(value any) string {
	switch current := value.(type) {
	case int:
		return fmt.Sprintf("%d", current)
	case int64:
		return fmt.Sprintf("%d", current)
	case float64:
		return fmt.Sprintf("%.0f", current)
	default:
		return "0"
	}
}

func (s *Server) renderUIProjectRecipe(project model.Project, offset int) (string, error) {
	page, err := omni.LoadRecipePage(s.recipeRoot(), dataSourceUIPageSize, offset)
	if err != nil {
		return "", err
	}
	var options strings.Builder
	options.WriteString(`<option value="">No catalog recipe</option>`)
	for _, recipe := range page.Recipes {
		selected := ""
		if recipe.ID == project.RecipeID {
			selected = " selected"
		}
		options.WriteString(`<option value="` + uiAttribute(recipe.ID) + `"` + selected + `>` + uiEscape(recipe.ID) + `</option>`)
	}
	raw := project.Recipe
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var compact any
	if err := json.Unmarshal(raw, &compact); err != nil {
		return "", fmt.Errorf("decode project recipe: %w", err)
	}
	pretty, err := json.MarshalIndent(compact, "", "  ")
	if err != nil {
		return "", err
	}
	return `<div data-project-tab-panel="recipe" class="scrollbar space-y-4"><section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex recipe</h3><select data-projects-field="recipeId" class="mt-4 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm">` + options.String() + `</select>` + renderUIDataPagination("projects#loadRecipePage", "recipe", page.Offset, len(page.Recipes), page.HasMore) + `<textarea data-projects-field="recipeJson" rows="18" class="scrollbar mt-4 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs">` + uiEscape(string(pretty)) + `</textarea><button type="button" data-action="projects#saveRecipe" data-project-id="` + uiInt(project.ID) + `" class="mt-3 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save recipe</button></section></div>`, nil
}
