package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

type uiScreenComponent struct {
	HTML        chatComponentHTML `json:"html"`
	MonitorID   string            `json:"monitor_id,omitempty"`
	Offset      int               `json:"offset"`
	HasPrevious bool              `json:"has_previous"`
	HasMore     bool              `json:"has_more"`
}

func (s *Server) handleUIScreenMonitors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.hostBridgeClient() == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: run `omni host serve`")
		return
	}
	if _, err := s.resolveProjectID(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pageRequest, err := screenMonitorPageRequest(r, screenMonitorUIPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := s.hostBridgeClient().ScreenMonitors(ctx, pageRequest)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	body, selected, err := renderUIScreenMonitorOptions(page.Monitors)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	pagination, err := renderUIScreenMonitorPagination(page)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeChatComponentJSON(w, uiScreenComponent{
		HTML: chatComponentHTML{Bundle: renderRecyclrTemplateHTML("screen-monitor-options", body, "innerHTML") +
			renderRecyclrTemplateHTML("screen-monitor-pagination", pagination, "innerHTML")},
		MonitorID: selected, Offset: page.Offset, HasPrevious: page.HasPrevious, HasMore: page.HasMore,
	})
}

func renderUIScreenMonitorOptions(items []hostbridge.ScreenMonitor) (string, string, error) {
	if len(items) == 0 {
		return `<option value="">No monitors found</option>`, "", nil
	}
	for index, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
			return "", "", fmt.Errorf("monitor %d lacks id or name", index)
		}
	}
	selected := items[0].ID
	for _, item := range items {
		if item.Primary {
			selected = item.ID
			break
		}
	}
	var body strings.Builder
	for _, item := range items {
		selection := ""
		if item.ID == selected {
			selection = " selected"
		}
		label := item.Name
		if item.Width > 0 && item.Height > 0 {
			label += fmt.Sprintf(" (%d×%d)", item.Width, item.Height)
		}
		if item.Primary {
			label += " · primary"
		}
		body.WriteString(`<option value="` + uiAttribute(item.ID) + `"` + selection + `>` + uiEscape(label) + `</option>`)
	}
	return body.String(), selected, nil
}

func renderUIScreenMonitorPagination(page hostbridge.ScreenMonitorPage) (string, error) {
	if !page.HasPrevious && !page.HasMore {
		return "", nil
	}
	var body strings.Builder
	body.WriteString(`<nav class="flex items-center justify-between gap-2" aria-label="Monitor pages">`)
	if page.HasPrevious {
		if page.PreviousOffset < 0 || page.PreviousOffset >= page.Offset {
			return "", fmt.Errorf("monitor page returned an invalid previous offset")
		}
		body.WriteString(`<button type="button" data-action="screen#loadMonitorPage" data-page-offset="` + fmt.Sprint(page.PreviousOffset) + `" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Previous monitors</button>`)
	}
	if page.HasMore {
		if page.NextOffset <= page.Offset {
			return "", fmt.Errorf("monitor page returned an invalid next offset")
		}
		body.WriteString(`<button type="button" data-action="screen#loadMonitorPage" data-page-offset="` + fmt.Sprint(page.NextOffset) + `" class="ml-auto rounded-md border border-white/10 px-3 py-1.5 text-xs">Next monitors</button>`)
	}
	body.WriteString(`</nav>`)
	return body.String(), nil
}
