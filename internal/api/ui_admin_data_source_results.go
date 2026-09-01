package api

import (
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleUIAdminDataSourceSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	source, ok := s.uiDataSourceFromRequest(w, r)
	if !ok {
		return
	}
	connection, err := source.DirectConnection()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	schema, err := datasource.InspectSchema(r.Context(), connection)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeUIOperationalComponent(w, "data-source-schema", renderUIDataSourceSchema(schema))
}

func (s *Server) uiDataSourceFromRequest(w http.ResponseWriter, r *http.Request) (source queue.DataSourceRecord, ok bool) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "data sources require PostgreSQL")
		return source, false
	}
	id, err := exactUIQuery(r, "id", 128)
	if err != nil || id == "" {
		if err == nil {
			err = errUIDataSourceIDRequired{}
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return source, false
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return source, false
	}
	return record, true
}

type errUIDataSourceIDRequired struct{}

func (errUIDataSourceIDRequired) Error() string { return "data source id is required" }

func renderUIDataSourceSchema(tables []omni.DBSchemaTable) string {
	if len(tables) == 0 {
		return `<p class="text-sm text-zinc-500">No tables found.</p>`
	}
	var body strings.Builder
	body.WriteString(`<div class="space-y-2">`)
	for _, table := range tables {
		fullName := table.Schema + "." + table.Name
		body.WriteString(`<details class="rounded-md border border-white/10 bg-zinc-900/40 px-3 py-2"><summary class="cursor-pointer font-mono text-xs text-cyan-200">` + uiEscape(fullName) + `</summary><ul class="mt-2 space-y-1 pl-2">`)
		for _, column := range table.Columns {
			nullability := " NOT NULL"
			if column.Nullable {
				nullability = ""
			}
			body.WriteString(`<li class="font-mono text-[11px] text-zinc-400">` + uiEscape(column.Name) + ` <span class="text-zinc-600">` + uiEscape(column.DataType+nullability) + `</span></li>`)
		}
		body.WriteString(`</ul></details>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}
