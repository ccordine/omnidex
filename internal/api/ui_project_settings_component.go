package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func (s *Server) renderUIProjectSettings(r *http.Request, project model.Project) (string, error) {
	models, err := s.resolvedModelsForProject(r.Context(), project.ID)
	if err != nil {
		return "", fmt.Errorf("resolve project models: %w", err)
	}
	automation, err := loadScrumAutoWorkConfig(project.Settings)
	if err != nil {
		return "", err
	}
	modelFields, err := decodeUIProjectModelFields(models)
	if err != nil {
		return "", err
	}
	modelOverrides, err := s.projectModelConfig(project)
	if err != nil {
		return "", err
	}
	return `<div data-project-tab-panel="settings" class="scrollbar space-y-4">` +
		uiProjectSettingsSection(project) +
		uiProjectConfigSection(project.ID, project.UpdatedAt, "Project model overrides", "project-model", "projects#saveModelConfig", "projects#clearModelConfig", modelFields, modelOverrides) +
		renderUIProjectAutomation(project.ID, automation) + `</div>`, nil
}

func uiProjectSettingsSection(project model.Project) string {
	revision := uiAttribute(project.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return `<section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Project</h3><div class="mt-4 grid gap-4 lg:grid-cols-2">` + uiProjectField("name", project.Name) + `<div><div class="flex items-end gap-2">` + uiProjectField("location", project.Location) + `<button type="button" data-action="projects#browseForEdit" data-project-id="` + uiInt(project.ID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Browse…</button></div></div><label class="block lg:col-span-2"><span class="text-xs text-zinc-500">Description</span><textarea data-projects-field="description" rows="3" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm">` + uiEscape(project.Description) + `</textarea></label></div><div class="mt-4 flex gap-2"><button type="button" data-action="projects#saveProject" data-project-id="` + uiInt(project.ID) + `" data-project-updated-at="` + revision + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save project</button><button type="button" data-action="projects#rescanProject" data-project-id="` + uiInt(project.ID) + `" class="rounded-md border border-white/10 px-3 py-2 text-sm">Detect stack</button><button type="button" data-action="projects#deleteProject" data-project-id="` + uiInt(project.ID) + `" data-project-updated-at="` + revision + `" class="rounded-md border border-rose-400/30 px-4 py-2 text-sm text-rose-300">Delete</button></div></section>`
}

type uiProjectModelField struct {
	Key     string
	Label   string
	Value   string
	Options []string
}

func uiProjectConfigSection(projectID int64, updatedAt time.Time, title, prefix, saveAction, clearAction string, fields []uiProjectModelField, overrides map[string]string) string {
	var body strings.Builder
	body.WriteString(`<section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5"><h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">` + uiEscape(title) + `</h3><div class="mt-4 grid gap-4 lg:grid-cols-2">`)
	for _, field := range fields {
		body.WriteString(`<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(field.Label) + `</span>`)
		if len(field.Options) > 0 {
			body.WriteString(`<select data-project-config="` + uiAttribute(prefix) + `" data-config-key="` + uiAttribute(field.Key) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs"><option value="">Use inherited: ` + uiEscape(field.Value) + `</option>`)
			for _, option := range field.Options {
				selected := ""
				if overrides[field.Key] == option {
					selected = " selected"
				}
				body.WriteString(`<option value="` + uiAttribute(option) + `"` + selected + `>` + uiEscape(option) + `</option>`)
			}
			body.WriteString(`</select>`)
		} else {
			body.WriteString(`<input data-project-config="` + uiAttribute(prefix) + `" data-config-key="` + uiAttribute(field.Key) + `" value="` + uiAttribute(overrides[field.Key]) + `" placeholder="Inherited: ` + uiAttribute(field.Value) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs" />`)
		}
		body.WriteString(`</label>`)
	}
	revision := uiAttribute(updatedAt.UTC().Format(time.RFC3339Nano))
	body.WriteString(`</div><div class="mt-4 flex gap-2"><button type="button" data-action="` + saveAction + `" data-project-id="` + uiInt(projectID) + `" data-project-updated-at="` + revision + `" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save</button><button type="button" data-action="` + clearAction + `" data-project-id="` + uiInt(projectID) + `" data-project-updated-at="` + revision + `" class="rounded-md border border-white/10 px-4 py-2 text-sm">Clear overrides</button></div></section>`)
	return body.String()
}

func decodeUIProjectModelFields(models map[string]any) ([]uiProjectModelField, error) {
	rawFields, ok := models["fields"].([]map[string]any)
	if !ok || len(rawFields) != len(modelconfig.Fields) {
		return nil, fmt.Errorf("project model field inventory is not exact")
	}
	fields := make([]uiProjectModelField, 0, len(rawFields))
	for index, raw := range rawFields {
		definition := modelconfig.Fields[index]
		if len(raw) != 6 || raw["key"] != definition.Key || raw["label"] != definition.Label ||
			raw["description"] != definition.Description {
			return nil, fmt.Errorf("project model field %d does not match its registered definition", index)
		}
		envKeys, ok := raw["env_keys"].([]string)
		if !ok || !equalExactStrings(envKeys, definition.EnvKeys) {
			return nil, fmt.Errorf("project model field %q has invalid environment authority", definition.Key)
		}
		options, ok := raw["options"].([]string)
		if !ok || !equalExactStrings(options, definition.Options) {
			return nil, fmt.Errorf("project model field %q has invalid option authority", definition.Key)
		}
		value, ok := raw["value"].(string)
		if !ok || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || len(value) > 4096 || value != strings.TrimSpace(value) {
			return nil, fmt.Errorf("project model field %q has invalid resolved value", definition.Key)
		}
		fields = append(fields, uiProjectModelField{Key: definition.Key, Label: definition.Label, Value: value, Options: options})
	}
	return fields, nil
}

func equalExactStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
