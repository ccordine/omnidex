package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var uiAdminTabs = map[string]struct{}{
	"overview": {}, "ai": {}, "datasources": {}, "health": {}, "advanced": {},
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
		modelOffset, err := exactChannelQueryInteger(r, "model_offset", 0, 0, 1<<30)
		if err != nil {
			return "", err
		}
		return s.renderUIAdminAI(r.Context(), modelOffset)
	case "datasources":
		return `<div data-admin-tab-panel="datasources" class="mx-auto max-w-6xl space-y-4">` +
			`<div data-controller="admin-data-sources" data-recyclr-sink="admin-data-sources" class="space-y-4">` + uiLoading("Loading data sources…") + `</div></div>`, nil
	case "health":
		return renderUIAdminHealth(), nil
	case "advanced":
		return renderUIAdminAdvanced(), nil
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
	network, err := s.networkSettingsPayload(r)
	if err != nil {
		return "", err
	}
	stats := map[string]int64{}
	if s.repo != nil {
		stats, err = s.repo.MindStats(r.Context())
		if err != nil {
			return "", fmt.Errorf("load mind statistics: %w", err)
		}
	}
	return `<div data-admin-tab-panel="overview" class="mx-auto max-w-5xl space-y-4">` +
		uiAdminSection("Network access", "LAN URL for phones, tablets, and other devices on your network.", renderUIAdminNetwork(network)) +
		uiAdminSection("Mind overview", "Counts for durable memory, candidates, jobs, and telemetry.", renderUIAdminMindStats(stats)) +
		uiAdminSection("Document ingest", "Upload reference documents into explicit candidate or durable staging.", renderUIAdminIngest()) +
		`</div>`, nil
}

func (s *Server) renderUIAdminAI(ctx context.Context, modelOffset int) (string, error) {
	modelSettings, err := buildModelSettingsResponse()
	if err != nil {
		return "", fmt.Errorf("load model settings: %w", err)
	}
	storedSecrets := map[string]string{}
	if s.repo != nil {
		storedSecrets, err = s.repo.GetAPISecrets(ctx)
		if err != nil {
			return "", fmt.Errorf("load stored API secrets: %w", err)
		}
	}
	modelsBody, err := s.renderUIOllamaModels(ctx, modelOffset)
	if err != nil {
		return "", err
	}
	return `<div data-admin-tab-panel="ai" class="mx-auto max-w-5xl space-y-4">` +
		uiAdminSection("API keys", "Stored in PostgreSQL; environment values are used only when no database value is set.", renderUISecretFields(storedSecrets)) +
		uiAdminSection("Ollama models", "Pull, inspect, and remove local models used by the stack.", modelsBody) +
		uiAdminSection("Global model defaults", "Exact station model settings from the authoritative environment file.", renderUIModelFields(modelSettings)) +
		`</div>`, nil
}

func (s *Server) renderUIOllamaModels(parent context.Context, offset int) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	page, err := s.ollamaClient().ListModelPage(ctx, dataSourceUIPageSize, offset)
	if err != nil {
		return "", fmt.Errorf("Ollama models unavailable: %w", err)
	}
	configured := map[string]struct{}{}
	if cfg, configErr := s.envModelConfig(); configErr == nil {
		for _, name := range cfg.ModelNames() {
			configured[name] = struct{}{}
		}
	}
	return renderUIOllamaModelList(s.ollamaEndpoint(), page.Models, configured) +
		renderUIDataPagination("admin#loadModelPage", "model", page.Offset, len(page.Models), page.HasMore), nil
}

func renderUIAdminHealth() string {
	return `<div data-admin-tab-panel="health" class="mx-auto max-w-6xl space-y-4">` +
		uiAdminSection("Core health", "Live health from the running core service.", `<pre data-chat-target="statusOutput" data-recyclr-sink="status-output" class="scrollbar max-h-[360px] overflow-y-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-zinc-900/60 p-4 text-sm leading-6 text-zinc-200">`+uiLoading("Loading health…")+`</pre>`) +
		`<div class="grid gap-4 lg:grid-cols-2">` +
		uiAdminSection("Research stack", "Generation, embedding, runtime, and search readiness.", `<div data-chat-target="researchStatusOutput" data-recyclr-sink="research-status-output">`+uiLoading("Loading research health…")+`</div>`) +
		uiAdminSection("Host bridge", "Folder, terminal, and screen bridge state.", `<div data-chat-target="hostBridgeStatusOutput" data-recyclr-sink="host-bridge-status-output">`+uiLoading("Loading host bridge…")+`</div>`) + `</div></div>`
}

func renderUIAdminAdvanced() string {
	return `<div data-admin-tab-panel="advanced" class="mx-auto max-w-5xl space-y-4"><section class="rounded-xl border border-rose-300/20 bg-rose-400/5 p-5">` +
		`<h3 class="text-sm font-semibold uppercase tracking-[.18em] text-rose-200">Destructive maintenance</h3>` +
		`<p class="mt-1 text-xs text-rose-200/70">Reset the database schema. This cannot be undone.</p>` +
		`<button data-action="chat#migrateFresh" type="button" class="mt-4 rounded-md border border-rose-300/30 bg-rose-400/10 px-4 py-2 text-sm font-semibold text-rose-100">Migrate fresh</button></section></div>`
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
