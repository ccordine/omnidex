import { escapeHTML } from "./dom";
import { COLUMN_LABELS, SCRUM_COLUMNS, type ScrumCreateTicketConfig } from "./scrum_types";

export function renderScrumCreateCardModal(defaultColumn = "backlog", createTicket: ScrumCreateTicketConfig = { enabled: false, column: "backlog" }): string {
  const columnOptions = SCRUM_COLUMNS.map((col) => {
    const selected = col === defaultColumn ? " selected" : "";
    return `<option value="${escapeHTML(col)}"${selected}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`;
  }).join("");
  const createTicketColumn = createTicket.column || defaultColumn || "backlog";
  const ticketColumnOptions = ["backlog", "ready", "assigned"].map((col) => {
    const selected = col === createTicketColumn ? " selected" : "";
    return `<option value="${escapeHTML(col)}"${selected}>${escapeHTML(COLUMN_LABELS[col] ?? col)}</option>`;
  }).join("");
  return `
    <div class="border-b border-white/10 p-4 md:p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p class="text-xs uppercase tracking-[.20em] text-cyan-200/80">Scrum</p>
          <h2 class="mt-1 text-2xl font-semibold tracking-tight text-zinc-100">New card</h2>
        </div>
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-300">Cancel</button>
      </div>
    </div>
    <form data-action="submit->scrum#createCard" class="omni-modal-body scrollbar space-y-4 p-4 md:p-5">
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Title</span>
        <input data-scrum-field="newTitle" type="text" required autofocus placeholder="What needs doing?" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40" />
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Description</span>
        <textarea data-scrum-field="newDesc" rows="4" placeholder="Optional context for Omnidex" class="scrollbar mt-2 w-full resize-y rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-100 outline-none focus:border-cyan-300/40"></textarea>
      </label>
      <label class="block">
        <span class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-500">Column</span>
        <select data-scrum-field="newColumn" class="mt-2 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">${columnOptions}</select>
      </label>
      <section class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">
        <label class="flex cursor-pointer items-start gap-3 text-sm text-zinc-200">
          <input data-scrum-field="newCreateTicket" type="checkbox" class="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"${createTicket.enabled ? " checked" : ""} />
          <span>
            <span class="block font-medium text-zinc-100">Generate card ticket after create</span>
            <span class="mt-1 block text-xs leading-5 text-zinc-500">Queues a planning-mode agent job from the title and description. This checkbox and column are saved as your project preference.</span>
          </span>
        </label>
        <label class="mt-3 block">
          <span class="text-xs text-zinc-500">Insert new ticket card in</span>
          <select data-scrum-field="newCreateTicketColumn" class="mt-1 w-full rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none">${ticketColumnOptions}</select>
        </label>
      </section>
      <div class="flex justify-end gap-2 border-t border-white/10 pt-4">
        <button type="button" data-action="scrum#closeModal" class="rounded-md border border-white/10 px-4 py-2 text-sm text-zinc-300">Cancel</button>
        <button type="submit" data-scrum-submit="create" class="inline-flex items-center justify-center gap-2 rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200 disabled:cursor-not-allowed disabled:opacity-60">Create card</button>
      </div>
    </form>
  `;
}
