package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) renderUIProjectTab(r *http.Request, project model.Project, tab string) (string, error) {
	switch tab {
	case "scrum":
		return renderUIProjectScrum(project.Location), nil
	case "terminal":
		return renderUIProjectTerminal(project.Location), nil
	case "screen":
		return renderUIProjectScreen(project.Location), nil
	case "settings":
		return s.renderUIProjectSettings(r, project)
	case "map":
		payload, err := s.loadProjectCodebaseMapPayload(r.Context(), project.Location)
		if err != nil {
			return "", err
		}
		return renderUIProjectMap(project.ID, payload), nil
	case "git":
		payload, err := s.loadProjectGitStatus(r.Context(), project, project.Location)
		if err != nil {
			return "", err
		}
		return renderUIProjectGit(project.ID, payload), nil
	case "recipe":
		offset, err := exactChannelQueryInteger(r, "recipe_offset", 0, 0, 1<<30)
		if err != nil {
			return "", err
		}
		return s.renderUIProjectRecipe(project, offset)
	default:
		return "", fmt.Errorf("unsupported project tab %q", tab)
	}
}

func renderUIProjectScrum(location string) string {
	return `<div data-project-tab-panel="scrum" class="flex min-h-0 flex-col gap-3"><div class="grid shrink-0 grid-cols-1 items-center gap-3 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]"><p class="truncate font-mono text-[11px] text-zinc-500">` + uiEscape(location) + `</p><div data-scrum-target="focus" data-recyclr-sink="scrum-focus" class="flex justify-center"></div><div class="flex items-center justify-end gap-2"><span data-scrum-target="status" class="text-xs text-zinc-500"></span><button type="button" data-action="scrum#openCreateCardModal" class="rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950">+ Card</button><button type="button" data-action="scrum#refresh" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300">Refresh</button></div></div><div data-scrum-target="flowSummary" data-recyclr-sink="scrum-flow-summary" class="hidden shrink-0"></div><div data-scrum-target="columns" data-recyclr-sink="scrum-columns" class="shrink-0"></div><div class="relative scrollbar flex min-h-0 flex-1 flex-col overflow-x-auto overflow-y-hidden" data-scrum-board-scroll><div data-scrum-target="boardOverlay" class="pointer-events-none absolute inset-0 z-10 hidden items-center justify-center bg-zinc-950/80" role="status"><span data-scrum-target="boardOverlayMessage">Working…</span></div><div data-scrum-target="board" data-recyclr-sink="scrum-board" class="scrum-kanban min-h-0"></div></div><div data-recyclr-sink="scrum-pagination" class="shrink-0"></div></div>`
}

func renderUIProjectTerminal(location string) string {
	return `<div data-project-tab-panel="terminal" class="flex min-h-0 flex-col gap-3"><div class="flex items-center justify-between gap-3"><p class="truncate font-mono text-[11px] text-zinc-500">` + uiEscape(location) + `</p><div class="flex gap-2"><span data-terminal-target="status" class="text-xs text-zinc-500">Idle</span><button type="button" data-action="terminal#reconnect" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Reconnect</button><button type="button" data-action="terminal#toggleFullscreen" data-terminal-target="fullscreenButton" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Fullscreen</button></div></div><div data-terminal-target="frame" class="relative min-h-0 flex-1 overflow-hidden rounded-xl border border-white/10 bg-zinc-950"><div data-terminal-target="mount" class="h-full min-h-[420px] w-full p-1"></div></div></div>`
}

func renderUIProjectScreen(location string) string {
	return `<div data-project-tab-panel="screen" class="flex min-h-0 flex-col gap-3"><div class="flex items-center justify-between gap-3"><p class="truncate font-mono text-[11px] text-zinc-500">` + uiEscape(location) + `</p><div class="flex gap-2"><span data-screen-target="status" class="text-xs text-zinc-500">Idle</span><button type="button" data-action="screen#reconnect" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Reconnect</button><button type="button" data-action="screen#toggleFullscreen" data-screen-target="fullscreenButton" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Fullscreen</button></div></div><div class="flex flex-wrap items-end gap-3 rounded-xl border border-white/10 bg-zinc-950/60 p-4"><label class="min-w-[180px] flex-1 text-xs text-zinc-500">Monitor<select data-screen-target="monitorSelect" data-recyclr-sink="screen-monitor-options" data-action="change->screen#changeMonitor" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2"><option value="">Loading…</option></select></label>` + uiScreenQualityControls() + `<div data-recyclr-sink="screen-monitor-pagination" class="w-full"></div></div><div data-screen-target="frame" class="relative min-h-0 flex-1 overflow-hidden rounded-xl border border-white/10 bg-black"><img data-screen-target="stream" alt="Host monitor stream" class="h-full min-h-[420px] w-full object-contain" /><div data-screen-target="placeholder" class="absolute inset-0 grid place-items-center text-sm text-zinc-500">Open this tab to start the monitor stream.</div></div></div>`
}

func uiScreenQualityControls() string {
	return `<label class="w-28 text-xs text-zinc-500">FPS<select data-screen-target="fpsSelect" data-action="change->screen#changeQuality" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2"><option value="8">8</option><option value="12" selected>12</option><option value="20">20</option></select></label><label class="w-32 text-xs text-zinc-500">Scale<select data-screen-target="scaleSelect" data-action="change->screen#changeQuality" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2"><option value="100">100%</option><option value="75" selected>75%</option><option value="50">50%</option></select></label><label class="min-w-[220px] flex-1 text-xs text-zinc-500">LAN stream URL<input data-screen-target="streamUrl" readonly class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-[11px]" /></label>`
}

func uiMapStringList(values map[string]any, key string) []string {
	raw, _ := values[key].([]string)
	return raw
}

func uiProjectField(name, value string) string {
	return `<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(strings.ReplaceAll(name, "_", " ")) + `</span><input data-projects-field="` + uiAttribute(name) + `" value="` + uiAttribute(value) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm" /></label>`
}
