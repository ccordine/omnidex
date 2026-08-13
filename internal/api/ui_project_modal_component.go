package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) handleUIProjectModal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	kind, err := exactUIQuery(r, "kind", 16)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body string
	switch kind {
	case "create":
		body, err = s.renderUIProjectCreateModal(r)
	case "browse":
		body, err = s.renderUIProjectBrowseModal(r)
	default:
		err = fmt.Errorf("unsupported project modal kind %q", kind)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeUIOperationalComponent(w, "modal", body)
}

func (s *Server) renderUIProjectCreateModal(r *http.Request) (string, error) {
	selected, err := exactUIQuery(r, "selected", 4096)
	if err != nil {
		return "", err
	}
	name, err := exactUIQuery(r, "name", 256)
	if err != nil {
		return "", err
	}
	description, err := exactUIQuery(r, "description", 4096)
	if err != nil {
		return "", err
	}
	recipeID, err := exactUIQuery(r, "recipe_id", 256)
	if err != nil {
		return "", err
	}
	offset, err := exactChannelQueryInteger(r, "recipe_offset", 0, 0, 1<<30)
	if err != nil {
		return "", err
	}
	page, err := omni.LoadRecipePage(s.recipeRoot(), dataSourceUIPageSize, offset)
	if err != nil {
		return "", err
	}
	var options strings.Builder
	options.WriteString(`<option value="">No catalog recipe</option>`)
	for _, recipe := range page.Recipes {
		selectedOption := ""
		if recipe.ID == recipeID {
			selectedOption = " selected"
		}
		options.WriteString(`<option value="` + uiAttribute(recipe.ID) + `"` + selectedOption + `>` + uiEscape(recipe.ID) + `</option>`)
	}
	return `<div class="border-b border-white/10 p-4"><h2 class="text-xl font-semibold text-zinc-100">New project</h2></div><form data-action="submit->projects#submitCreate" class="omni-modal-body scrollbar space-y-4 p-4"><div data-projects-modal-feedback class="hidden rounded-md border px-3 py-2 text-sm" role="status"></div><label class="block text-xs text-zinc-500">Working directory<div class="mt-2 flex gap-2"><input data-projects-field="selectedPath" value="` + uiAttribute(selected) + `" readonly class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs" /><button data-action="projects#openBrowse" type="button" class="rounded-md border border-white/10 px-4 py-2 text-sm">Choose folder…</button></div></label>` + uiProjectModalInput("Name", "createName", name) + `<label class="block text-xs text-zinc-500">Catalog recipe<select data-projects-field="createRecipe" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2">` + options.String() + `</select></label>` + renderUIDataPagination("projects#loadCreateRecipePage", "recipe", page.Offset, len(page.Recipes), page.HasMore) + `<label class="block text-xs text-zinc-500">Description<textarea data-projects-field="createDesc" rows="3" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2">` + uiEscape(description) + `</textarea></label><div class="flex justify-end gap-2"><button type="button" data-action="projects#closeCreateModal" class="rounded-md border border-white/10 px-4 py-2">Cancel</button><button type="submit" data-projects-create-submit class="rounded-md bg-cyan-300 px-4 py-2 font-semibold text-zinc-950">Create project</button></div></form>`, nil
}

func uiProjectModalInput(label, field, value string) string {
	return `<label class="block text-xs text-zinc-500">` + uiEscape(label) + `<input data-projects-field="` + field + `" value="` + uiAttribute(value) + `" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm" /></label>`
}

func (s *Server) renderUIProjectBrowseModal(r *http.Request) (string, error) {
	path, err := exactUIQuery(r, "path", 4096)
	if err != nil {
		return "", err
	}
	selected, err := exactUIQuery(r, "selected", 4096)
	if err != nil {
		return "", err
	}
	mode, err := exactUIQuery(r, "mode", 16)
	if err != nil {
		return "", err
	}
	if mode != "create" && mode != "edit" {
		return "", fmt.Errorf("unsupported browse mode %q", mode)
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return "", err
	}
	result, err := s.uiBrowseDirectories(r.Context(), path, offset)
	if err != nil {
		return "", err
	}
	if selected == "" {
		selected = result.Path
	}
	var rows strings.Builder
	for _, entry := range result.Entries {
		if !entry.IsDir {
			continue
		}
		rows.WriteString(`<div class="flex gap-2"><button type="button" data-action="projects#selectBrowseDir" data-path="` + uiAttribute(entry.Path) + `" class="min-w-0 flex-1 rounded-md border border-white/10 px-3 py-2 text-left">📁 ` + uiEscape(entry.Name) + `</button><button type="button" data-action="projects#enterBrowseDir" data-path="` + uiAttribute(entry.Path) + `" class="rounded-md border border-white/10 px-3 py-2">Open</button></div>`)
	}
	pagination, err := uiBrowsePagination(result)
	if err != nil {
		return "", err
	}
	return `<div class="border-b border-white/10 p-4"><h2 class="text-xl font-semibold">Choose working directory</h2><p class="mt-2 font-mono text-xs text-zinc-500">` + uiEscape(result.Path) + `</p></div><div class="omni-modal-body scrollbar grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_300px]"><input type="hidden" data-browse-field="currentPath" value="` + uiAttribute(result.Path) + `" /><input type="hidden" data-browse-field="currentOffset" value="` + fmt.Sprint(result.Offset) + `" /><div data-projects-modal-feedback class="hidden lg:col-span-2 rounded-md border px-3 py-2 text-sm"></div><div class="space-y-2">` + uiBrowseParent(result.Parent) + rows.String() + pagination + `</div><aside class="space-y-4"><div class="rounded-lg border border-white/10 p-4"><h3 class="text-xs uppercase text-zinc-500">Selected</h3><p class="mt-3 break-all font-mono text-xs">` + uiEscape(selected) + `</p><button type="button" data-action="projects#confirmBrowse" data-path="` + uiAttribute(selected) + `" class="mt-4 w-full rounded-md bg-cyan-300 px-3 py-2 font-semibold text-zinc-950">Use this directory</button></div><div class="rounded-lg border border-white/10 p-4"><input data-browse-field="newFolderName" placeholder="my-project" class="w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2" /><button type="button" data-action="projects#createBrowseFolder" class="mt-3 w-full rounded-md border border-white/10 px-3 py-2">Create folder</button></div></aside></div>`, nil
}

func uiBrowseParent(parent string) string {
	if parent == "" {
		return ""
	}
	return `<button type="button" data-action="projects#enterBrowseDir" data-path="` + uiAttribute(parent) + `" class="w-full rounded-md border border-white/10 px-3 py-2 text-left">↑ Parent directory</button>`
}

func uiBrowsePagination(result hostbridge.BrowseResult) (string, error) {
	previous, hasPrevious, err := browsePreviousOffset(result)
	if err != nil {
		return "", err
	}
	if !hasPrevious && !result.HasMore {
		return "", nil
	}
	var body strings.Builder
	body.WriteString(`<nav class="flex items-center justify-between gap-2 pt-2" aria-label="Directory pages">`)
	if hasPrevious {
		body.WriteString(`<button type="button" data-action="projects#loadBrowsePage" data-page-offset="` + fmt.Sprint(previous) + `" class="rounded-md border border-white/10 px-3 py-2 text-xs">Previous</button>`)
	}
	if result.HasMore {
		if result.NextOffset <= result.Offset {
			return "", fmt.Errorf("directory page returned an invalid next offset")
		}
		body.WriteString(`<button type="button" data-action="projects#loadBrowsePage" data-page-offset="` + fmt.Sprint(result.NextOffset) + `" class="ml-auto rounded-md border border-white/10 px-3 py-2 text-xs">Next</button>`)
	}
	body.WriteString(`</nav>`)
	return body.String(), nil
}

func (s *Server) uiBrowseDirectories(ctx context.Context, path string, offset int) (hostbridge.BrowseResult, error) {
	opts := hostbridge.BrowseOptions{Limit: browseUIPageSize, Offset: offset, DirectoriesOnly: true}
	if client := s.hostBridgeClient(); client != nil {
		timeout, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		result, err := client.Browse(timeout, path, opts)
		if err != nil {
			return hostbridge.BrowseResult{}, err
		}
		if result == nil {
			return hostbridge.BrowseResult{}, fmt.Errorf("host bridge returned no directory result")
		}
		return *result, nil
	}
	opts, err := s.projectAuthorizedBrowseOptions(ctx, path, opts)
	if err != nil {
		return hostbridge.BrowseResult{}, err
	}
	result, err := hostbridge.ListDirectory(path, opts)
	if err != nil {
		return hostbridge.BrowseResult{}, err
	}
	if result == nil {
		return hostbridge.BrowseResult{}, fmt.Errorf("directory browser returned no result")
	}
	return *result, nil
}
