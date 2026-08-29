package api

import (
	"fmt"
	"html"
	"strings"
)

type researchStatusResponse struct {
	WebSearch webSearchRuntimeStatus `json:"web_search"`
	HTML      chatComponentHTML      `json:"html"`
}

func (s *Server) collectResearchStatus() (researchStatusResponse, error) {
	status, err := s.collectWebSearchStatus()
	if err != nil {
		return researchStatusResponse{}, err
	}
	return researchStatusResponse{WebSearch: status}, nil
}

func renderResearchStatusHTML(status researchStatusResponse) string {
	var output strings.Builder
	output.WriteString(`<div class="space-y-3" role="status" aria-live="polite">`)
	fmt.Fprintf(&output, `<div class="rounded-md border border-white/10 bg-white/[.03] p-3"><div class="text-[11px] font-semibold uppercase tracking-[.16em] text-zinc-500">Web specialist</div><div class="mt-1 text-sm font-semibold %s">%s</div></div>`, webSearchTone(status.WebSearch), html.EscapeString(webSearchState(status.WebSearch)))
	output.WriteString(`</div>`)
	return output.String()
}

func webSearchState(status webSearchRuntimeStatus) string {
	if !status.Enabled {
		return "not configured"
	}
	return "configured"
}

func webSearchTone(status webSearchRuntimeStatus) string {
	if status.Enabled {
		return "text-emerald-200"
	}
	return "text-zinc-300"
}
