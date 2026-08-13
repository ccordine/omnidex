package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) renderUIProjectSettings(r *http.Request, project model.Project) (string, error) {
	models, err := s.resolvedModelsForProjectCard(r.Context(), project.ID, ScrumCard{})
	if err != nil {
		return "", fmt.Errorf("resolve project models: %w", err)
	}
	agents, err := s.resolvedAgentsForProjectCard(r.Context(), project.ID, ScrumCard{})
	if err != nil {
		return "", fmt.Errorf("resolve project agents: %w", err)
	}
	automation, err := loadScrumAutoWorkConfig(project.Settings)
	if err != nil {
		return "", err
	}
	modelFields, _ := models["fields"].([]map[string]any)
	agentFields, _ := agents["fields"].([]map[string]any)
	modelOverrides, err := s.projectModelConfig(project)
	if err != nil {
		return "", err
	}
	agentOverrides, err := s.projectAgentConfig(project)
	if err != nil {
		return "", err
	}
	return `<div data-project-tab-panel="settings" class="scrollbar space-y-4">` +
		uiProjectSettingsSection(project) +
		uiProjectConfigSection("Project model overrides", "project-model", "projects#saveModelConfig", "projects#clearModelConfig", modelFields, modelOverrides) +
		uiProjectConfigSection("Project agent overrides", "project-agent", "projects#saveAgentConfig", "projects#clearAgentConfig", agentFields, agentOverrides) +
		renderUIProjectAutomation(project.ID, automation) + `</div>`, nil
}

func uiProjectSettingsSection(project model.Project) string {
	return `<section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Project</h3><div class="mt-4 grid gap-4 lg:grid-cols-2">` + uiProjectField("name", project.Name) + `<div><div class="flex items-end gap-2">` + uiProjectField("location", project.Location) + `<button type="button" data-action="projects#browseForEdit" data-project-id="` + uiInt(project.ID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Browse…</button></div></div><label class="block lg:col-span-2"><span class="text-xs text-zinc-500">Description</span><textarea data-projects-field="description" rows="3" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm">` + uiEscape(project.Description) + `</textarea></label></div><div class="mt-4 flex gap-2"><button type="button" data-action="projects#saveProject" data-project-id="` + uiInt(project.ID) + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save project</button><button type="button" data-action="projects#rescanProject" data-project-id="` + uiInt(project.ID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Detect stack</button><button type="button" data-action="projects#deleteProject" data-project-id="` + uiInt(project.ID) + `" class="rounded-md border border-rose-400/30 px-4 py-2 text-sm text-rose-300">Delete</button></div></section>`
}

func uiProjectConfigSection(title, prefix, saveAction, clearAction string, fields []map[string]any, overrides map[string]string) string {
	var body strings.Builder
	body.WriteString(`<section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">` + uiEscape(title) + `</h3><div class="mt-4 grid gap-4 lg:grid-cols-2">`)
	for _, field := range fields {
		key, label := stringMapValue(field, "key"), stringMapValue(field, "label")
		resolved := stringMapValue(field, "value")
		body.WriteString(`<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(label) + `</span>`)
		if options, ok := field["options"].([]string); ok && len(options) > 0 {
			body.WriteString(`<select data-project-config="` + uiAttribute(prefix) + `" data-config-key="` + uiAttribute(key) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs"><option value="">Use inherited: ` + uiEscape(resolved) + `</option>`)
			for _, option := range options {
				selected := ""
				if overrides[key] == option {
					selected = " selected"
				}
				body.WriteString(`<option value="` + uiAttribute(option) + `"` + selected + `>` + uiEscape(option) + `</option>`)
			}
			body.WriteString(`</select>`)
		} else {
			body.WriteString(`<input data-project-config="` + uiAttribute(prefix) + `" data-config-key="` + uiAttribute(key) + `" value="` + uiAttribute(overrides[key]) + `" placeholder="Inherited: ` + uiAttribute(resolved) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs" />`)
		}
		body.WriteString(`</label>`)
	}
	body.WriteString(`</div><div class="mt-4 flex gap-2"><button type="button" data-action="` + saveAction + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save</button><button type="button" data-action="` + clearAction + `" class="rounded-md border border-white/10 px-4 py-2 text-sm">Clear overrides</button></div></section>`)
	return body.String()
}

func renderUIProjectAutomation(projectID int64, config ScrumAutoWorkConfig) string {
	checked := ""
	if config.Enabled {
		checked = " checked"
	}
	selected := map[string]bool{}
	for _, column := range config.SourceColumns {
		selected[column] = true
	}
	var options strings.Builder
	for _, column := range []string{"backlog", "ready", "assigned", "in_progress", "blocked"} {
		state := ""
		if selected[column] {
			state = " checked"
		}
		options.WriteString(`<label class="flex items-center gap-2 rounded-md border border-white/10 px-3 py-2 text-xs"><input type="checkbox" data-projects-field="autoWorkColumn" data-auto-work-column="` + column + `"` + state + ` />` + uiEscape(column) + `</label>`)
	}
	return `<section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Scrum automation</h3><label class="mt-4 flex items-center gap-2"><input type="checkbox" data-projects-field="autoWorkEnabled"` + checked + ` />Auto-work queue</label><div class="mt-3 flex flex-wrap gap-2">` + options.String() + `</div><button type="button" data-action="projects#saveScrumAutomation" data-project-id="` + uiInt(projectID) + `" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save automation</button></section>`
}
