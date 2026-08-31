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
