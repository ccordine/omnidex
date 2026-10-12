package api

import (
	"strconv"
	"strings"
)

func renderUIAdminMindStats(stats map[string]int64) string {
	if len(stats) == 0 {
		return `<p class="text-sm text-zinc-500">Mind stats require PostgreSQL.</p>`
	}
	rows := []struct{ label, key string }{
		{"Memory chunks", "memory_chunks"}, {"Memory candidates", "memory_candidates"},
		{"Pending review", "candidate_pending"}, {"Jobs", "jobs"},
	}
	var body strings.Builder
	body.WriteString(`<div class="grid gap-2 sm:grid-cols-2">`)
	for _, row := range rows {
		body.WriteString(`<div class="rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2"><div class="text-[11px] uppercase tracking-wide text-zinc-500">`)
		body.WriteString(uiEscape(row.label))
		body.WriteString(`</div><div class="mt-1 font-mono text-lg text-cyan-200">`)
		body.WriteString(strconv.FormatInt(stats[row.key], 10))
		body.WriteString(`</div></div>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}

func renderUIAdminIngest() string {
	return `<form data-action="submit->admin#uploadDocuments" class="mt-4 space-y-3">` +
		`<input data-admin-target="ingestFiles" type="file" multiple class="block w-full text-sm text-zinc-300 file:mr-3 file:rounded-md file:border-0 file:bg-cyan-300 file:px-3 file:py-2" />` +
		`<div class="grid gap-3 md:grid-cols-2"><label class="block text-xs text-zinc-500">Staging<select data-admin-target="ingestStage" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2"><option value="candidate">Candidate review</option><option value="durable">Durable memory</option></select></label>` +
		`<label class="block text-xs text-zinc-500">Extra tags<input data-admin-target="ingestTags" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2" /></label></div>` +
		`<button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Upload and study</button></form>`
}

func renderUIAdminCodingAgents() string {
	var body strings.Builder
	body.WriteString(`<div data-controller="coding-agent-settings" data-action="coding-agent-settings:status->admin#receiveStatus" class="space-y-4">`)
	body.WriteString(`<p class="text-xs text-zinc-500">Applies to new jobs. Codex models come from your account; Refresh reloads the catalog. Update changes the installed Codex CLI.</p>`)
	body.WriteString(`<div class="space-y-3">`)
	for _, role := range []struct{ key, label string }{{"planner", "Task planner"}, {"executor", "Executor"}, {"reviewer", "Reviewer"}} {
		body.WriteString(`<fieldset class="grid min-w-0 gap-3 rounded-lg border border-white/10 p-3 sm:grid-cols-3"><legend class="px-1 text-sm text-zinc-300">` + role.label + `</legend>`)
		for _, field := range []struct{ key, label string }{{"model", "Model"}, {"effort", "Effort"}, {"service_tier", "Speed"}} {
			action := `change->coding-agent-settings#changed`
			if field.key == "model" {
				action = `change->coding-agent-settings#modelChanged`
			}
			body.WriteString(`<label class="min-w-0 text-xs text-zinc-500">` + field.label + `<select disabled data-agent-role="` + role.key + `" data-agent-field="` + field.key + `" data-action="` + action + `" aria-label="` + role.label + ` ` + field.label + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/50 disabled:opacity-50"></select></label>`)
		}
		body.WriteString(`</fieldset>`)
	}
	body.WriteString(`</div><div class="flex flex-wrap items-center gap-2"><button type="button" disabled data-coding-agent-settings-target="save" data-action="coding-agent-settings#save" class="rounded-md bg-cyan-300 px-3 py-2 text-sm font-semibold text-zinc-950 disabled:opacity-50">Save</button><button type="button" data-coding-agent-settings-target="refresh" data-action="coding-agent-settings#refresh" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-100 disabled:opacity-50">Refresh</button><button type="button" disabled data-coding-agent-settings-target="update" data-action="coding-agent-settings#update" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-100 disabled:opacity-50">Update</button><span data-coding-agent-settings-target="version" class="text-xs text-zinc-500"></span></div><p data-coding-agent-settings-target="status" role="status" aria-live="polite" class="text-sm text-zinc-400">Loading…</p></div>`)
	return body.String()
}

func renderUIConfigFields(fields []map[string]any, prefix string) string {
	var body strings.Builder
	body.WriteString(`<div class="grid gap-4 lg:grid-cols-2">`)
	for _, field := range fields {
		key := stringMapValue(field, "key")
		label := stringMapValue(field, "label")
		description := stringMapValue(field, "description")
		value := stringMapValue(field, "value")
		body.WriteString(`<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(label) + `</span>`)
		options, _ := field["options"].([]string)
		if len(options) == 0 {
			body.WriteString(`<input data-admin-field="` + uiAttribute(prefix+key) + `" value="` + uiAttribute(value) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs" />`)
		} else {
			body.WriteString(`<select data-admin-field="` + uiAttribute(prefix+key) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs"><option value="">Default</option>`)
			for _, option := range options {
				selected := ""
				if value == option {
					selected = " selected"
				}
				body.WriteString(`<option value="` + uiAttribute(option) + `"` + selected + `>` + uiEscape(option) + `</option>`)
			}
			body.WriteString(`</select>`)
		}
		body.WriteString(`<span class="mt-1 block text-[11px] text-zinc-600">` + uiEscape(description) + `</span></label>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}
