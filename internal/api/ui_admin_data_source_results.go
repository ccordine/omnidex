package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
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

func (s *Server) handleUIAdminDataSourceQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	source, ok := s.uiDataSourceFromRequest(w, r)
	if !ok {
		return
	}
	var request struct {
		SQL string `json:"sql"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid query request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "query request contains trailing data")
		return
	}
	connection, err := source.DirectConnection()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := datasource.RunSQL(r.Context(), connection, request.SQL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeUIOperationalComponent(w, "data-source-query-result", renderUIDataSourceQueryResult(result))
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
		body.WriteString(`</ul><button type="button" data-action="admin-data-sources#insertSchemaQuery" data-table-name="` + uiAttribute(fullName) + `" class="mt-2 rounded border border-white/10 px-2 py-0.5 text-[11px] text-zinc-400">Insert SELECT *</button></details>`)
	}
	body.WriteString(`</div>`)
	return body.String()
}

func renderUIDataSourceQueryResult(result datasource.QueryResult) string {
	if len(result.Columns) == 0 {
		return `<p class="text-sm text-zinc-500">Query returned no columns.</p>`
	}
	var body strings.Builder
	body.WriteString(`<p class="font-mono text-[11px] text-zinc-500">` + uiEscape(result.SQL) + `</p><p class="mt-1 text-[11px] text-zinc-600">` + strconv.Itoa(result.Count) + ` rows</p><div class="scrollbar mt-3 max-h-[360px] overflow-auto rounded-lg border border-white/10"><table class="min-w-full border-collapse"><thead class="sticky top-0 bg-zinc-950/95"><tr>`)
	for _, column := range result.Columns {
		body.WriteString(`<th class="px-3 py-2 text-left font-mono text-[11px] uppercase text-zinc-500">` + uiEscape(column) + `</th>`)
	}
	body.WriteString(`</tr></thead><tbody>`)
	for _, row := range result.Rows {
		body.WriteString(`<tr class="border-t border-white/5">`)
		for _, column := range result.Columns {
			body.WriteString(`<td class="px-3 py-2 font-mono text-xs text-zinc-300">` + uiEscape(stringifyUIValue(row[column])) + `</td>`)
		}
		body.WriteString(`</tr>`)
	}
	body.WriteString(`</tbody></table></div>`)
	return body.String()
}

func stringifyUIValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "[unrepresentable value]"
	}
	return string(raw)
}
