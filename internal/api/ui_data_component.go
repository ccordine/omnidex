package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type uiDataComponent struct {
	HTML              chatComponentHTML `json:"html"`
	SelectedSourceID  string            `json:"selected_source_id,omitempty"`
	SelectedChannelID string            `json:"selected_channel_id,omitempty"`
	SourceOffset      int               `json:"source_offset"`
	SourceHasMore     bool              `json:"source_has_more"`
	ChannelOffset     int               `json:"channel_offset"`
	ChannelHasMore    bool              `json:"channel_has_more"`
}

func (s *Server) handleUIDataComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "data panel requires PostgreSQL")
		return
	}
	sourceID, err := exactUIQuery(r, "source_id", 128)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelID, err := exactUIQuery(r, "channel_id", 128)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourceRequest, err := fixedDataSourcePageRequest(r, "source_offset", dataSourceUIPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sourcePage, err := s.repo.ListDataSourcesPage(r.Context(), sourceRequest)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	selected, selectedOK, err := s.uiDataSourceSelection(r.Context(), sourcePage.Items, sourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sourceID != "" && !selectedOK {
		writeError(w, http.StatusConflict, "selected data source is no longer present")
		return
	}
	if !selectedOK && len(sourcePage.Items) > 0 {
		selected, sourceID, selectedOK = sourcePage.Items[0], sourcePage.Items[0].ID, true
	}
	channelRequest, err := fixedDataSourcePageRequest(r, "channel_offset", dataSourceUIPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelPage := queue.DataSourceChannelPage{Items: []model.DataSourceChannel{}, Offset: channelRequest.Offset}
	if selectedOK {
		channelPage, err = s.repo.ListDataSourceChannelsPage(r.Context(), sourceID, channelRequest)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	selectedChannel, channelOK, err := s.uiDataChannelSelection(r.Context(), sourceID, channelPage.Items, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if channelID != "" && !channelOK {
		writeError(w, http.StatusConflict, "selected data channel is no longer present")
		return
	}
	if !channelOK && len(channelPage.Items) > 0 {
		selectedChannel, channelID, channelOK = channelPage.Items[0], channelPage.Items[0].ID, true
	}
	body := renderUIDataPanel(sourcePage, selected, selectedOK, channelPage, selectedChannel)
	writeChatComponentJSON(w, uiDataComponent{
		HTML:             chatComponentHTML{Bundle: renderRecyclrTemplateHTML("data-panel", body, "innerHTML")},
		SelectedSourceID: sourceID, SelectedChannelID: channelID,
		SourceOffset: sourcePage.Offset, SourceHasMore: sourcePage.HasMore,
		ChannelOffset: channelPage.Offset, ChannelHasMore: channelPage.HasMore,
	})
}

func findUIDataChannel(items []model.DataSourceChannel, id string) (model.DataSourceChannel, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return model.DataSourceChannel{}, false
}

func renderUIDataPanel(sources queue.DataSourcePage, selected queue.DataSourceRecord, hasSource bool, channels queue.DataSourceChannelPage, channel model.DataSourceChannel) string {
	return `<div class="grid h-full min-h-0 gap-0 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)]">` +
		`<aside class="border-b border-white/10 lg:border-b-0 lg:border-r"><div class="border-b border-white/10 px-3 py-2 text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Databases</div><div class="scrollbar max-h-[220px] overflow-y-auto">` + renderUIDataSourceButtons(sources.Items, selected.ID) + `</div>` + renderUIDataPagination("data#loadDataPage", "source", sources.Offset, len(sources.Items), sources.HasMore) + `</aside>` +
		`<aside class="border-b border-white/10 lg:border-b-0"><div class="border-b border-white/10 px-3 py-2 text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Channels</div><div class="scrollbar p-3">` + renderUIDataChannelButtons(channels.Items, channel.ID, hasSource) + renderUIDataPagination("data#loadDataPage", "channel", channels.Offset, len(channels.Items), channels.HasMore) + `</div></aside></div>`
}

func renderUIDataSourceButtons(items []queue.DataSourceRecord, selectedID string) string {
	if len(items) == 0 {
		return `<p class="px-3 py-4 text-xs text-zinc-500">No databases configured.</p>`
	}
	var body strings.Builder
	for _, item := range items {
		active := "hover:bg-white/5"
		if item.ID == selectedID {
			active = "bg-cyan-300/10"
		}
		body.WriteString(`<button type="button" data-action="data#selectSource" data-source-id="` + uiAttribute(item.ID) + `" class="block w-full border-b border-white/5 px-3 py-2 text-left ` + active + `"><div class="text-sm font-medium text-zinc-100">` + uiEscape(item.Name) + `</div><div class="mt-0.5 font-mono text-[10px] text-zinc-500">` + uiEscape(item.Host+"/"+item.DatabaseName) + `</div></button>`)
	}
	return body.String()
}

func renderUIDataChannelButtons(items []model.DataSourceChannel, selectedID string, hasSource bool) string {
	if !hasSource {
		return `<p class="text-xs text-zinc-500">Select a database.</p>`
	}
	var body strings.Builder
	body.WriteString(`<div class="space-y-2">`)
	for _, item := range items {
		style := "border-white/10"
		if item.ID == selectedID {
			style = "border-cyan-300/40 bg-cyan-300/10"
		}
		body.WriteString(`<button type="button" data-action="data#selectChannel" data-channel-id="` + uiAttribute(item.ID) + `" class="block w-full rounded-md border ` + style + ` px-2 py-2 text-left"><div class="truncate text-xs font-medium text-zinc-100">` + uiEscape(item.Name) + `</div><div class="mt-0.5 text-[10px] text-zinc-600">` + uiEscape(item.UpdatedAt.UTC().Format(time.RFC3339)) + `</div></button>`)
	}
	if len(items) == 0 {
		body.WriteString(`<p class="text-xs text-zinc-500">No channels yet.</p>`)
	}
	body.WriteString(`</div><button type="button" data-action="data#createChannel" class="mt-3 w-full rounded-md border border-dashed border-cyan-300/30 px-2 py-2 text-xs font-semibold text-cyan-100">+ New channel</button>`)
	return body.String()
}
