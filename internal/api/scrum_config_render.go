package api

import (
	"fmt"
	"html"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
)

var scrumModelConfigSourceLabels = map[string]string{
	"env":     "Environment defaults",
	"project": "Project overrides",
	"card":    "Card overrides",
}

var scrumAgentConfigSourceLabels = map[string]string{
	"instance":  "This run",
	"card":      "Card override",
	"project":   "Project override",
	"workspace": "Workspace default",
	"env":       "Environment fallback",
}

var scrumAgentSystemLabels = map[string]string{
	"omnidex": "Omnidex (local stack)",
	"cursor":  "Cursor SDK",
	"codex":   "Codex SDK",
}

var scrumAgentOptionLabels = map[string]string{
	"minimal":              "Minimal",
	"low":                  "Low",
	"medium":               "Medium",
	"high":                 "High",
	"xhigh":                "Extra high",
	"read-only":            "Read only",
	"workspace-write":      "Workspace write",
	"danger-full-access":   "Danger full access",
	"never":                "Never",
	"on-request":           "On request",
	"on-failure":           "On failure",
	"untrusted":            "Untrusted",
	"disabled":             "Disabled",
	"cached":               "Cached",
	"live":                 "Live",
	"true":                 "Enabled",
	"false":                "Disabled",
}

var scrumPreAlphaAgents = map[string]struct{}{
	"omnidex": {},
}

func renderPreAlphaBadgeHTML() string {
	return `<span class="inline-flex shrink-0 items-center gap-1 rounded-full border border-fuchsia-400/45 bg-gradient-to-r from-fuchsia-500/25 via-amber-400/20 to-orange-400/25 px-2 py-0.5 text-[10px] font-bold uppercase tracking-[0.14em] text-fuchsia-100 shadow-[0_0_12px_rgba(232,121,249,.25)]" title="Omnidex is in pre-alpha"><span aria-hidden="true" class="text-[11px] leading-none">🚩</span>Pre-alpha</span>`
}

func renderModelConfigSectionHTML(fields []scrumConfigField, overrides map[string]string, resolvedSource, entityID string) string {
	if len(fields) == 0 {
		return ""
	}
	sourceLabel := scrumModelConfigSourceLabels[resolvedSource]
	if sourceLabel == "" {
		sourceLabel = resolvedSource
	}
	var rows strings.Builder
	for _, field := range fields {
		override := strings.TrimSpace(overrides[field.Key])
		inherited := strings.TrimSpace(field.Value)
		control := renderScrumModelFieldControl(field, override, inherited)
		rows.WriteString(fmt.Sprintf(`
        <label class="block">
          <span class="text-xs text-zinc-500">%s</span>
          %s
          <span class="mt-1 block text-[11px] leading-5 text-zinc-600">%s</span>
        </label>
      `, html.EscapeString(field.Label), control, html.EscapeString(field.Description)))
	}
	return fmt.Sprintf(`
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex models</h3>
          <p class="mt-1 text-xs text-zinc-500">Card-level overrides. Empty fields inherit from project, then environment.</p>
        </div>
        <span class="rounded-full border border-white/10 bg-zinc-900/80 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">Effective: %s</span>
      </div>
      <div class="mt-4 grid gap-4 lg:grid-cols-2">%s</div>
      <div class="mt-4 flex flex-wrap gap-2">
        <button type="button" data-action="scrum#saveModelConfig" data-card-id="%s" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save model settings</button>
        <button type="button" data-action="scrum#clearModelConfig" data-card-id="%s" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">Clear overrides</button>
      </div>
    </section>
  `, html.EscapeString(sourceLabel), rows.String(), html.EscapeString(entityID), html.EscapeString(entityID))
}

func renderScrumModelFieldControl(field scrumConfigField, override, inherited string) string {
	fieldAttr := fmt.Sprintf(`data-scrum-field="model_%s"`, html.EscapeString(field.Key))
	if len(field.Options) > 0 {
		var options strings.Builder
		inheritSuffix := ""
		if inherited != "" {
			inheritSuffix = fmt.Sprintf(" (%s)", html.EscapeString(inherited))
		}
		options.WriteString(fmt.Sprintf(`<option value="">Inherit%s</option>`, inheritSuffix))
		for _, option := range field.Options {
			selected := ""
			if override == option {
				selected = " selected"
			}
			options.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(option), selected, html.EscapeString(option)))
		}
		return fmt.Sprintf(`<select %s class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40">%s</select>`, fieldAttr, options.String())
	}
	return fmt.Sprintf(`<input type="text" %s value="%s" placeholder="%s" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />`,
		fieldAttr, html.EscapeString(override), html.EscapeString(firstNonEmpty(inherited, "Inherit default")))
}

func renderAgentConfigSectionHTML(fields []scrumConfigField, overrides map[string]string, resolvedSource, resolvedSystem, entityID string) string {
	if len(fields) == 0 {
		return ""
	}
	sourceLabel := scrumAgentConfigSourceLabels[resolvedSource]
	if sourceLabel == "" {
		sourceLabel = resolvedSource
	}
	systemLabel := scrumAgentSystemLabels[resolvedSystem]
	if systemLabel == "" {
		systemLabel = resolvedSystem
	}
	systemBadge := ""
	if _, ok := scrumPreAlphaAgents[resolvedSystem]; ok {
		systemBadge = " " + renderPreAlphaBadgeHTML()
	}
	var rows strings.Builder
	for _, field := range fields {
		override := strings.TrimSpace(overrides[field.Key])
		inherited := strings.TrimSpace(field.Value)
		switch field.Key {
		case "agent_system":
			rows.WriteString(renderScrumAgentSystemPickerHTML(field, override, inherited))
		case "agent_strict":
			checked := override == "true" || (override == "" && inherited == "true")
			rows.WriteString(fmt.Sprintf(`
          <label class="flex items-start gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-3">
            <input type="checkbox" data-scrum-field="agent_%s" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"%s />
            <span>
              <span class="block text-sm text-zinc-200">%s</span>
              <span class="mt-1 block text-[11px] leading-5 text-zinc-600">%s</span>
            </span>
          </label>
        `, html.EscapeString(field.Key), checkedAttr(checked), html.EscapeString(field.Label), html.EscapeString(field.Description)))
		default:
			rows.WriteString(renderScrumAgentOptionFieldHTML(field, override, inherited))
		}
	}
	return fmt.Sprintf(`
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Execution agent</h3>
          <p class="mt-1 text-xs text-zinc-500">Card override for who runs work. Priority: this run → card → project → workspace → environment.</p>
        </div>
        <div class="space-y-1 text-right">
          <span class="block rounded-full border border-white/10 bg-zinc-900/80 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide text-zinc-400">Effective: %s</span>
          <span class="flex flex-wrap items-center justify-end gap-2 font-mono text-[11px] text-cyan-200">%s%s</span>
        </div>
      </div>
      <div class="mt-4 grid gap-4">%s</div>
      <div class="mt-4 flex flex-wrap gap-2">
        <button type="button" data-action="scrum#saveAgentConfig" data-card-id="%s" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save agent settings</button>
        <button type="button" data-action="scrum#clearAgentConfig" data-card-id="%s" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300 hover:border-cyan-300/40 hover:bg-cyan-300/10">Clear overrides</button>
      </div>
    </section>
  `, html.EscapeString(sourceLabel), html.EscapeString(systemLabel), systemBadge, rows.String(), html.EscapeString(entityID), html.EscapeString(entityID))
}

func renderScrumAgentSystemPickerHTML(field scrumConfigField, override, inherited string) string {
	options := field.Options
	if len(options) == 0 {
		options = []string{agentconfig.SystemOmnidex, agentconfig.SystemCursor, agentconfig.SystemCodex}
	}
	selected := override
	inheritLabel := scrumAgentSystemLabels[inherited]
	if inheritLabel == "" {
		inheritLabel = inherited
	}
	fieldAttr := fmt.Sprintf(`data-scrum-field="agent_%s"`, html.EscapeString(field.Key))
	inheritBadge := ""
	if _, ok := scrumPreAlphaAgents[inherited]; ok {
		inheritBadge = renderPreAlphaBadgeHTML()
	}
	var rows strings.Builder
	rows.WriteString(renderScrumAgentSystemRadioRow(fieldAttr, field.Key, "", fmt.Sprintf(`Inherit (<span class="text-zinc-400">%s</span>)`, html.EscapeString(inheritLabel)), inheritBadge, selected == ""))
	for _, option := range options {
		label := scrumAgentSystemLabels[option]
		if label == "" {
			label = option
		}
		badge := ""
		if _, ok := scrumPreAlphaAgents[option]; ok {
			badge = renderPreAlphaBadgeHTML()
		}
		rows.WriteString(renderScrumAgentSystemRadioRow(fieldAttr, field.Key, option, html.EscapeString(label), badge, selected == option))
	}
	return fmt.Sprintf(`
    <fieldset class="block">
      <legend class="text-xs text-zinc-500">%s</legend>
      <div class="mt-2 grid gap-2">%s</div>
      <span class="mt-2 block text-[11px] leading-5 text-zinc-600">%s</span>
    </fieldset>
  `, html.EscapeString(field.Label), rows.String(), html.EscapeString(field.Description))
}

func renderScrumAgentSystemRadioRow(fieldAttr, fieldKey, value, labelHTML, badge string, active bool) string {
	classes := "border-white/10 bg-zinc-900/50 hover:border-white/20"
	if active {
		classes = "border-cyan-300/40 bg-cyan-300/10"
	}
	checked := ""
	if active {
		checked = " checked"
	}
	return fmt.Sprintf(`
      <label class="flex cursor-pointer items-center gap-3 rounded-md border px-3 py-2.5 transition %s">
        <input type="radio" %s name="agent_scrum_%s" value="%s" class="mt-0.5 border-white/20 bg-zinc-900 text-cyan-300 focus:ring-cyan-300/40"%s />
        <span class="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-sm text-zinc-200">%s%s</span>
      </label>
    `, classes, fieldAttr, html.EscapeString(fieldKey), html.EscapeString(value), checked, labelHTML, badge)
}

func renderScrumAgentOptionFieldHTML(field scrumConfigField, override, inherited string) string {
	fieldAttr := fmt.Sprintf(`data-scrum-field="agent_%s"`, html.EscapeString(field.Key))
	if len(field.Options) > 0 {
		inheritLabel := "default"
		if inherited != "" {
			inheritLabel = scrumAgentOptionLabels[inherited]
			if inheritLabel == "" {
				inheritLabel = inherited
			}
		}
		var options strings.Builder
		options.WriteString(fmt.Sprintf(`<option value="">Inherit (%s)</option>`, html.EscapeString(inheritLabel)))
		for _, option := range field.Options {
			selected := ""
			if override == option {
				selected = " selected"
			}
			label := scrumAgentOptionLabels[option]
			if label == "" {
				label = option
			}
			options.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(option), selected, html.EscapeString(label)))
		}
		return fmt.Sprintf(`
      <label class="block">
        <span class="text-xs text-zinc-500">%s</span>
        <select %s class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40">%s</select>
        <span class="mt-1 block text-[11px] leading-5 text-zinc-600">%s</span>
      </label>
    `, html.EscapeString(field.Label), fieldAttr, options.String(), html.EscapeString(field.Description))
	}
	return fmt.Sprintf(`
    <label class="block">
      <span class="text-xs text-zinc-500">%s</span>
      <input %s value="%s" placeholder="%s" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />
      <span class="mt-1 block text-[11px] leading-5 text-zinc-600">%s</span>
    </label>
  `, html.EscapeString(field.Label), fieldAttr, html.EscapeString(override), html.EscapeString(firstNonEmpty(inherited, "Inherit default")), html.EscapeString(field.Description))
}

func checkedAttr(checked bool) string {
	if checked {
		return ` checked`
	}
	return ""
}
