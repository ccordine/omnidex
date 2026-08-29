package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

type uiDataSourcesComponent struct {
	HTML       chatComponentHTML `json:"html"`
	SelectedID string            `json:"selected_source_id,omitempty"`
	Offset     int               `json:"offset"`
	HasMore    bool              `json:"has_more"`
}

func (s *Server) handleUIAdminDataSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "data sources require PostgreSQL")
		return
	}
	selectedID, err := exactUIQuery(r, "selected_id", 128)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	editingID, err := exactUIQuery(r, "editing_id", 128)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request, err := dataSourcePageRequest(r, dataSourceUIPageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repo.ListDataSourcesPage(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	selected, selectedOK, err := s.uiDataSourceSelection(r.Context(), page.Items, selectedID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if selectedID != "" && !selectedOK {
		writeError(w, http.StatusConflict, "selected data source is no longer present")
		return
	}
	if !selectedOK && len(page.Items) > 0 {
		selected = page.Items[0]
		selectedID = selected.ID
		selectedOK = true
	}
	editing, editingOK, err := s.uiDataSourceSelection(r.Context(), page.Items, editingID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if editingID != "" && !editingOK {
		writeError(w, http.StatusConflict, "editing data source is no longer present")
		return
	}
	body := renderUIDataSourcesPanel(page.Items, selected, selectedOK, editing, editingOK, page.Offset, page.HasMore)
	writeChatComponentJSON(w, uiDataSourcesComponent{
		HTML:       chatComponentHTML{Bundle: renderRecyclrTemplateHTML("admin-data-sources", body, "innerHTML")},
		SelectedID: selectedID,
		Offset:     page.Offset,
		HasMore:    page.HasMore,
	})
}

func uiFindDataSource(items []queue.DataSourceRecord, id string) (queue.DataSourceRecord, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return queue.DataSourceRecord{}, false
}

func renderUIDataSourcesPanel(items []queue.DataSourceRecord, selected queue.DataSourceRecord, hasSelected bool, editing queue.DataSourceRecord, isEditing bool, offset int, hasMore bool) string {
	return uiAdminSection("Configured sources", "Read-only PostgreSQL connections for bounded SQL queries.", renderUIDataSourceList(items, selected.ID, offset, hasMore)) +
		uiAdminSection(map[bool]string{true: "Edit data source", false: "Add data source"}[isEditing], "Credentials remain server-authoritative and are never returned to the browser.", renderUIDataSourceForm(editing, isEditing)) +
		uiAdminSection("Query explorer", "Inspect schema and run validated read-only SQL.", renderUIDataSourceExplorer(selected, hasSelected))
}

func renderUIDataSourceList(items []queue.DataSourceRecord, selectedID string, offset int, hasMore bool) string {
	if len(items) == 0 {
		return `<p class="text-sm text-zinc-500">No data sources on this page.</p>` +
			renderUIDataPagination("admin-data-sources#loadDataSourcePage", "source", offset, 0, hasMore)
	}
	var body strings.Builder
	body.WriteString(`<div class="space-y-2">`)
	for _, source := range items {
		border := "border-white/10 bg-zinc-900/50"
		if source.ID == selectedID {
			border = "border-cyan-300/40 bg-cyan-300/5"
		}
		status := "Untested"
		statusClass := "text-zinc-400"
		if source.LastTestStatus == "ok" {
			status, statusClass = "Connected", "text-emerald-200"
		} else if source.LastTestStatus == "failed" {
			status, statusClass = "Failed", "text-rose-200"
		}
		connection := "Host-authorized · " + source.AuthorityURL
		if source.ExecutionMode == "direct" && source.UseDSN {
			connection = "Direct PostgreSQL · DSN"
		} else if source.ExecutionMode == "direct" {
			connection = fmt.Sprintf("%s:%d/%s", source.Host, source.Port, source.DatabaseName)
		}
		body.WriteString(`<article class="rounded-md border ` + border + ` px-3 py-3"><div class="flex flex-wrap items-start justify-between gap-3"><button type="button" data-action="admin-data-sources#selectDataSource" data-source-id="` + uiAttribute(source.ID) + `" class="min-w-0 text-left"><div class="font-medium text-zinc-100">` + uiEscape(source.Name) + `</div><div class="mt-1 font-mono text-[11px] text-zinc-500">` + uiEscape(connection) + `</div></button><div class="flex flex-wrap items-center gap-2"><span class="text-[10px] uppercase ` + statusClass + `">` + status + `</span>`)
		actions := []struct{ method, label, class string }{{"editDataSource", "Edit", "text-zinc-300"}, {"deleteDataSource", "Remove", "text-rose-200"}}
		if source.ExecutionMode == "direct" {
			actions = append([]struct{ method, label, class string }{{"testDataSource", "Test", "text-zinc-300"}}, actions...)
		}
		for _, action := range actions {
			body.WriteString(`<button type="button" data-action="admin-data-sources#` + action.method + `" data-source-id="` + uiAttribute(source.ID) + `" class="rounded-md border border-white/10 px-2 py-1 text-xs ` + action.class + `">` + action.label + `</button>`)
		}
		body.WriteString(`</div></div></article>`)
	}
	body.WriteString(`</div>`)
	body.WriteString(renderUIDataPagination("admin-data-sources#loadDataSourcePage", "source", offset, len(items), hasMore))
	return body.String()
}

func renderUIDataSourceForm(source queue.DataSourceRecord, editing bool) string {
	button := "Add data source"
	if editing {
		button = "Save changes"
	}
	checked := ""
	if source.UseDSN {
		checked = " checked"
	}
	mode := string(source.ExecutionMode)
	if mode == "" {
		mode = "direct"
	}
	directClass, delegatedClass := "", " hidden"
	if mode == "delegated" {
		directClass, delegatedClass = " hidden", ""
	}
	port := source.Port
	if port == 0 {
		port = 5432
	}
	form := `<form data-action="submit->admin-data-sources#saveDataSource" data-ds-source-form class="grid gap-3"><input type="hidden" data-ds-field="id" value="` + uiAttribute(source.ID) + `" />` +
		uiDSInput("Name", "name", source.Name, "text") +
		uiDSSelect("Database type", "driver", "postgres", []string{"postgres"}) +
		uiDSSelectWithAction("Execution", "execution_mode", mode, []string{"direct", "delegated"}, "change->admin-data-sources#toggleDataSourceExecutionPanel") +
		`<div data-ds-direct-fields class="grid gap-3` + directClass + `"><label class="flex items-center gap-2 text-sm text-zinc-300"><input type="checkbox" data-ds-field="use_dsn" data-action="change->admin-data-sources#toggleDataSourceDSNPanel"` + checked + ` />Use connection string (DSN)</label>` +
		`<div data-ds-connection-fields class="grid gap-3 md:grid-cols-2">` + uiDSInput("DSN", "dsn", "", "password") + uiDSInput("Host", "host", source.Host, "text") + uiDSInput("Port", "port", strconv.Itoa(port), "number") + uiDSInput("Database", "database_name", source.DatabaseName, "text") + uiDSInput("Username", "username", source.Username, "text") + uiDSInput("Password", "password", "", "password") + uiDSSelect("SSL mode", "ssl_mode", source.SSLMode, []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}) + `</div></div>` +
		`<div data-ds-delegated-fields class="grid gap-3 md:grid-cols-2` + delegatedClass + `">` + uiDSInput("Host authority URL", "authority_url", source.AuthorityURL, "url") + uiDSInput("Credential environment variable", "credential_env", source.CredentialEnv, "text") + `</div>` +
		`<div class="flex gap-2"><button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">` + button + `</button>`
	if editing {
		form += `<button type="button" data-action="admin-data-sources#cancelEditDataSource" class="rounded-md border border-white/10 px-4 py-2 text-sm">Cancel</button>`
	}
	return form + `</div></form>`
}

func uiDSInput(label, field, value, kind string) string {
	return `<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(label) + `</span><input data-ds-field="` + uiAttribute(field) + `" type="` + kind + `" value="` + uiAttribute(value) + `" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm" /></label>`
}

func uiDSSelect(label, field, value string, options []string) string {
	return uiDSSelectWithAction(label, field, value, options, "")
}

func uiDSSelectWithAction(label, field, value string, options []string, action string) string {
	var body strings.Builder
	actionAttribute := ""
	if action != "" {
		actionAttribute = ` data-action="` + uiAttribute(action) + `"`
	}
	body.WriteString(`<label class="block"><span class="text-xs text-zinc-500">` + uiEscape(label) + `</span><select data-ds-field="` + uiAttribute(field) + `"` + actionAttribute + ` class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm">`)
	for _, option := range options {
		selected := ""
		if option == value || (value == "" && option == "prefer") {
			selected = " selected"
		}
		body.WriteString(`<option value="` + uiAttribute(option) + `"` + selected + `>` + uiEscape(option) + `</option>`)
	}
	body.WriteString(`</select></label>`)
	return body.String()
}

func renderUIDataSourceExplorer(source queue.DataSourceRecord, exists bool) string {
	if !exists {
		return `<p class="text-sm text-zinc-500">Add and select a data source to explore it.</p>`
	}
	if source.ExecutionMode == "delegated" {
		return `<p class="text-sm text-zinc-400">Delegated sources expose schema and bounded evidence only inside a turn carrying a current host-issued authority id.</p>`
	}
	return `<div class="space-y-4"><div class="flex flex-wrap items-center justify-between gap-2"><div><h4 class="text-sm font-semibold text-zinc-200">` + uiEscape(source.Name) + `</h4><p class="text-xs text-zinc-500">Read-only · maximum 500 rows</p></div><button type="button" data-action="admin-data-sources#loadDataSourceSchema" data-source-id="` + uiAttribute(source.ID) + `" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Load schema</button></div>` +
		`<div data-recyclr-sink="data-source-schema" class="scrollbar max-h-[280px] overflow-y-auto"><p class="text-sm text-zinc-500">Load schema to browse tables.</p></div>` +
		`<div><textarea data-ds-field="sql" rows="5" placeholder="SELECT * FROM my_table LIMIT 20" class="w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs"></textarea><button type="button" data-action="admin-data-sources#runDataSourceQuery" data-source-id="` + uiAttribute(source.ID) + `" class="mt-2 rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950">Run query</button></div>` +
		`<div data-recyclr-sink="data-source-query-result"><p class="text-sm text-zinc-500">Run a query to see results.</p></div></div>`
}
