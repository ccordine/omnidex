package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
)

func renderResearchStatusHTML(status researchStatusResponse) string {
	var output strings.Builder
	output.WriteString(`<div class="space-y-3" role="status" aria-live="polite">`)
	output.WriteString(`<div class="grid gap-2 sm:grid-cols-2">`)
	output.WriteString(renderProviderRoleStatusHTML("Generation", status.GenerationProvider))
	output.WriteString(renderProviderRoleStatusHTML("Embeddings", status.EmbeddingProvider))
	output.WriteString(renderResearchMetricHTML("Runnable", yesNo(status.ResearchRunnable), runnableTone(status.ResearchRunnable)))
	output.WriteString(renderResearchMetricHTML("Web context", webSearchState(status.WebSearch), webSearchTone(status.WebSearch)))
	output.WriteString(`</div>`)
	if status.Ollama != nil {
		output.WriteString(renderOllamaRuntimeHTML(*status.Ollama))
	}
	if len(status.WebSearch.Probes) > 0 {
		output.WriteString(renderWebSearchProbesHTML(status.WebSearch.Probes))
	}
	if len(status.Warnings) > 0 {
		output.WriteString(`<div class="rounded-md border border-amber-300/30 bg-amber-300/10 p-3 text-sm text-amber-100"><div class="font-semibold">Attention required</div><ul class="mt-1 list-disc space-y-1 pl-5">`)
		for _, warning := range status.Warnings {
			fmt.Fprintf(&output, `<li>%s</li>`, html.EscapeString(warning))
		}
		output.WriteString(`</ul></div>`)
	}
	output.WriteString(`</div>`)
	return output.String()
}

func renderProviderRoleStatusHTML(role string, status generationProviderStatus) string {
	name := status.Provider
	if definition, ok := catalog.Lookup(status.Provider); ok {
		name = definition.DisplayName
	}
	detail := "configuration invalid"
	if status.State == "configured" && !status.Probed {
		detail = "configured · not probed"
	} else if status.State != "" {
		detail = strings.ReplaceAll(status.State, "_", " ")
	}
	var output strings.Builder
	fmt.Fprintf(&output, `<section class="rounded-md border p-3 %s"><div class="flex items-start justify-between gap-3"><div><div class="text-[11px] font-semibold uppercase tracking-[.16em] text-zinc-500">%s</div><div class="mt-1 text-sm font-semibold text-zinc-100">%s</div></div><span class="rounded-full border px-2 py-0.5 text-[11px] font-medium">%s</span></div>`, providerStateTone(status.State), html.EscapeString(role), html.EscapeString(name), html.EscapeString(detail))
	if status.Model != "" {
		fmt.Fprintf(&output, `<div class="mt-2 truncate font-mono text-xs text-zinc-400" title="%s">%s</div>`, html.EscapeString(status.Model), html.EscapeString(status.Model))
	}
	if status.Error != "" {
		fmt.Fprintf(&output, `<div class="mt-2 text-xs leading-5 text-rose-200">%s</div>`, html.EscapeString(status.Error))
	}
	output.WriteString(`</section>`)
	return output.String()
}

func renderResearchMetricHTML(label, value, tone string) string {
	return fmt.Sprintf(`<div class="rounded-md border border-white/10 bg-white/[.03] p-3"><div class="text-[11px] font-semibold uppercase tracking-[.16em] text-zinc-500">%s</div><div class="mt-1 text-sm font-semibold %s">%s</div></div>`, html.EscapeString(label), tone, html.EscapeString(value))
}

func renderOllamaRuntimeHTML(status ollamaRuntimeStatus) string {
	var output strings.Builder
	output.WriteString(`<section class="rounded-md border border-white/10 bg-white/[.03] p-3"><div class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Ollama runtime</div><dl class="mt-2 space-y-1 font-mono text-xs text-zinc-300">`)
	writeResearchDetail(&output, "endpoint", status.BaseURL)
	writeResearchDetail(&output, "required", strings.Join(status.ConfiguredModels, ", "))
	writeResearchDetail(&output, "missing", strings.Join(status.MissingModels, ", "))
	if status.EmbeddingModel != "" {
		embeddingState := "missing"
		if status.EmbeddingAvailable {
			embeddingState = "available"
		}
		writeResearchDetail(&output, "embedding", status.EmbeddingModel+" ("+embeddingState+")")
	}
	if status.LastProviderError != "" {
		fmt.Fprintf(&output, `<div class="text-rose-200"><dt class="inline text-rose-300">error</dt> <dd class="inline">%s</dd></div>`, html.EscapeString(status.LastProviderError))
	}
	output.WriteString(`</dl></section>`)
	return output.String()
}

func renderWebSearchProbesHTML(probes []webSearchProbeStatus) string {
	var output strings.Builder
	output.WriteString(`<section class="rounded-md border border-white/10 bg-white/[.03] p-3"><div class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Web probes</div><div class="mt-2 space-y-2">`)
	for _, probe := range probes {
		state := "failed"
		stateClass := "text-rose-200"
		if probe.Reachable {
			state = fmt.Sprintf("HTTP %d", probe.StatusCode)
			stateClass = "text-emerald-200"
		}
		fmt.Fprintf(&output, `<div class="flex items-start justify-between gap-3 rounded border border-white/10 bg-zinc-950/40 p-2 font-mono text-xs"><div class="min-w-0"><div class="text-zinc-200">%s</div><div class="truncate text-zinc-500">%s</div>`, html.EscapeString(probe.Provider), html.EscapeString(probe.TargetURL))
		if probe.Error != "" {
			fmt.Fprintf(&output, `<div class="mt-1 text-rose-200">%s</div>`, html.EscapeString(probe.Error))
		}
		fmt.Fprintf(&output, `</div><span class="shrink-0 %s">%s</span></div>`, stateClass, html.EscapeString(state))
	}
	output.WriteString(`</div></section>`)
	return output.String()
}

func writeResearchDetail(output *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		value = "none"
	}
	fmt.Fprintf(output, `<div><dt class="inline text-zinc-500">%s</dt> <dd class="inline">%s</dd></div>`, html.EscapeString(label), html.EscapeString(value))
}

func providerStateTone(state string) string {
	switch state {
	case "reachable":
		return "border-emerald-300/30 bg-emerald-300/5 text-emerald-200"
	case "configured":
		return "border-cyan-300/30 bg-cyan-300/5 text-cyan-200"
	case "degraded":
		return "border-amber-300/30 bg-amber-300/5 text-amber-200"
	default:
		return "border-rose-300/30 bg-rose-300/5 text-rose-200"
	}
}

func webSearchState(status webSearchRuntimeStatus) string {
	if !status.Enabled {
		return "disabled"
	}
	if status.ReachableProvider {
		return "reachable"
	}
	return "degraded"
}

func webSearchTone(status webSearchRuntimeStatus) string {
	if !status.Enabled {
		return "text-zinc-300"
	}
	if status.ReachableProvider {
		return "text-emerald-200"
	}
	return "text-amber-200"
}

func runnableTone(runnable bool) string {
	if runnable {
		return "text-emerald-200"
	}
	return "text-rose-200"
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
