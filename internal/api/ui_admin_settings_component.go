package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/secrets"
)

func renderUIAdminNetwork(settings map[string]any) string {
	coreURL := stringMapValue(settings, "core_url")
	host := stringMapValue(settings, "host")
	listen := stringMapValue(settings, "listen_addr")
	requestURL := stringMapValue(settings, "request_url")
	source := stringMapValue(settings, "source")
	port, _ := settings["port"].(int)
	if port == 0 {
		if numeric, ok := settings["port"].(float64); ok {
			port = int(numeric)
		}
	}
	requestHint := ""
	if requestURL != "" {
		requestHint = `<p class="mt-2 text-xs text-zinc-500">This browser session: <span class="font-mono text-zinc-300">` + uiEscape(requestURL) + `</span></p>`
	}
	return `<p class="text-sm text-zinc-400">Use this URL on other LAN devices, not localhost.</p>` +
		`<div class="mt-3 rounded-md border border-cyan-300/20 bg-cyan-300/5 px-3 py-2"><a href="` + uiAttribute(coreURL) +
		`" target="_blank" rel="noopener noreferrer" class="font-mono text-sm text-cyan-200">` + uiEscape(coreURL) + `</a>` +
		`<div class="mt-1 text-[11px] uppercase tracking-wide text-zinc-500">` + uiEscape(source) + ` · listen ` + uiEscape(listen) + `</div></div>` + requestHint +
		`<form data-action="submit->admin#saveNetwork" class="mt-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_120px_auto]">` +
		uiAdminInput("Host / IP", "networkHost", host, "text") + uiAdminInput("Port", "networkPort", strconv.Itoa(port), "number") +
		`<div class="self-end"><button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save URL</button></div></form>`
}

func uiAdminInput(label, field, value, inputType string) string {
	return `<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(label) + `</span><input data-admin-field="` + uiAttribute(field) +
		`" type="` + uiAttribute(inputType) + `" value="` + uiAttribute(value) +
		`" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm text-zinc-100 outline-none focus:border-cyan-300/40" /></label>`
}

func renderUIAdminMindStats(stats map[string]int64) string {
	if len(stats) == 0 {
		return `<p class="text-sm text-zinc-500">Mind stats require PostgreSQL.</p>`
	}
	rows := []struct{ label, key string }{
		{"Memory chunks", "memory_chunks"}, {"Memory candidates", "memory_candidates"},
		{"Pending review", "candidate_pending"}, {"Jobs", "jobs"}, {"Telemetry events", "telemetry_events"},
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

func renderUIOllamaModelList(endpoint string, models []ollama.ModelInfo, configured map[string]struct{}) string {
	var body strings.Builder
	body.WriteString(`<form data-action="submit->admin#pullModel" class="flex flex-wrap gap-2"><input data-admin-target="pullModel" placeholder="model:tag" class="min-w-[220px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm" /><button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Pull model</button></form>`)
	body.WriteString(`<p class="mb-3 mt-4 font-mono text-xs text-zinc-500">` + uiEscape(endpoint) + `</p>`)
	if len(models) == 0 {
		body.WriteString(`<p class="text-sm text-zinc-500">No models installed.</p>`)
		return body.String()
	}
	body.WriteString(`<div class="space-y-2">`)
	for _, model := range models {
		_, active := configured[model.Name]
		badge := ""
		if active {
			badge = `<span class="rounded-full border border-cyan-300/30 bg-cyan-300/10 px-2 py-0.5 text-[10px] text-cyan-200">In config</span>`
		}
		body.WriteString(`<article class="flex items-center justify-between gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2"><div><div class="font-mono text-sm text-zinc-100">` + uiEscape(model.Name) + `</div><div class="text-[11px] text-zinc-500">` + fmt.Sprintf("%.2f GB", float64(model.Size)/(1024*1024*1024)) + `</div></div><div class="flex items-center gap-2">` + badge + `<button type="button" data-action="admin#deleteOllamaModel" data-model-name="` + uiAttribute(model.Name) + `" class="rounded-md border border-rose-300/30 px-2 py-1 text-xs text-rose-200">Remove</button></div></article>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}

func renderUIModelFields(payload map[string]any) string {
	fields, _ := payload["fields"].([]map[string]any)
	envFile, _ := payload["env_file"].(string)
	return `<p class="mb-3 font-mono text-xs text-zinc-500">Env file: ` + uiEscape(envFile) + `</p>` +
		renderUIConfigFields(fields, "model_") +
		`<button type="button" data-action="admin#saveGlobalModels" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save global model settings</button>`
}

func renderUIAgentFields(fields []map[string]any) string {
	return renderUIConfigFields(fields, "agent_") +
		`<button type="button" data-action="admin#saveGlobalAgents" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save workspace agent settings</button>`
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

func renderUISecretFields(stored map[string]string) string {
	var body strings.Builder
	body.WriteString(`<div class="grid gap-4 lg:grid-cols-2">`)
	for _, field := range secrets.FieldList(stored) {
		key := stringMapValue(field, "key")
		label := stringMapValue(field, "label")
		description := stringMapValue(field, "description")
		source := stringMapValue(field, "source")
		hint := stringMapValue(field, "hint")
		status := source
		if hint != "" {
			status += " " + hint
		}
		body.WriteString(`<div class="rounded-md border border-white/10 bg-zinc-900/50 p-4"><div class="flex items-center justify-between gap-2"><span class="text-sm font-medium text-zinc-100">` + uiEscape(label) + `</span><span class="text-[10px] uppercase text-zinc-400">` + uiEscape(status) + `</span></div><input type="password" autocomplete="off" data-admin-field="secret_` + uiAttribute(key) + `" class="mt-3 w-full rounded-md border border-white/10 bg-zinc-950 px-3 py-2 font-mono text-xs" /><div class="mt-2 flex items-center justify-between gap-2"><span class="text-[11px] text-zinc-600">` + uiEscape(description) + `</span>`)
		if source == "database" {
			body.WriteString(`<button type="button" data-action="admin#clearSecret" data-secret-key="` + uiAttribute(key) + `" class="rounded-md border border-rose-300/30 px-2 py-1 text-[11px] text-rose-200">Clear stored</button>`)
		}
		body.WriteString(`</div></div>`)
	}
	body.WriteString(`</div><button type="button" data-action="admin#saveAPISecrets" class="mt-4 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Save API keys</button>`)
	return body.String()
}
