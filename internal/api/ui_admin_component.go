package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

var uiAdminTabs = map[string]struct{}{
	"overview": {}, "ai": {}, "datasources": {}, "health": {},
}

func (s *Server) handleUIAdminComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tab, err := exactUIQuery(r, "tab", 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if tab == "" {
		tab = "overview"
	}
	if _, ok := uiAdminTabs[tab]; !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported admin tab %q", tab))
		return
	}
	body, err := s.renderUIAdminTab(r, tab)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeUIOperationalComponent(w, "admin-tab-panel", body)
}

func (s *Server) renderUIAdminTab(r *http.Request, tab string) (string, error) {
	switch tab {
	case "overview":
		return s.renderUIAdminOverview(r)
	case "ai":
		query, err := parseUIOllamaManagerQuery(r)
		if err != nil {
			return "", err
		}
		return s.renderUIAdminAI(r.Context(), query)
	case "datasources":
		return `<div data-admin-tab-panel="datasources" class="mx-auto max-w-6xl space-y-4">` +
			`<div data-controller="admin-data-sources" data-recyclr-sink="admin-data-sources" class="space-y-4">` + uiLoading("Loading data sources…") + `</div></div>`, nil
	case "health":
		return renderUIAdminHealth(), nil
	default:
		return "", fmt.Errorf("unsupported admin tab %q", tab)
	}
}

func uiAdminSection(title, description, body string) string {
	return `<section class="rounded-xl border border-white/10 bg-zinc-950/50 p-5"><div class="mb-4">` +
		`<h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-400">` + uiEscape(title) + `</h3>` +
		`<p class="mt-1 text-xs leading-5 text-zinc-500">` + uiEscape(description) + `</p></div>` + body + `</section>`
}

func (s *Server) renderUIAdminOverview(r *http.Request) (string, error) {
	stats := map[string]int64{}
	if s.repo != nil {
		var err error
		stats, err = s.repo.MindStats(r.Context())
		if err != nil {
			return "", fmt.Errorf("load mind statistics: %w", err)
		}
	}
	return `<div data-admin-tab-panel="overview" class="mx-auto max-w-5xl space-y-4">` +
		uiAdminSection("Mind overview", "Counts for durable memory, candidates, jobs, and telemetry.", renderUIAdminMindStats(stats)) +
		uiAdminSection("Document ingest", "Upload reference documents into explicit candidate or durable staging.", renderUIAdminIngest()) +
		`</div>`, nil
}

func (s *Server) renderUIAdminAI(ctx context.Context, query uiOllamaManagerQuery) (string, error) {
	modelsBody, err := s.renderUIOllamaManager(ctx, query)
	if err != nil {
		return "", err
	}
	return `<div data-admin-tab-panel="ai" class="mx-auto max-w-5xl space-y-4">` +
		uiAdminSection("Local Ollama model manager", "Search the official catalog, watch durable downloads, and manage models available to character personalities.", modelsBody) +
		`</div>`, nil
}

func renderUIAdminHealth() string {
	return `<div data-admin-tab-panel="health" class="mx-auto max-w-6xl space-y-4">` +
		uiAdminSection("Core health", "Live health from the running core service.", `<pre data-chat-target="statusOutput" data-recyclr-sink="status-output" class="scrollbar max-h-[360px] overflow-y-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-zinc-900/60 p-4 text-sm leading-6 text-zinc-200">`+uiLoading("Loading health…")+`</pre>`) +
		uiAdminSection("Host bridge", "Folder, terminal, and screen bridge state.", `<div data-chat-target="hostBridgeStatusOutput" data-recyclr-sink="host-bridge-status-output">`+uiLoading("Loading host bridge…")+`</div>`) + `</div>`
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
