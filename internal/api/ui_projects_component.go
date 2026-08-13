package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const uiProjectsPageSize = 20

type uiProjectsComponent struct {
	HTML  chatComponentHTML `json:"html"`
	Count int               `json:"count"`
}

func (s *Server) handleUIProjectsComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require PostgreSQL")
		return
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projects, err := s.repo.ListProjects(r.Context(), uiProjectsPageSize+1, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasMore := len(projects) > uiProjectsPageSize
	if hasMore {
		projects = projects[:uiProjectsPageSize]
	}
	body, err := s.renderUIProjectList(r, projects, offset, hasMore)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, uiProjectsComponent{
		HTML:  chatComponentHTML{Bundle: renderRecyclrTemplateHTML("projects-list", body, "innerHTML")},
		Count: len(projects),
	})
}

func (s *Server) renderUIProjectList(r *http.Request, projects []model.Project, offset int, hasMore bool) (string, error) {
	if len(projects) == 0 && offset == 0 {
		return `<div class="rounded-xl border border-dashed border-white/10 p-8 text-sm text-zinc-500">No projects yet. Create one by choosing a working directory.</div>`, nil
	}
	var body strings.Builder
	for _, project := range projects {
		jobs, err := s.repo.CountProjectJobs(r.Context(), project.ID)
		if err != nil {
			return "", fmt.Errorf("count project jobs: %w", err)
		}
		cards, err := s.repo.CountProjectCards(r.Context(), project.ID)
		if err != nil {
			return "", fmt.Errorf("count project cards: %w", err)
		}
		automation, err := loadScrumAutoWorkConfig(project.Settings)
		if err != nil {
			return "", fmt.Errorf("project %d settings: %w", project.ID, err)
		}
		action, label, glyph := "startAutoWork", "Start auto-work", "▶"
		if automation.Enabled {
			action, label, glyph = "pauseAutoWork", "Pause auto-work", "⏸"
		}
		body.WriteString(`<article class="rounded-xl border border-white/10 bg-zinc-950/60 p-4"><div class="flex items-start gap-3"><button type="button" data-action="projects#openProject" data-project-id="` + uiInt(project.ID) + `" class="min-w-0 flex-1 text-left"><h3 class="truncate text-base font-semibold text-zinc-100">` + uiEscape(project.Name) + `</h3><p class="mt-1 truncate font-mono text-xs text-zinc-500">` + uiEscape(project.Location) + `</p></button><div class="shrink-0 text-right text-[11px] text-zinc-500"><div>` + uiEscape(project.UpdatedAt.UTC().Format(time.RFC3339)) + `</div><div class="mt-1">` + strconv.FormatInt(cards, 10) + ` cards · ` + strconv.FormatInt(jobs, 10) + ` jobs</div></div><button type="button" data-action="projects#` + action + `" data-project-id="` + uiInt(project.ID) + `" aria-label="` + uiAttribute(label+" for "+project.Name) + `" class="grid h-9 w-9 place-items-center rounded-md border border-cyan-300/30 bg-cyan-300/10">` + glyph + `</button></div></article>`)
	}
	if offset > 0 || hasMore {
		body.WriteString(`<nav class="flex items-center justify-between gap-2" aria-label="Project pages">`)
		if offset > 0 {
			body.WriteString(`<button type="button" data-action="projects#loadProjectPage" data-page-offset="` + strconv.Itoa(max(0, offset-uiProjectsPageSize)) + `" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Previous</button>`)
		}
		if hasMore {
			body.WriteString(`<button type="button" data-action="projects#loadProjectPage" data-page-offset="` + strconv.Itoa(offset+uiProjectsPageSize) + `" class="ml-auto rounded-md border border-white/10 px-3 py-1.5 text-xs">Next</button>`)
		}
		body.WriteString(`</nav>`)
	}
	return body.String(), nil
}
