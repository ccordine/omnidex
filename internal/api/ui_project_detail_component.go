package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

var uiProjectTabs = map[string]struct{}{"scrum": {}, "terminal": {}, "screen": {}, "settings": {}, "map": {}, "git": {}, "recipe": {}}

type uiProjectDetailComponent struct {
	HTML            chatComponentHTML `json:"html"`
	ProjectID       int64             `json:"project_id"`
	ProjectName     string            `json:"project_name"`
	ProjectLocation string            `json:"project_location"`
	Tab             string            `json:"tab"`
}

func (s *Server) handleUIProjectComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require PostgreSQL")
		return
	}
	idText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/ui/projects/"), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid project id")
		return
	}
	tab, err := exactUIQuery(r, "tab", 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if tab == "" {
		tab = "scrum"
	}
	if _, ok := uiProjectTabs[tab]; !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported project tab %q", tab))
		return
	}
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	panel, err := s.renderUIProjectTab(r, project, tab)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	body := renderUIProjectDetailShell(project, tab, panel)
	writeChatComponentJSON(w, uiProjectDetailComponent{
		HTML:      chatComponentHTML{Bundle: renderRecyclrTemplateHTML("project-detail", body, "innerHTML")},
		ProjectID: id, ProjectName: project.Name, ProjectLocation: project.Location, Tab: tab,
	})
}

func renderUIProjectDetailShell(project model.Project, activeTab, panel string) string {
	return `<div data-controller="terminal screen" class="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden"><div class="flex shrink-0 flex-wrap items-center justify-between gap-3"><div class="min-w-0"><button type="button" data-action="projects#backToList" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">← All projects</button><h3 class="mt-3 truncate text-2xl font-semibold tracking-tight text-zinc-100">` + uiEscape(project.Name) + `</h3><p class="mt-1 truncate font-mono text-xs text-zinc-500">` + uiEscape(project.Location) + `</p></div></div>` + renderUIProjectTabs(activeTab) + `<div class="project-tab-stack">` + panel + `</div></div>`
}

func renderUIProjectTabs(active string) string {
	items := []struct{ id, label string }{{"scrum", "Scrum"}, {"terminal", "Terminal"}, {"screen", "Screen"}, {"settings", "Settings"}, {"map", "Codebase map"}, {"git", "Git"}, {"recipe", "Recipe"}}
	var body strings.Builder
	body.WriteString(`<nav class="flex shrink-0 flex-wrap gap-2" aria-label="Project sections">`)
	for _, item := range items {
		classes := "border-white/10 text-zinc-400"
		if item.id == active {
			classes = "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
		}
		body.WriteString(`<button type="button" data-action="projects#showTab" data-project-tab="` + item.id + `" class="rounded-md border px-3 py-2 text-sm font-medium ` + classes + `">` + item.label + `</button>`)
	}
	body.WriteString(`</nav>`)
	return body.String()
}
