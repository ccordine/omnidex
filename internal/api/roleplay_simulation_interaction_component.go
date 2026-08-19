package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

func renderRoleplayInteractions(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Configured interactions", "Use this server-defined syntax in the canonical chat composer."))
	if len(state.Interactions) == 0 {
		body.WriteString(chatEmptyState("No interactions are configured."))
	}
	for _, command := range state.Interactions {
		syntax := "/" + command.Key
		if command.ArgumentMode == roleplay.CommandArgumentRequired {
			syntax += ` "description"`
		}
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">` +
			`<div class="flex items-start justify-between gap-2"><h5 class="text-sm font-semibold text-zinc-100">` +
			html.EscapeString(command.Name) + `</h5><code class="rounded border border-white/10 bg-zinc-950 px-2 py-1 text-[10px] text-violet-100">` +
			html.EscapeString(syntax) + `</code></div><p class="mt-2 whitespace-pre-wrap text-xs text-zinc-300">` +
			html.EscapeString(command.Description) + `</p>`)
		body.WriteString(`<button type="button" data-action="chat#useRoleplayCommand" data-roleplay-command="` +
			html.EscapeString(syntax) + `" class="mt-2 rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-300 transition hover:border-violet-300/40">Place in composer</button></article>`)
	}
	pagination, err := renderRoleplayPagination(
		state, "interactions", state.Page.Interactions, len(state.Interactions), state.InteractionsMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func renderRoleplayItemTemplates(state roleplaySimulationComponentState) (string, error) {
	var body strings.Builder
	body.WriteString(roleplaySectionStart("Item templates", "These definitions supply exact /give and /take text for the canonical composer."))
	if len(state.ItemTemplates) == 0 {
		body.WriteString(chatEmptyState("No item templates are configured."))
	}
	for _, item := range state.ItemTemplates {
		give, err := roleplay.CanonicalItemAction(roleplay.SimulationActionGive, item.Name)
		if err != nil {
			return "", fmt.Errorf("render item template give syntax: %w", err)
		}
		take, err := roleplay.CanonicalItemAction(roleplay.SimulationActionTake, item.Name)
		if err != nil {
			return "", fmt.Errorf("render item template take syntax: %w", err)
		}
		uses := "unlimited uses"
		if item.UsePolicy == roleplay.ItemUseFinite {
			uses = strconv.Itoa(item.InitialUses) + " initial uses"
		}
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/60 p-3">` +
			`<div class="flex items-start justify-between gap-2"><h5 class="text-sm font-semibold text-zinc-100">` +
			html.EscapeString(item.Name) + `</h5><span class="rounded border border-white/10 px-2 py-1 text-[10px] text-zinc-400">` +
			html.EscapeString(uses) + `</span></div><p class="mt-2 whitespace-pre-wrap text-xs text-zinc-300">` +
			html.EscapeString(item.Description) + `</p><div class="mt-2 flex flex-wrap gap-2">`)
		body.WriteString(roleplayComposerCommandButton(give, "Place /give in composer"))
		body.WriteString(roleplayComposerCommandButton(take, "Place /take in composer"))
		body.WriteString(`</div></article>`)
	}
	pagination, err := renderRoleplayPagination(
		state, "item-templates", state.Page.ItemTemplates, len(state.ItemTemplates), state.ItemTemplatesMore,
	)
	if err != nil {
		return "", err
	}
	body.WriteString(pagination)
	body.WriteString(`</section>`)
	return body.String(), nil
}

func roleplayComposerCommandButton(command, label string) string {
	return `<button type="button" data-action="chat#useRoleplayCommand" data-roleplay-command="` +
		html.EscapeString(command) +
		`" class="rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-300 transition hover:border-violet-300/40">` +
		html.EscapeString(label) + `</button>`
}

func renderRoleplayDefinitionForms() string {
	return `<details class="rounded-md border border-white/10 bg-zinc-950/35 p-3">` +
		`<summary class="cursor-pointer text-xs font-semibold uppercase tracking-[.12em] text-zinc-200">Simulation configuration</summary>` +
		`<div class="mt-3 space-y-3">` + renderRoleplayMeterDefinitionForm() +
		renderRoleplayInteractionDefinitionForm() + renderRoleplayItemDefinitionForm() + `</div></details>`
}

func renderRoleplayMeterDefinitionForm() string {
	return `<form data-action="submit->chat#registerRoleplayMeter" class="space-y-2 rounded-md border border-white/10 bg-zinc-900/40 p-3">` +
		`<h5 class="text-xs font-semibold text-zinc-100">Register meter</h5>` +
		roleplayTextInput("key", "Key", "", "text", true) + roleplayTextInput("name", "Name", "", "text", true) +
		`<div class="grid grid-cols-3 gap-2">` + roleplayTextInput("minimum", "Minimum", "", "number", true) +
		roleplayTextInput("maximum", "Maximum", "", "number", true) +
		roleplayTextInput("initial_value", "Initial", "", "number", true) + `</div>` +
		roleplaySubmitButton("Register meter") + `</form>`
}

func renderRoleplayInteractionDefinitionForm() string {
	return `<form data-action="submit->chat#registerRoleplayInteraction" class="space-y-2 rounded-md border border-white/10 bg-zinc-900/40 p-3">` +
		`<h5 class="text-xs font-semibold text-zinc-100">Register interaction</h5>` +
		roleplayTextInput("key", "Command key", "", "text", true) + roleplayTextInput("name", "Name", "", "text", true) +
		roleplayTextArea("description", "Description", "", true) +
		`<label class="block text-[11px] text-zinc-400"><span>Argument mode</span><select name="argument_mode" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-950 px-2.5 py-2 text-xs text-zinc-100"><option value="none">None</option><option value="required">Required</option></select></label>` +
		roleplayTextArea("effects", "Meter effects, one key:delta per line", "", true) +
		roleplaySubmitButton("Register interaction") + `</form>`
}

func renderRoleplayItemDefinitionForm() string {
	return `<form data-action="submit->chat#registerRoleplayItem" class="space-y-2 rounded-md border border-white/10 bg-zinc-900/40 p-3">` +
		`<h5 class="text-xs font-semibold text-zinc-100">Register item template</h5>` +
		roleplayTextInput("name", "Name", "", "text", true) + roleplayTextArea("description", "Description", "", true) +
		`<div class="grid grid-cols-2 gap-2"><label class="block text-[11px] text-zinc-400"><span>Use policy</span><select name="use_policy" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-950 px-2.5 py-2 text-xs text-zinc-100"><option value="infinite">Infinite</option><option value="finite">Finite</option></select></label>` +
		roleplayTextInput("initial_uses", "Initial uses", "0", "number", true) + `</div>` +
		roleplayTextInput("priority", "Priority", strconv.Itoa(0), "number", true) +
		roleplayTextArea("effects", "Meter effects, one key:delta per line", "", true) +
		`<div class="grid grid-cols-3 gap-2">` + roleplayTextInput("trigger_meter_key", "Trigger meter (optional)", "", "text", false) +
		`<label class="block text-[11px] text-zinc-400"><span>Direction</span><select name="trigger_direction" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-950 px-2.5 py-2 text-xs text-zinc-100"><option value="">None</option><option value="at_or_below">At or below</option><option value="at_or_above">At or above</option></select></label>` +
		roleplayTextInput("trigger_threshold", "Threshold", "", "number", false) + `</div>` +
		roleplaySubmitButton("Register item") + `</form>`
}
