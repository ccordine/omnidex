package api

import (
	"fmt"
	"html"
	"strings"
)

type roleplaySetupSection struct {
	Key         string
	Label       string
	Description string
	Body        string
}

func renderRoleplaySetupFlow(defaultKey string, sections []roleplaySetupSection) (string, error) {
	if len(sections) < 2 || len(sections) > 4 {
		return "", fmt.Errorf("roleplay setup requires two to four organized sections")
	}
	allowed := map[string]struct{}{"scene": {}, "cast": {}, "state": {}, "actions": {}}
	seen := make(map[string]struct{}, len(sections))
	defaultFound := false
	for _, section := range sections {
		if _, exists := allowed[section.Key]; !exists {
			return "", fmt.Errorf("roleplay setup section %q is not registered", section.Key)
		}
		if _, duplicate := seen[section.Key]; duplicate {
			return "", fmt.Errorf("roleplay setup section %q is duplicated", section.Key)
		}
		if section.Label == "" || section.Label != strings.TrimSpace(section.Label) ||
			section.Description == "" || section.Description != strings.TrimSpace(section.Description) ||
			strings.TrimSpace(section.Body) == "" {
			return "", fmt.Errorf("roleplay setup section %q is incomplete", section.Key)
		}
		seen[section.Key] = struct{}{}
		defaultFound = defaultFound || section.Key == defaultKey
	}
	if !defaultFound {
		return "", fmt.Errorf("roleplay setup default section %q is unavailable", defaultKey)
	}

	var navigation strings.Builder
	var panels strings.Builder
	for _, section := range sections {
		selected := section.Key == defaultKey
		selectedText := "false"
		tabIndex := `-1`
		buttonClass := "border-transparent text-zinc-400 hover:border-white/10 hover:bg-white/[.03] hover:text-zinc-100"
		hidden := ` hidden`
		if selected {
			selectedText = "true"
			tabIndex = `0`
			buttonClass = "border-violet-300/30 bg-violet-300/10 text-violet-100"
			hidden = ""
		}
		tabID := "roleplay-setup-tab-" + section.Key
		panelID := "roleplay-setup-panel-" + section.Key
		navigation.WriteString(`<button type="button" id="` + tabID + `" role="tab" aria-selected="` +
			selectedText + `" aria-controls="` + panelID + `" tabindex="` + tabIndex +
			`" data-roleplay-setup-tab="` + section.Key + `" data-action="chat#selectRoleplaySetupSection" class="min-w-[10rem] rounded-lg border px-3 py-2.5 text-left transition md:min-w-0 ` + buttonClass + `">` +
			`<span class="block text-xs font-semibold">` + html.EscapeString(section.Label) + `</span>` +
			`<span class="mt-1 hidden text-[10px] leading-4 text-zinc-500 md:block">` +
			html.EscapeString(section.Description) + `</span></button>`)
		panels.WriteString(`<section id="` + panelID + `" role="tabpanel" aria-labelledby="` + tabID +
			`" tabindex="0" data-roleplay-setup-panel="` + section.Key + `"` + hidden +
			` class="space-y-3 outline-none"><header class="pb-1"><h3 class="text-base font-semibold text-zinc-100">` +
			html.EscapeString(section.Label) + `</h3><p class="mt-1 text-xs leading-5 text-zinc-500">` +
			html.EscapeString(section.Description) + `</p></header>` + section.Body + `</section>`)
	}

	return `<div data-roleplay-setup-flow data-roleplay-default-setup-section="` + html.EscapeString(defaultKey) +
		`" class="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] md:grid-cols-[13rem_minmax(0,1fr)] md:grid-rows-1">` +
		`<nav role="tablist" aria-label="World setup sections" class="scrollbar flex gap-2 overflow-x-auto border-b border-white/10 bg-zinc-950/55 p-3 md:flex-col md:overflow-y-auto md:border-b-0 md:border-r">` +
		navigation.String() + `</nav><div class="scrollbar min-h-0 overflow-y-auto bg-zinc-950/20 p-4 md:p-5">` +
		panels.String() + `</div></div>`, nil
}
