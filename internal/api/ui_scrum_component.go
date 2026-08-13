package api

import (
	"fmt"
	"net/http"
	"strings"
)

func (s *Server) handleUIScrumCreateCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.repo.GetProject(r.Context(), projectID); err != nil {
		writeProjectError(w, err)
		return
	}
	column, err := exactUIQuery(r, "column", 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	column = normalizeScrumColumn(column)
	if column == "" {
		writeError(w, http.StatusBadRequest, "column must be a registered Scrum column")
		return
	}
	writeUIOperationalComponent(w, "modal", renderUIScrumCreateCard(column))
}

func renderUIScrumCreateCard(column string) string {
	var options strings.Builder
	for _, candidate := range scrumColumns {
		selected := ""
		if candidate == column {
			selected = " selected"
		}
		options.WriteString(`<option value="` + uiAttribute(candidate) + `"` + selected + `>` + uiEscape(scrumColumnLabel(candidate)) + `</option>`)
	}
	return fmt.Sprintf(`<div class="border-b border-white/10 p-4 md:p-5"><div class="flex flex-wrap items-start justify-between gap-3"><div><p class="text-xs uppercase tracking-[.20em] text-cyan-200/80">Scrum</p><h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">New card</h2></div><button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Cancel</button></div></div><form data-action="submit->scrum#createCard" class="omni-modal-body scrollbar space-y-4 p-4 md:p-5"><label class="block"><span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Title</span><input data-scrum-field="newTitle" type="text" required autofocus placeholder="What needs doing?" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" /></label><label class="block"><span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Description</span><textarea data-scrum-field="newDesc" rows="4" placeholder="Optional context for Omnidex" class="scrollbar mt-2 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-100 outline-none focus:border-cyan-300/40"></textarea></label><label class="block"><span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Column</span><select data-scrum-field="newColumn" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">%s</select></label><div class="flex justify-end gap-2 border-t border-white/10 pt-4"><button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300">Cancel</button><button type="submit" data-scrum-submit="create" class="inline-flex items-center justify-center gap-2 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60">Create card</button></div></form>`, options.String())
}
