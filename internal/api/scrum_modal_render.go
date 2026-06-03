package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
)

var scrumModalTabs = []struct {
	ID    string
	Label string
}{
	{"card", "Card"},
	{"files", "Files"},
	{"tests", "Tests"},
	{"config", "Config"},
	{"recipe", "Recipe"},
	{"channel", "Channel"},
}

func renderScrumModalTabNavHTML(card ScrumCard, activeTab string) string {
	var b strings.Builder
	for _, tab := range scrumModalTabs {
		classes := "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-200"
		if tab.ID == activeTab {
			classes = "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
		}
		b.WriteString(fmt.Sprintf(
			`<button type="button" data-action="scrum#showCardTab" data-scrum-tab="%s" class="inline-flex items-center rounded-md border px-3 py-2 text-sm font-medium transition %s">%s%s</button>`,
			html.EscapeString(tab.ID),
			classes,
			html.EscapeString(tab.Label),
			scrumModalTabBadgeHTML(card, tab.ID),
		))
	}
	return b.String()
}

func renderScrumModalToolbarHTML(card ScrumCard, board ScrumBoard, playQueue map[string]any) string {
	moveOptions := scrumModalColumnOptionsHTML(card.Column)
	hasJob := strings.TrimSpace(card.JobID) != ""
	jobLine := ""
	if hasJob {
		jobLine = fmt.Sprintf(`<p class="w-full font-mono text-[11px] text-cyan-200/80">Job #%s · %s</p>`, html.EscapeString(card.JobID), html.EscapeString(firstNonEmpty(board.ProjectDirectory, "not set")))
	}
	markDone := ""
	if normalizeScrumColumn(card.Column) == "review" {
		markDone = fmt.Sprintf(`<button type="button" data-action="scrum#markDone" data-card-id="%s" class="rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-1.5 text-xs font-semibold text-emerald-200 hover:bg-emerald-400/20">Mark done</button>`, html.EscapeString(card.ID))
	}
	syncJob := ""
	if hasJob && normalizeScrumColumn(card.Column) != "done" {
		syncJob = fmt.Sprintf(`<button type="button" data-action="scrum#syncJob" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200">Sync job</button>`, html.EscapeString(card.ID))
	}
	return fmt.Sprintf(`
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-zinc-950/40 px-4 py-3 md:px-5">
      <div class="flex flex-wrap items-center gap-2">
        <span class="%s">%s</span>
        %s
        <select data-action="change->scrum#modalMoveSelect" data-card-id="%s" class="rounded-md border border-white/10 bg-zinc-900 px-2 py-1.5 text-xs text-zinc-100 outline-none">%s</select>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        %s
        %s
        %s
        %s
        <button type="button" data-action="scrum#deleteCard" data-card-id="%s" class="rounded-md border border-rose-400/25 px-3 py-1.5 text-xs text-rose-300 hover:bg-rose-400/10">Delete</button>
      </div>
      %s
    </div>
  `,
		scrumStatusPillClass(card.Column),
		html.EscapeString(scrumColumnLabel(card.Column)),
		renderScrumPlayStateBadgeHTML(card),
		html.EscapeString(card.ID),
		moveOptions,
		renderScrumModalAssignCTAHTML(card),
		renderScrumModalPlayActionsHTML(card, playQueue),
		markDone,
		syncJob,
		html.EscapeString(card.ID),
		jobLine,
	)
}

func renderScrumModalDetailsHTML(card ScrumCard) string {
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Summary</h3>
        <button type="button" data-action="scrum#saveDetails" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-200 transition hover:border-cyan-300/40 hover:bg-cyan-300/10">Save</button>
      </div>
      <input data-scrum-field="title" type="text" value="%s" class="mt-3 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-lg font-semibold text-zinc-100 outline-none focus:border-cyan-300/40" />
      <textarea data-scrum-field="description" rows="8" placeholder="Describe the work, context, and acceptance criteria…" class="scrollbar mt-3 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-200 outline-none focus:border-cyan-300/40">%s</textarea>
    </section>
  `, html.EscapeString(card.ID), html.EscapeString(card.Title), html.EscapeString(card.Description))
}

func renderScrumModalChecklistHTML(card ScrumCard) string {
	var items strings.Builder
	for _, item := range card.Checklist {
		checked := ""
		textClass := ""
		if item.Done {
			checked = " checked"
			textClass = "text-zinc-500 line-through"
		}
		items.WriteString(fmt.Sprintf(`
      <div class="flex items-start gap-2 rounded-md border border-white/10 bg-zinc-950/40 px-3 py-2">
        <label class="flex min-w-0 flex-1 items-start gap-3 text-sm text-zinc-200">
          <input type="checkbox" data-action="change->scrum#toggleChecklistItem" data-card-id="%s" data-item-id="%s" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"%s />
          <span class="%s">%s</span>
        </label>
        <button type="button" data-action="scrum#removeChecklistItem" data-card-id="%s" data-item-id="%s" class="shrink-0 rounded px-1.5 py-0.5 text-xs text-zinc-500 hover:bg-rose-400/10 hover:text-rose-300" title="Remove">×</button>
      </div>
    `, html.EscapeString(card.ID), html.EscapeString(item.ID), checked, textClass, html.EscapeString(item.Text), html.EscapeString(card.ID), html.EscapeString(item.ID)))
	}
	done, total := scrumChecklistProgress(card.Checklist)
	empty := `<p class="text-sm text-zinc-500">No checklist items yet. Add tasks Omnidex should complete.</p>`
	if items.Len() == 0 {
		items.WriteString(empty)
	}
	header := "Checklist"
	if total > 0 {
		header = fmt.Sprintf("Checklist · %d/%d", done, total)
	}
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">%s</h3>
      </div>
      <div class="mt-3 space-y-2">%s</div>
      <form data-action="submit->scrum#addChecklistItem" data-card-id="%s" class="mt-3 flex gap-2">
        <input data-scrum-field="checklistText" type="text" placeholder="Add checklist item" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
        <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Add</button>
      </form>
    </section>
  `, html.EscapeString(header), items.String(), html.EscapeString(card.ID))
}

func renderScrumModalRefFilesHTML(card ScrumCard, files, dirs []string) string {
	var attached strings.Builder
	for _, file := range card.RefFiles {
		attached.WriteString(fmt.Sprintf(`
    <li class="flex items-center justify-between gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2">
      <span class="min-w-0 truncate font-mono text-xs text-zinc-200">%s</span>
      <button type="button" data-action="scrum#removeRefFile" data-card-id="%s" data-ref-file="%s" class="shrink-0 text-xs text-rose-300 hover:text-rose-200">Remove</button>
    </li>
  `, html.EscapeString(file), html.EscapeString(card.ID), html.EscapeString(file)))
	}
	refSet := map[string]struct{}{}
	for _, file := range card.RefFiles {
		refSet[file] = struct{}{}
	}
	var dirOptions, fileOptions strings.Builder
	dirLimit := 80
	for _, dir := range dirs {
		if _, used := refSet[dir]; used {
			continue
		}
		dirOptions.WriteString(fmt.Sprintf(`<option value="%s">dir/%s</option>`, html.EscapeString(dir), html.EscapeString(dir)))
		dirLimit--
		if dirLimit <= 0 {
			break
		}
	}
	limit := 160
	for _, file := range files {
		if _, used := refSet[file]; used {
			continue
		}
		fileOptions.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, html.EscapeString(file), html.EscapeString(file)))
		limit--
		if limit <= 0 {
			break
		}
	}
	attachForm := ""
	if dirOptions.Len() > 0 || fileOptions.Len() > 0 {
		attachForm = fmt.Sprintf(`<form data-action="submit->scrum#addRefFile" data-card-id="%s" class="mt-3 flex flex-wrap gap-2"><select data-scrum-field="refFile" class="min-w-[12rem] flex-1 rounded-md border border-white/10 bg-zinc-900 px-2 py-2 text-xs text-zinc-100 outline-none"><option value="">Pick project file or directory…</option>%s%s</select><button type="submit" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40">Attach</button></form>`, html.EscapeString(card.ID), dirOptions.String(), fileOptions.String())
	}
	list := attached.String()
	if list == "" {
		list = `<li class="text-sm text-zinc-500">No files attached.</li>`
	}
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Reference files</h3>
      <p class="mt-1 text-xs text-zinc-500">Attach project files and directories Omnidex should read when playing this card.</p>
      <ul class="mt-3 space-y-2">%s</ul>
      %s
    </section>
  `, list, attachForm)
}

func renderScrumModalFilesTabHTML(card ScrumCard, files, dirs []string) string {
	return fmt.Sprintf(`
    <div class="mx-auto max-w-4xl space-y-4">
      %s
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Upload context files</h3>
        <p class="mt-1 text-xs text-zinc-500">Uploads are saved into <span class="font-mono text-zinc-400">.omni/card-files/%s</span> and attached to this card.</p>
        <form data-action="submit->scrum#uploadRefFiles" data-card-id="%s" class="mt-3 flex flex-wrap items-center gap-2">
          <input data-scrum-field="uploadFiles" type="file" multiple class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 file:mr-3 file:rounded file:border-0 file:bg-cyan-300 file:px-2 file:py-1 file:text-xs file:font-semibold file:text-zinc-950" />
          <button type="submit" class="rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Upload</button>
        </form>
      </section>
    </div>
  `, renderScrumModalRefFilesHTML(card, files, dirs), html.EscapeString(card.ID), html.EscapeString(card.ID))
}

func renderScrumModalTestCriteriaHTML(card ScrumCard) string {
	var items strings.Builder
	for _, item := range card.TestCriteria {
		checked := ""
		textClass := ""
		if item.Done {
			checked = " checked"
			textClass = "text-zinc-500 line-through"
		}
		items.WriteString(fmt.Sprintf(`
      <div class="flex items-start gap-2 rounded-md border border-emerald-400/15 bg-emerald-400/5 px-3 py-2">
        <label class="flex min-w-0 flex-1 items-start gap-3 text-sm text-zinc-200">
          <input type="checkbox" data-action="change->scrum#toggleTestCriterion" data-card-id="%s" data-item-id="%s" class="mt-1 rounded border-white/20 bg-zinc-900 text-emerald-300"%s />
          <span class="%s">%s</span>
        </label>
        <button type="button" data-action="scrum#removeTestCriterion" data-card-id="%s" data-item-id="%s" class="shrink-0 rounded px-1.5 py-0.5 text-xs text-zinc-500 hover:bg-rose-400/10 hover:text-rose-300" title="Remove">×</button>
      </div>
    `, html.EscapeString(card.ID), html.EscapeString(item.ID), checked, textClass, html.EscapeString(item.Text), html.EscapeString(card.ID), html.EscapeString(item.ID)))
	}
	done, total := scrumChecklistProgress(card.TestCriteria)
	empty := `<p class="text-sm text-zinc-500">No tests defined. Add unit, integration, or manual verification steps.</p>`
	if items.Len() == 0 {
		items.WriteString(empty)
	}
	progress := ""
	if total > 0 {
		progress = fmt.Sprintf(" · %d/%d passing", done, total)
	}
	return fmt.Sprintf(`
    <section class="rounded-lg border border-emerald-400/20 bg-zinc-950/50 p-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-emerald-400/80">Test criteria</h3>
          <p class="mt-1 text-xs text-zinc-500">Tests the AI should implement or satisfy before this card is done.%s</p>
        </div>
      </div>
      <div class="mt-3 space-y-2">%s</div>
      <form data-action="submit->scrum#addTestCriterion" data-card-id="%s" class="mt-3 flex gap-2">
        <input data-scrum-field="testCriterionText" type="text" placeholder="e.g. go test ./internal/api passes" class="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-emerald-300/40" />
        <button type="submit" class="rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-2 text-xs font-semibold text-emerald-100 hover:bg-emerald-400/20">Add</button>
      </form>
    </section>
  `, html.EscapeString(progress), items.String(), html.EscapeString(card.ID))
}

func renderScrumModalTestsTabHTML(card ScrumCard) string {
	return fmt.Sprintf(`
    <div class="mx-auto max-w-3xl space-y-4">
      <p class="text-sm leading-6 text-zinc-400">Define what “done” means for this card. Play and channel runs include these criteria in agent context; check them off as the agent satisfies each one.</p>
      %s
    </div>
  `, renderScrumModalTestCriteriaHTML(card))
}

func renderScrumCoachChatHTML(card ScrumCard) string {
	if len(card.PlanningChat) == 0 {
		return `<p class="text-xs text-zinc-500">Ask the coach to refine scope, split work, or draft a card ticket prompt.</p>`
	}
	var b strings.Builder
	for _, msg := range card.PlanningChat {
		shell := "border-white/10 bg-zinc-900/70"
		if strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			shell = "border-cyan-300/25 bg-cyan-300/10"
		}
		b.WriteString(fmt.Sprintf(`<div class="rounded-md border %s px-3 py-2"><div class="text-[10px] uppercase tracking-wide text-zinc-500">%s</div><div class="mt-1 whitespace-pre-wrap text-xs leading-5 text-zinc-200">%s</div></div>`, shell, html.EscapeString(msg.Role), html.EscapeString(msg.Content)))
	}
	return b.String()
}

func renderScrumCoachPanelHTML(card ScrumCard) string {
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card coach</h3>
          <p class="mt-1 text-[11px] leading-5 text-zinc-500">Meta-planning for this card only. Try <span class="font-mono text-zinc-400">/plan</span> <span class="font-mono text-zinc-400">/research</span> <span class="font-mono text-zinc-400">/card</span> <span class="font-mono text-zinc-400">/scan</span></p>
        </div>
      </div>
      <div class="scrollbar mt-3 max-h-36 space-y-2 overflow-y-auto pr-1" data-recyclr-sink="scrum-coach-toasts"><p class="text-xs text-zinc-600">Coach suggestions appear here as you edit.</p></div>
      <div class="scrollbar mt-3 max-h-52 space-y-2 overflow-y-auto pr-1" data-recyclr-sink="scrum-coach-chat">%s</div>
      <form data-action="submit->scrum#sendCoach" data-card-id="%s" class="mt-3 flex gap-2">
        <textarea data-scrum-field="coachMessage" rows="2" placeholder="Talk to the coach… /plan /research /card" class="scrollbar min-w-0 flex-1 resize-none rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
        <button type="submit" class="self-end rounded-md bg-cyan-300 px-3 py-2 text-xs font-semibold text-zinc-950 hover:bg-cyan-200">Send</button>
      </form>
    </section>
  `, renderScrumCoachChatHTML(card), html.EscapeString(card.ID))
}

func renderScrumCoachConfigSectionHTML(card ScrumCard) string {
	cfg := parseScrumCoachConfig(card.CoachConfig)
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Card coach</h3>
          <p class="mt-1 text-sm leading-6 text-zinc-400">Planning assistant settings for this card. Auto-suggest is off unless explicitly enabled here.</p>
        </div>
        <button type="button" data-action="scrum#saveCoachConfig" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40">Save coach settings</button>
      </div>
      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <label class="flex cursor-pointer items-start gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-3 text-xs text-zinc-300">
          <input type="checkbox" data-scrum-field="coachEnabled" class="mt-0.5 rounded border-white/20 bg-zinc-900 text-cyan-300"%s />
          <span>
            <span class="block font-medium text-zinc-100">Coach chat</span>
            <span class="mt-1 block text-zinc-500">Allow manual coach messages on the Card tab.</span>
          </span>
        </label>
        <label class="flex cursor-pointer items-start gap-3 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-3 text-xs text-zinc-300">
          <input type="checkbox" data-scrum-field="coachAutoScan" class="mt-0.5 rounded border-white/20 bg-zinc-900 text-cyan-300"%s />
          <span>
            <span class="block font-medium text-zinc-100">Auto-suggest while editing</span>
            <span class="mt-1 block text-zinc-500">Run the coach automatically after card field edits.</span>
          </span>
        </label>
        <label class="block text-xs text-zinc-500 sm:col-span-2">
          Model
          <input data-scrum-field="coachModel" type="text" value="%s" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs text-zinc-100 outline-none focus:border-cyan-300/40" />
        </label>
      </div>
    </section>
  `, html.EscapeString(card.ID), checkedAttr(cfg.Enabled), checkedAttr(cfg.AutoScan), html.EscapeString(cfg.Model))
}

func renderScrumModalCardTabHTML(card ScrumCard) string {
	return fmt.Sprintf(`
    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(280px,360px)]">
      <div class="space-y-4">
        %s
        %s
        %s
      </div>
      <div class="space-y-4 xl:sticky xl:top-0 xl:self-start">
        %s
        %s
      </div>
    </div>
  `,
		renderScrumModalDetailsHTML(card),
		renderScrumModalChecklistHTML(card),
		renderScrumModalCardTicketHTML(card),
		renderScrumModalTagsPanelHTML(card),
		renderScrumCoachPanelHTML(card),
	)
}

func renderScrumModalConfigTabHTML(ctx scrumModalRenderContext) string {
	card := ctx.Card
	agentOverrides := ctx.AgentOverrides
	usingCursor := ctx.AgentSystem == agentconfig.SystemCursor || strings.TrimSpace(agentOverrides["agent_system"]) == agentconfig.SystemCursor
	usingOmnidex := ctx.AgentSystem == agentconfig.SystemOmnidex || strings.TrimSpace(agentOverrides["agent_system"]) == agentconfig.SystemOmnidex
	cursorClass := "border-white/10 text-zinc-200 hover:border-cyan-300/40"
	if usingCursor {
		cursorClass = "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
	}
	omnidexClass := cursorClass
	if usingOmnidex {
		omnidexClass = "border-cyan-300/40 bg-cyan-300/10 text-cyan-100"
	}
	modelSection := `<p class="text-sm text-zinc-500">Model config unavailable.</p>`
	if len(ctx.ModelFields) > 0 {
		modelSection = renderModelConfigSectionHTML(ctx.ModelFields, ctx.ModelOverrides, ctx.ModelSource, card.ID)
	}
	agentSection := ""
	if len(ctx.AgentFields) > 0 {
		agentSection = renderAgentConfigSectionHTML(ctx.AgentFields, ctx.AgentOverrides, ctx.AgentSource, ctx.AgentSystem, card.ID)
	}
	return fmt.Sprintf(`
    <div class="space-y-4">
      %s
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Execution layer</h3>
        <p class="mt-2 text-sm leading-6 text-zinc-400">Play runs the resolved agent (card → project → env) with full card context: title, description, checklist, card ticket draft, ref files, and recipe. A programmatic manager reads <span class="font-mono text-zinc-300">SCRUM_STATUS:</span> from agent output to move the card to review, blocked, or back to assigned.</p>
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="%s" data-agent-system="cursor" class="rounded-md border %s px-3 py-1.5 text-xs font-semibold">Use Cursor</button>
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="%s" data-agent-system="codex" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 hover:border-cyan-300/40">Use Codex</button>
          <button type="button" data-action="scrum#quickSetAgent" data-card-id="%s" data-agent-system="omnidex" class="inline-flex items-center gap-2 rounded-md border %s px-3 py-1.5 text-xs font-semibold">Use Omnidex %s</button>
        </div>
        <p class="mt-3 text-[11px] leading-5 text-zinc-600">Cursor/Codex also need an API key — set under <span class="font-semibold text-zinc-500">Admin → API secrets</span> (DB, preferred) or <span class="font-mono">CURSOR_API_KEY</span> / <span class="font-mono">CODEX_API_KEY</span> in env. Project agent choice overrides env defaults.</p>
      </section>
      <p class="text-xs text-zinc-500">Overrides inherit project → environment.</p>
      %s
      %s
    </div>
  `,
		renderScrumCoachConfigSectionHTML(card),
		html.EscapeString(card.ID), cursorClass,
		html.EscapeString(card.ID),
		html.EscapeString(card.ID), omnidexClass, renderPreAlphaBadgeHTML(),
		modelSection,
		agentSection,
	)
}

func renderScrumModalRecipeTabHTML(ctx scrumModalRenderContext) string {
	card := ctx.Card
	effectiveRecipeID := strings.TrimSpace(card.RecipeID)
	if effectiveRecipeID == "" {
		effectiveRecipeID = strings.TrimSpace(ctx.ProjectRecipeID)
	}
	effectiveRecipe := jsonRawObjectMap(card.Recipe)
	if len(effectiveRecipe) == 0 {
		effectiveRecipe = ctx.ProjectRecipe
	}
	if effectiveRecipe == nil {
		effectiveRecipe = map[string]any{}
	}
	var options strings.Builder
	for _, recipe := range ctx.Recipes {
		selected := ""
		if recipe.ID == effectiveRecipeID {
			selected = " selected"
		}
		options.WriteString(fmt.Sprintf(`<option value="%s"%s>%s — %s</option>`, html.EscapeString(recipe.ID), selected, html.EscapeString(recipe.ID), html.EscapeString(recipe.Description)))
	}
	recipeJSON, _ := json.MarshalIndent(effectiveRecipe, "", "  ")
	inherited := strings.TrimSpace(card.RecipeID) == "" && len(jsonRawObjectMap(card.Recipe)) == 0 && (strings.TrimSpace(ctx.ProjectRecipeID) != "" || len(ctx.ProjectRecipe) > 0)
	inheritNote := "Card-specific recipe used when this card plays."
	if inherited {
		inheritNote = "Inheriting project recipe until you save card overrides."
	}
	return fmt.Sprintf(`
    <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Omnidex recipe</h3>
          <p class="mt-1 text-xs text-zinc-500">%s</p>
        </div>
        <select data-scrum-field="recipeId" class="max-w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">
          <option value="">No catalog recipe</option>
          %s
        </select>
      </div>
      <textarea data-scrum-field="recipeJson" rows="18" class="scrollbar mt-4 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 font-mono text-xs leading-5 text-zinc-100 outline-none focus:border-cyan-300/40">%s</textarea>
      <div class="mt-3 flex flex-wrap gap-2">
        <button type="button" data-action="scrum#loadCatalogRecipe" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-2 text-xs text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Load catalog template</button>
        <button type="button" data-action="scrum#saveRecipe" data-card-id="%s" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Save recipe</button>
      </div>
    </section>
  `, html.EscapeString(inheritNote), options.String(), html.EscapeString(string(recipeJSON)), html.EscapeString(card.ID), html.EscapeString(card.ID))
}

func renderScrumModalChannelTabHTML(ctx scrumModalRenderContext) string {
	card := ctx.Card
	componentID := scrumCardChatComponentID(card.ID)
	chatEndpoint := fmt.Sprintf("/v1/scrum/cards/%s/chat", url.PathEscape(card.ID))
	if ctx.ProjectID > 0 {
		chatEndpoint += fmt.Sprintf("?project_id=%d", ctx.ProjectID)
	}
	isLive := card.PlayState == "running" || card.PlayState == "queued" || card.PlayState == "reviewing"
	isRunning := card.PlayState == "running" || card.PlayState == "reviewing"
	liveBadge := scrumChannelLiveBadgeHTML(card)
	status := scrumChannelSessionStatusHTML(card, ctx.PlayQueue)
	interrupt := ""
	if isRunning {
		interrupt = fmt.Sprintf(`<button type="button" data-action="scrum#pausePlay" data-card-id="%s" class="rounded-md border border-rose-400/35 bg-rose-400/10 px-3 py-1.5 text-xs font-semibold text-rose-100 transition hover:bg-rose-400/20">Interrupt</button>`, html.EscapeString(card.ID))
	}
	sync := ""
	if strings.TrimSpace(card.JobID) != "" && !isRunning {
		sync = fmt.Sprintf(`<button type="button" data-action="scrum#syncJob" data-card-id="%s" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-200 transition hover:border-cyan-300/40">Sync job</button>`, html.EscapeString(card.ID))
	}
	jobLine := ""
	if strings.TrimSpace(card.JobID) != "" {
		jobLine = fmt.Sprintf(`<span class="font-mono text-[11px] text-cyan-200/90">Job #%s</span>`, html.EscapeString(card.JobID))
	}
	submitLabel := "Send"
	if ctx.PilotPending {
		submitLabel = "Sending…"
	}
	placeholder := "Message uses this card's Config tab agent and models…"
	if isLive {
		placeholder = "Steer the running agent…"
	}
	messagesAttrs := fmt.Sprintf(`data-controller="chat-component" data-chat-component-id-value="%s" data-chat-component-endpoint-value="%s" data-chat-component-target="messages" data-scrum-channel-messages data-recyclr-sink="chat-%s-messages"`,
		html.EscapeString(componentID), html.EscapeString(chatEndpoint), html.EscapeString(componentID))
	return renderChannelSurfaceHTML(scrumChannelSurfaceOptions{
		Eyebrow:       "Card channel",
		Title:         card.Title,
		StatusHTML:    status,
		ActionsHTML:   jobLine + interrupt + sync,
		BadgeHTML:     fmt.Sprintf(`<span class="rounded-full border px-3 py-1 text-xs font-medium %s">%s</span>`, liveBadge.tone, html.EscapeString(liveBadge.label)),
		MessagesHTML:  renderScrumModalChannelMessagesHTML(card, ctx.PilotPending),
		MessagesAttrs: messagesAttrs,
		MessagesClass: "scrum-channel-scroll scrollbar min-h-0 flex-1 overflow-y-auto overflow-x-hidden flex flex-col gap-1.5 px-3 py-3 md:px-4",
		ComposerHTML: renderChatComposerHTML(scrumChannelComposerOptions{
			FormAction:    "submit->chat-component#send",
			KeydownAction: "chat-component#composerKeydown",
			CardID:        card.ID,
			ComponentID:   componentID,
			Endpoint:      chatEndpoint,
			Placeholder:   placeholder,
			Disabled:      ctx.PilotPending,
			SubmitLabel:   submitLabel,
		}),
	})
}

func renderScrumModalActiveTabHTML(ctx scrumModalRenderContext) string {
	var htmlBody string
	switch ctx.Tab {
	case "files":
		htmlBody = renderScrumModalFilesTabHTML(ctx.Card, ctx.Files, ctx.Dirs)
	case "tests":
		htmlBody = renderScrumModalTestsTabHTML(ctx.Card)
	case "config":
		htmlBody = renderScrumModalConfigTabHTML(ctx)
	case "recipe":
		htmlBody = renderScrumModalRecipeTabHTML(ctx)
	case "channel":
		htmlBody = renderScrumModalChannelTabHTML(ctx)
	default:
		htmlBody = renderScrumModalCardTabHTML(ctx.Card)
	}
	return fmt.Sprintf(`<div data-scrum-tab-panel="%s" data-recyclr-sink="scrum-modal-%s">%s</div>`, html.EscapeString(ctx.Tab), html.EscapeString(ctx.Tab), htmlBody)
}

func renderScrumCardModalInnerHTML(ctx scrumModalRenderContext) string {
	card := ctx.Card
	return fmt.Sprintf(`
    <div class="shrink-0 border-b border-white/10 p-4 md:p-5" data-scrum-modal-card-id="%s">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div class="font-mono text-xs text-cyan-200">%s</div>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">%s</h2>
        </div>
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Close</button>
      </div>
      <p data-scrum-modal-feedback class="mt-3 hidden rounded-md border px-3 py-2 text-xs leading-5" role="status" aria-live="polite"></p>
    </div>
    <div class="shrink-0" data-recyclr-sink="scrum-modal-toolbar">%s</div>
    <div class="shrink-0 border-b border-white/10 px-4 py-3 md:px-5" data-recyclr-sink="scrum-modal-tabs">
      <nav class="flex flex-wrap gap-2" aria-label="Card sections">%s</nav>
    </div>
    <div class="omni-modal-body scrollbar p-4 md:p-5" data-recyclr-sink="scrum-modal-active-tab">
      %s
    </div>
  `,
		html.EscapeString(card.ID),
		html.EscapeString(card.ID),
		html.EscapeString(card.Title),
		renderScrumModalToolbarHTML(card, ctx.Board, ctx.PlayQueue),
		renderScrumModalTabNavHTML(card, ctx.Tab),
		renderScrumModalActiveTabHTML(ctx),
	)
}

func renderScrumCardModalBundle(ctx scrumModalRenderContext) string {
	return renderRecyclrTemplateHTML("modal", renderScrumCardModalInnerHTML(ctx), "innerHTML")
}

func renderScrumCardModalPartialBundle(ctx scrumModalRenderContext) string {
	card := ctx.Card
	return renderRecyclrTemplateHTML("scrum-modal-toolbar", renderScrumModalToolbarHTML(card, ctx.Board, ctx.PlayQueue), "innerHTML") +
		renderRecyclrTemplateHTML("scrum-modal-tabs", fmt.Sprintf(`<nav class="flex flex-wrap gap-2" aria-label="Card sections">%s</nav>`, renderScrumModalTabNavHTML(card, ctx.Tab)), "innerHTML") +
		renderRecyclrTemplateHTML("scrum-modal-active-tab", renderScrumModalActiveTabHTML(ctx), "innerHTML")
}

func scrumModalColumnOptionsHTML(activeColumn string) string {
	activeColumn = normalizeScrumColumn(activeColumn)
	var b strings.Builder
	for _, column := range scrumColumns {
		selected := ""
		if column == activeColumn {
			selected = " selected"
		}
		b.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(column), selected, html.EscapeString(scrumColumnLabel(column))))
	}
	return b.String()
}

func renderScrumModalAssignCTAHTML(card ScrumCard) string {
	if normalizeScrumColumn(card.Column) == "assigned" {
		return ""
	}
	return fmt.Sprintf(`<button type="button" data-action="scrum#assignCard" data-card-id="%s" class="rounded-md border border-violet-300/40 bg-violet-300/15 px-3 py-1.5 text-xs font-semibold text-violet-100 transition hover:border-violet-200/50 hover:bg-violet-300/25">→ Assigned</button>`, html.EscapeString(card.ID))
}

func renderScrumModalPlayActionsHTML(card ScrumCard, playQueue map[string]any) string {
	playUnlocked := scrumPlayControlUnlocked(card)
	isRunning := card.PlayState == "running"
	isQueued := card.PlayState == "queued"
	hasActiveRunner := playQueueString(playQueue, "running_card_id") != ""
	playLabel := "Play"
	if hasActiveRunner && !isRunning {
		playLabel = "Queue"
	}
	playEnabledClass := "rounded-md bg-cyan-300 px-3 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-cyan-200"
	playDisabledClass := "cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60"
	pivotEnabledClass := "rounded-md border border-violet-300/30 bg-violet-300/10 px-3 py-1.5 text-xs font-semibold text-violet-100 hover:bg-violet-300/20"
	pivotDisabledClass := "cursor-not-allowed rounded-md border border-white/10 bg-zinc-900/80 px-3 py-1.5 text-xs font-semibold text-zinc-500 opacity-60"

	playButton := fmt.Sprintf(`<button type="button" disabled class="%s" title="Move card to Assigned to play">▶ %s</button>`, playDisabledClass, html.EscapeString(playLabel))
	if playUnlocked && !isQueued {
		playButton = fmt.Sprintf(`<button type="button" data-action="scrum#play" data-card-id="%s" class="%s">▶ %s</button>`, html.EscapeString(card.ID), playEnabledClass, html.EscapeString(playLabel))
	}
	pivotButton := ""
	if hasActiveRunner && !isRunning && !isQueued {
		if playUnlocked {
			pivotButton = fmt.Sprintf(`<button type="button" data-action="scrum#pivotPlay" data-card-id="%s" class="%s">Play now</button>`, html.EscapeString(card.ID), pivotEnabledClass)
		} else {
			pivotButton = fmt.Sprintf(`<button type="button" disabled class="%s" title="Move card to Assigned to play">Play now</button>`, pivotDisabledClass)
		}
	}
	pauseButton := fmt.Sprintf(`<button type="button" disabled class="%s" title="Move card to Assigned to play">Pause</button>`, playDisabledClass)
	if isRunning {
		pauseButton = fmt.Sprintf(`<button type="button" data-action="scrum#pausePlay" data-card-id="%s" class="rounded-md border border-amber-300/30 bg-amber-300/10 px-3 py-1.5 text-xs font-semibold text-amber-100 hover:bg-amber-300/20">Pause</button>`, html.EscapeString(card.ID))
	} else if playUnlocked {
		pauseButton = fmt.Sprintf(`<button type="button" disabled class="%s">Pause</button>`, playDisabledClass)
	}
	jumpQueueButton := ""
	if isQueued && hasActiveRunner {
		jumpQueueButton = fmt.Sprintf(`<button type="button" data-action="scrum#pivotPlay" data-card-id="%s" class="rounded-md border border-violet-300/30 px-3 py-1.5 text-xs text-violet-100 hover:bg-violet-300/10">Jump queue</button>`, html.EscapeString(card.ID))
	}
	return playButton + pivotButton + pauseButton + jumpQueueButton
}

type scrumChannelLiveBadge struct {
	label string
	tone  string
}

func scrumChannelLiveBadgeHTML(card ScrumCard) scrumChannelLiveBadge {
	switch card.PlayState {
	case "running":
		return scrumChannelLiveBadge{"streaming", "border-amber-300/30 bg-amber-300/10 text-amber-100"}
	case "reviewing":
		return scrumChannelLiveBadge{"reviewing", "border-cyan-300/30 bg-cyan-300/10 text-cyan-100"}
	case "queued":
		return scrumChannelLiveBadge{"queued", "border-violet-300/30 bg-violet-300/10 text-violet-100"}
	case "paused":
		return scrumChannelLiveBadge{"paused", "border-zinc-400/30 bg-zinc-400/10 text-zinc-300"}
	default:
		if strings.TrimSpace(card.JobID) != "" {
			return scrumChannelLiveBadge{"has job", "border-cyan-300/25 bg-cyan-300/10 text-cyan-100"}
		}
		return scrumChannelLiveBadge{"idle", "border-white/10 bg-white/[.04] text-zinc-400"}
	}
}

func scrumChannelSessionStatusHTML(card ScrumCard, playQueue map[string]any) string {
	switch card.PlayState {
	case "running":
		return `<span class="rounded-full border border-amber-300/30 bg-amber-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-200">Live</span>`
	case "queued":
		label := "Queued"
		if ids, ok := playQueue["queued_card_ids"].([]string); ok {
			for i, id := range ids {
				if id == card.ID {
					label = fmt.Sprintf("#%d in queue", i+1)
					break
				}
			}
		}
		return fmt.Sprintf(`<span class="rounded-full border border-violet-300/30 bg-violet-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-100">%s</span>`, html.EscapeString(label))
	case "paused":
		return `<span class="rounded-full border border-zinc-400/30 bg-zinc-400/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-zinc-300">Paused</span>`
	default:
		if strings.TrimSpace(card.JobID) != "" {
			return `<span class="rounded-full border border-cyan-300/25 bg-cyan-300/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-cyan-100">Has job</span>`
		}
		return ""
	}
}

func scrumModalTabBadgeHTML(card ScrumCard, tab string) string {
	pending := scrumModalChecklistPending(card)
	refs := card.RefFileCount
	if refs == 0 {
		refs = len(card.RefFiles)
	}
	chatCount := card.ChatCount
	if chatCount == 0 {
		chatCount = len(card.Chat)
	}
	planningCount := card.PlanningChatCount
	if planningCount == 0 {
		planningCount = len(card.PlanningChat)
	}
	hasCardTicket := card.HasCardTicket || strings.TrimSpace(card.CardTicket) != ""
	hasConfigOverrides := len(modelConfigStringMap(card.ModelConfig)) > 0 || len(agentConfigStringMap(card.AgentConfig)) > 0
	hasRecipe := strings.TrimSpace(card.RecipeID) != "" || len(jsonRawObjectMap(card.Recipe)) > 0
	isLive := card.PlayState == "running" || card.PlayState == "queued"

	switch tab {
	case "card":
		if planningCount > 0 {
			return scrumModalCountBadgeHTML(planningCount, "violet")
		}
		if pending > 0 {
			return scrumModalCountBadgeHTML(pending, "amber")
		}
		if hasCardTicket {
			return scrumModalDotBadgeHTML("emerald")
		}
	case "files":
		if refs > 0 {
			return scrumModalCountBadgeHTML(refs, "cyan")
		}
	case "tests":
		tests := card.TestCriteria
		pendingTests := 0
		for _, item := range tests {
			if !item.Done {
				pendingTests++
			}
		}
		if pendingTests > 0 {
			return scrumModalCountBadgeHTML(pendingTests, "amber")
		}
		if len(tests) > 0 {
			return scrumModalDotBadgeHTML("emerald")
		}
	case "config":
		if isLive {
			return scrumModalDotBadgeHTML("amber")
		}
		if hasConfigOverrides {
			return scrumModalDotBadgeHTML("cyan")
		}
	case "recipe":
		if hasRecipe {
			return scrumModalDotBadgeHTML("violet")
		}
	case "channel":
		if isLive {
			return scrumModalDotBadgeHTML("amber")
		}
		if chatCount > 0 {
			return scrumModalCountBadgeHTML(chatCount, "violet")
		}
		if strings.TrimSpace(card.ConsoleLog) != "" {
			return scrumModalDotBadgeHTML("cyan")
		}
	}
	return ""
}

func scrumModalChecklistPending(card ScrumCard) int {
	if card.ChecklistTotal > 0 {
		return maxInt(0, card.ChecklistTotal-card.ChecklistDone)
	}
	pending := 0
	for _, item := range card.Checklist {
		if !item.Done {
			pending++
		}
	}
	return pending
}

func scrumModalCountBadgeHTML(value int, tone string) string {
	if value <= 0 {
		return ""
	}
	tones := map[string]string{
		"cyan":   "border-cyan-300/30 bg-cyan-300/10 text-cyan-100",
		"violet": "border-violet-300/30 bg-violet-300/10 text-violet-100",
		"amber":  "border-amber-300/30 bg-amber-300/10 text-amber-100",
	}
	return fmt.Sprintf(`<span class="ml-1.5 inline-flex min-w-[1.25rem] items-center justify-center rounded-full border px-1.5 py-0.5 text-[10px] font-semibold %s">%d</span>`, tones[tone], value)
}

func scrumModalDotBadgeHTML(tone string) string {
	tones := map[string]string{
		"cyan":    "bg-cyan-300",
		"violet":  "bg-violet-300",
		"amber":   "bg-amber-300",
		"emerald": "bg-emerald-300",
	}
	return fmt.Sprintf(`<span class="ml-1.5 inline-flex h-2 w-2 rounded-full %s"></span>`, tones[tone])
}

func scrumStatusPillClass(status string) string {
	base := "rounded px-2 py-1 text-[11px] font-semibold uppercase tracking-[.14em]"
	switch normalizeScrumColumn(status) {
	case "done", "review":
		return base + " bg-emerald-300/15 text-emerald-200"
	case "in_progress", "running":
		return base + " bg-cyan-300/15 text-cyan-200"
	case "ready", "assigned":
		return base + " bg-amber-300/15 text-amber-200"
	case "blocked", "error":
		return base + " bg-rose-300/15 text-rose-200"
	default:
		return base + " bg-zinc-300/10 text-zinc-300"
	}
}

func scrumChecklistProgress(items []ScrumChecklistItem) (done, total int) {
	total = len(items)
	for _, item := range items {
		if item.Done {
			done++
		}
	}
	return done, total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
