package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

func renderRoleplayPagination(
	state roleplaySimulationComponentState,
	section string,
	offset, count int,
	hasMore bool,
) (string, error) {
	if offset == 0 && !hasMore {
		return "", nil
	}
	var controls strings.Builder
	controls.WriteString(`<nav class="mt-2 flex justify-end gap-2" aria-label="` + html.EscapeString(section) + ` pages">`)
	if offset > 0 {
		previous := offset - roleplaySimulationPageSize
		if previous < 0 {
			previous = 0
		}
		button, err := roleplayPageButton(state, section, previous, "Previous")
		if err != nil {
			return "", err
		}
		controls.WriteString(button)
	}
	if hasMore {
		button, err := roleplayPageButton(state, section, offset+count, "Next")
		if err != nil {
			return "", err
		}
		controls.WriteString(button)
	}
	controls.WriteString(`</nav>`)
	return controls.String(), nil
}

func roleplayPageButton(
	state roleplaySimulationComponentState,
	section string,
	offset int,
	label string,
) (string, error) {
	page := state.Page
	switch section {
	case "characters":
		page.Characters = offset
	case "personas":
		page.Personas = offset
	case "turn-order":
		page.TurnOrder = offset
	case "meters":
		page.Meters = offset
	case "inventory":
		page.Inventory = offset
	case "interactions":
		page.Interactions = offset
	case "item-templates":
		page.ItemTemplates = offset
	default:
		return "", fmt.Errorf("unsupported roleplay page section %q", section)
	}
	return `<button type="button" data-action="chat#loadRoleplayPage" data-roleplay-page-section="` +
		html.EscapeString(section) + `"` + roleplayPageAttributes(page) +
		` class="rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-300 transition hover:border-violet-300/40 disabled:cursor-wait disabled:opacity-60">` +
		html.EscapeString(label) + `</button>`, nil
}

func roleplayPageAttributes(page roleplaySimulationPageState) string {
	return ` data-characters-offset="` + strconv.Itoa(page.Characters) + `"` +
		` data-personas-offset="` + strconv.Itoa(page.Personas) + `"` +
		` data-turn-order-offset="` + strconv.Itoa(page.TurnOrder) + `"` +
		` data-meters-offset="` + strconv.Itoa(page.Meters) + `"` +
		` data-inventory-offset="` + strconv.Itoa(page.Inventory) + `"` +
		` data-interactions-offset="` + strconv.Itoa(page.Interactions) + `"` +
		` data-item-templates-offset="` + strconv.Itoa(page.ItemTemplates) + `"`
}
