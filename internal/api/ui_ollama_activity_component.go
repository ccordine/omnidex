package api

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

func renderUIOllamaDownloads(page queue.OllamaModelDownloadPage) string {
	var body strings.Builder
	body.WriteString(`<div><div class="flex items-center justify-between gap-3"><div><h4 class="text-sm font-semibold text-zinc-100">Download activity</h4><p class="mt-1 text-xs text-zinc-500">Durable progress survives page changes and service restarts.</p></div><span class="text-[11px] uppercase tracking-wide text-zinc-500">Live</span></div>`)
	if len(page.Items) == 0 {
		body.WriteString(`<p class="mt-3 text-sm text-zinc-500">No model downloads yet.</p></div>`)
		return body.String()
	}
	body.WriteString(`<div class="mt-3 space-y-2">`)
	for _, item := range page.Items {
		tone := "text-zinc-400"
		activity := ""
		switch item.State {
		case queue.OllamaModelDownloadQueued, queue.OllamaModelDownloadRunning:
			tone = "text-cyan-200"
			activity = `<span class="chat-typing-indicator" aria-label="Download active"><span class="chat-typing-dot"></span><span class="chat-typing-dot"></span><span class="chat-typing-dot"></span></span>`
		case queue.OllamaModelDownloadCompleted:
			tone = "text-emerald-300"
		case queue.OllamaModelDownloadFailed:
			tone = "text-rose-300"
		}
		progress := ""
		if item.TotalBytes > 0 {
			percent := float64(item.CompletedBytes) / float64(item.TotalBytes) * 100
			progress = fmt.Sprintf(` · %.1f%%`, percent)
		}
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2"><div class="flex items-center justify-between gap-3"><span class="font-mono text-sm text-zinc-100">` + uiEscape(item.Model) + `</span><span class="flex items-center gap-2 text-xs ` + tone + `">` + activity + uiEscape(string(item.State)) + `</span></div><p class="mt-1 text-xs text-zinc-500">` + uiEscape(item.Status) + uiEscape(progress) + `</p>`)
		if item.Error != "" {
			body.WriteString(`<p class="mt-2 break-words text-xs text-rose-300">` + uiEscape(item.Error) + `</p>`)
		}
		body.WriteString(`</article>`)
	}
	body.WriteString(`</div>`)
	body.WriteString(renderUIDataPagination(
		"admin#loadDownloadPage", "download", page.Offset, len(page.Items), page.HasMore,
	))
	body.WriteString(`</div>`)
	return body.String()
}

func renderUIInstalledOllamaModels(
	endpoint string,
	page ollama.ModelPage,
	configured map[string]string,
) string {
	var body strings.Builder
	body.WriteString(`<div><h4 class="text-sm font-semibold text-zinc-100">Installed locally</h4><form data-action="submit->admin#pullModel" class="mt-3 flex flex-wrap gap-2"><input data-admin-target="pullModel" placeholder="Exact model:tag" class="min-w-[220px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-sm" /><button type="submit" class="rounded-md border border-cyan-300/30 px-4 py-2 text-sm font-semibold text-cyan-100">Download exact name</button></form>`)
	body.WriteString(`<p class="mb-3 mt-3 font-mono text-xs text-zinc-500">` + uiEscape(endpoint) + `</p>`)
	if len(page.Models) == 0 {
		body.WriteString(`<p class="text-sm text-zinc-500">No models installed.</p></div>`)
		return body.String()
	}
	body.WriteString(`<div class="space-y-2">`)
	for _, model := range page.Models {
		usage := configured[model.Name]
		badge := ""
		if usage != "" {
			badge = `<span class="rounded-full border border-cyan-300/30 bg-cyan-300/10 px-2 py-0.5 text-[10px] text-cyan-200">` + uiEscape(usage) + `</span>`
		}
		details := []string{fmt.Sprintf("%.2f GB", float64(model.Size)/(1024*1024*1024))}
		for _, value := range []string{model.Details.Family, model.Details.ParameterSize, model.Details.QuantizationLevel} {
			if value != "" {
				details = append(details, value)
			}
		}
		body.WriteString(`<article class="flex items-center justify-between gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2"><div><div class="font-mono text-sm text-zinc-100">` + uiEscape(model.Name) + `</div><div class="text-[11px] text-zinc-500">` + uiEscape(strings.Join(details, " · ")) + `</div></div><div class="flex items-center gap-2">` + badge + `<button type="button" data-action="admin#deleteOllamaModel" data-model-name="` + uiAttribute(model.Name) + `" class="rounded-md border border-rose-300/30 px-2 py-1 text-xs text-rose-200">Remove</button></div></article>`)
	}
	body.WriteString(`</div>`)
	body.WriteString(renderUIDataPagination(
		"admin#loadModelPage", "model", page.Offset, len(page.Models), page.HasMore,
	))
	body.WriteString(`</div>`)
	return body.String()
}
