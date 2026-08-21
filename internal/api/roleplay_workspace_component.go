package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	roleplayWorldListTarget       = "roleplay-world-list"
	roleplayWorldPaginationTarget = "roleplay-world-pagination"
	roleplayLibraryListTarget     = "roleplay-library-list"
	roleplayLibraryPageTarget     = "roleplay-library-pagination"
)

func renderRoleplayWorldPage(page roleplay.WorldPage, limit int) (chatComponentPage, error) {
	if limit < 1 || len(page.Items) > limit {
		return chatComponentPage{}, fmt.Errorf("roleplay world page exceeds its requested bound")
	}
	var body strings.Builder
	if page.Offset == 0 && len(page.Items) == 0 {
		body.WriteString(chatEmptyState("No worlds yet. Create one to begin a story."))
	}
	for _, item := range page.Items {
		if item.World.Authority != roleplay.AuthorityFictionalCanon || item.CharacterCount < 1 {
			return chatComponentPage{}, fmt.Errorf("roleplay world page contains invalid authority")
		}
		body.WriteString(`<button type="button" data-action="chat#selectRoleplayWorld" data-channel-id="` +
			html.EscapeString(item.World.ChannelID) + `" class="roleplay-world-card group w-full rounded-xl border border-white/10 bg-white/[.025] p-3 text-left transition hover:border-violet-300/40 hover:bg-violet-300/[.07] focus:outline-none focus:ring-2 focus:ring-violet-300/30">`)
		body.WriteString(`<span class="block truncate text-sm font-semibold text-zinc-100">` +
			html.EscapeString(item.World.Name) + `</span>`)
		if item.SceneTitle == "" {
			body.WriteString(`<span class="mt-1 block text-xs text-amber-200/80">Scene setup needed</span>`)
		} else {
			body.WriteString(`<span class="mt-1 block truncate text-xs text-zinc-400">` +
				html.EscapeString(item.SceneTitle) + `</span>`)
		}
		body.WriteString(`<span class="mt-2 block text-[10px] uppercase tracking-[.14em] text-zinc-600">` +
			strconv.FormatInt(item.CharacterCount, 10) + ` characters</span></button>`)
	}
	next := roleplayNextOffset(page.Offset, len(page.Items), page.HasMore)
	location := "innerHTML"
	if page.Offset > 0 {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(roleplayWorldListTarget, body.String(), location) +
		renderRecyclrTemplateHTML(roleplayWorldPaginationTarget,
			roleplayWorkspacePagination("loadMoreRoleplayWorlds", "worlds", next), "innerHTML")
	return chatComponentPage{NextOffset: next, HasMore: next != nil, HTML: chatComponentHTML{Bundle: bundle}}, nil
}

func renderRoleplayLibraryPage(page roleplay.LibraryCharacterPage, limit int) (chatComponentPage, error) {
	if limit < 1 || len(page.Items) > limit {
		return chatComponentPage{}, fmt.Errorf("roleplay character library page exceeds its requested bound")
	}
	if page.SelectedWorldID != "" && !roleplayWorldIdentityPattern.MatchString(page.SelectedWorldID) {
		return chatComponentPage{}, fmt.Errorf("roleplay character library has invalid selected world authority")
	}
	var body strings.Builder
	if page.Offset == 0 && len(page.Items) == 0 {
		body.WriteString(chatEmptyState("The character library is empty."))
	}
	for _, character := range page.Items {
		if character.Authority != roleplay.AuthorityCharacterIdentity || character.MemoryCount < 0 || character.PlacementCount < 0 {
			return chatComponentPage{}, fmt.Errorf("roleplay character library contains invalid authority")
		}
		if character.PlacedInSelectedWorld && page.SelectedWorldID == "" {
			return chatComponentPage{}, fmt.Errorf("roleplay character placement lacks selected world authority")
		}
		body.WriteString(`<article class="rounded-xl border border-white/10 bg-white/[.025] p-3">`)
		body.WriteString(`<div class="flex items-start justify-between gap-3"><div class="min-w-0">`)
		body.WriteString(`<h4 class="truncate text-sm font-semibold text-zinc-100">` + html.EscapeString(character.Name) + `</h4>`)
		if character.Profile == nil {
			body.WriteString(`<p class="mt-1 text-xs text-amber-200/80">Character sheet needed</p>`)
		} else {
			body.WriteString(`<p class="mt-1 line-clamp-2 text-xs leading-5 text-zinc-400">` +
				html.EscapeString(character.Profile.Summary) + `</p>`)
		}
		body.WriteString(`</div>` + roleplayLibraryPlacementButton(page.SelectedWorldID, character) + `</div>`)
		body.WriteString(`<p class="mt-3 text-[10px] uppercase tracking-[.12em] text-zinc-600">` +
			strconv.FormatInt(character.MemoryCount, 10) + ` memories · ` +
			strconv.FormatInt(character.PlacementCount, 10) + ` worlds</p></article>`)
	}
	next := roleplayNextOffset(page.Offset, len(page.Items), page.HasMore)
	location := "innerHTML"
	if page.Offset > 0 {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(roleplayLibraryListTarget, body.String(), location) +
		renderRecyclrTemplateHTML(roleplayLibraryPageTarget,
			roleplayWorkspacePagination("loadMoreRoleplayCharacters", "characters", next), "innerHTML")
	return chatComponentPage{NextOffset: next, HasMore: next != nil, HTML: chatComponentHTML{Bundle: bundle}}, nil
}

func roleplayLibraryPlacementButton(
	selectedWorldID string,
	character roleplay.LibraryCharacterSummary,
) string {
	className := "shrink-0 rounded-lg border border-violet-300/30 bg-violet-300/10 px-2.5 py-1.5 text-[11px] font-semibold text-violet-100 transition hover:bg-violet-300/20 disabled:cursor-not-allowed disabled:opacity-50"
	if selectedWorldID == "" {
		return `<button type="button" class="` + className + `" disabled aria-disabled="true">Select world</button>`
	}
	if character.PlacedInSelectedWorld {
		return `<button type="button" class="` + className + `" disabled aria-disabled="true">In world</button>`
	}
	return `<button type="button" data-action="chat#placeRoleplayCharacter" data-library-character-id="` +
		html.EscapeString(character.ID) + `" class="` + className + `">Add</button>`
}

func roleplayNextOffset(offset, count int, hasMore bool) *int {
	if !hasMore {
		return nil
	}
	next := offset + count
	return &next
}

func roleplayWorkspacePagination(action, section string, next *int) string {
	if next == nil {
		return ""
	}
	return `<button type="button" data-action="chat#` + action + `" data-next-offset="` +
		strconv.Itoa(*next) + `" data-roleplay-page="` + section +
		`" class="w-full rounded-lg border border-white/10 px-3 py-2 text-xs font-semibold text-zinc-400 transition hover:border-violet-300/30 hover:text-violet-100 disabled:cursor-wait disabled:opacity-50">Load more</button>`
}
