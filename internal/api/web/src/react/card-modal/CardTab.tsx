import { useEffect, useMemo, useState } from "react";
import {
	applyScrumCardElaboration,
	assembleScrumCardTicket,
  fetchScrumTags,
	mutateScrumCardItem,
  patchScrumCard,
} from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, submitForm, TextArea, TextInput } from "./common";
import type { CardModalChildProps } from "./types";

export function CardTab({ context, projectID, mutationBusy, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const [title, setTitle] = useState(card.title);
  const [description, setDescription] = useState(card.description);
  const [checklistText, setChecklistText] = useState("");
  const [tagInput, setTagInput] = useState("");
  const [tagSuggestions, setTagSuggestions] = useState<string[]>([]);
  const [tagCatalogLoading, setTagCatalogLoading] = useState(false);
  const [tagCatalogError, setTagCatalogError] = useState("");
  const [cardPrompt, setCardPrompt] = useState(card.card_prompt ?? "");
  const [elaboration, setElaboration] = useState("");
  const [ticket, setTicket] = useState(card.card_ticket ?? "");

  useEffect(() => {
    setTitle(card.title);
    setDescription(card.description);
    setCardPrompt(card.card_prompt ?? "");
    setElaboration("");
    setTicket(card.card_ticket ?? "");
  }, [card.id, card.updated_at, card.card_ticket, card.card_prompt]);

  useEffect(() => {
    let active = true;
    const timer = window.setTimeout(async () => {
      setTagCatalogLoading(true);
      setTagCatalogError("");
      try {
        const tags = await fetchScrumTags(tagInput, projectID);
        if (active) setTagSuggestions(tags);
      } catch (error) {
        if (active) {
          setTagSuggestions([]);
          setTagCatalogError(error instanceof Error ? error.message : String(error));
        }
      } finally {
        if (active) setTagCatalogLoading(false);
      }
    }, 200);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [tagInput, projectID]);

  const tags = useMemo(() => card.tags, [card.tags]);

  async function saveDetails() {
    const updated = await runMutation("Saving card", () => patchScrumCard(card.id, card.updated_at, { title: title.trim(), description }, projectID));
    if (updated) onCardUpdated(updated);
  }

	async function mutateChecklist(mutation: Parameters<typeof mutateScrumCardItem>[2]) {
		const updated = await runMutation("Updating checklist", () => mutateScrumCardItem(card.id, "checklist", mutation, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function patchTags(nextTags: string[]) {
    const updated = await runMutation("Updating tags", () => patchScrumCard(card.id, card.updated_at, { tags: nextTags }, projectID));
    if (updated) {
      onCardUpdated(updated);
      return true;
    }
    return false;
  }

  async function assembleTicket() {
		const updated = await runMutation("Assembling ticket", () =>
			assembleScrumCardTicket(card.id, card.updated_at, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function applyElaboration() {
		const updated = await runMutation("Applying elaboration", () =>
			applyScrumCardElaboration(card.id, card.updated_at, elaboration, projectID));
    if (updated) onCardUpdated(updated);
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
      <div className="space-y-4">
        <Panel title="Details">
          <div className="space-y-3">
            <TextInput value={title} onChange={(event) => setTitle(event.target.value)} className="w-full text-base font-semibold" />
            <TextArea rows={8} value={description} onChange={(event) => setDescription(event.target.value)} className="w-full" />
            <ActionButton tone="primary" onClick={saveDetails} disabled={mutationBusy || !title.trim()}>
              Save details
            </ActionButton>
          </div>
        </Panel>

        <Panel title="Checklist">
          <div className="space-y-2">
            {card.checklist.length === 0 ? (
              <EmptyState>No checklist items.</EmptyState>
            ) : (
              card.checklist.map((item) => (
                <label key={item.id} className="flex items-start gap-2 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2 text-sm text-zinc-200">
                  <input
                    type="checkbox"
                    checked={item.done}
					disabled={mutationBusy}
						onChange={(event) => void mutateChecklist({ action: "toggle", expected_updated_at: card.updated_at, item_id: item.id, done: event.target.checked })}
                    className="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"
                  />
                  <span className={item.done ? "line-through decoration-zinc-500" : ""}>{item.text}</span>
                  <button
                    type="button"
					disabled={mutationBusy}
						onClick={() => void mutateChecklist({ action: "remove", expected_updated_at: card.updated_at, item_id: item.id })}
                    className="ml-auto text-xs text-zinc-500 hover:text-rose-200"
                  >
                    Remove
                  </button>
                </label>
              ))
            )}
            <form
              onSubmit={submitForm(() => {
                if (!checklistText.trim()) return;
				void mutateChecklist({ action: "add", expected_updated_at: card.updated_at, text: checklistText });
                setChecklistText("");
              })}
              className="flex gap-2"
            >
              <TextInput value={checklistText} onChange={(event) => setChecklistText(event.target.value)} placeholder="Add checklist item" className="min-w-0 flex-1" />
              <ActionButton type="submit" disabled={mutationBusy}>Add</ActionButton>
            </form>
          </div>
        </Panel>

        <Panel title="Ticket">
          <div className="space-y-3">
			<p className="text-xs text-zinc-500">Ticket assembly deterministically formats the current saved card. Elaboration is explicit user-authored text; no model expands the ticket.</p>
            <label className="block space-y-1 text-xs text-zinc-400">
              <span>Manual ticket note (not execution context)</span>
              <TextArea rows={3} value={cardPrompt} onChange={(event) => setCardPrompt(event.target.value)} placeholder="Manual ticket note" className="w-full" />
            </label>
            <label className="block space-y-1 text-xs text-zinc-400">
              <span>New elaboration</span>
              <TextArea rows={3} value={elaboration} onChange={(event) => setElaboration(event.target.value)} placeholder="User-authored elaboration" className="w-full" />
            </label>
            <TextArea rows={12} value={ticket} onChange={(event) => setTicket(event.target.value)} placeholder="Ticket details" className="w-full font-mono text-xs" />
            <div className="flex flex-wrap gap-2">
              <ActionButton disabled={mutationBusy} tone="primary" onClick={() => void assembleTicket()}>
				Assemble ticket
              </ActionButton>
              <ActionButton onClick={() => void applyElaboration()} disabled={mutationBusy || !elaboration.trim()}>
				Apply elaboration
              </ActionButton>
              <ActionButton
                disabled={mutationBusy}
                onClick={async () => {
                  const updated = await runMutation("Saving ticket draft", () => patchScrumCard(card.id, card.updated_at, { card_prompt: cardPrompt, card_ticket: ticket }, projectID));
                  if (updated) onCardUpdated(updated);
                }}
              >
                Save ticket
              </ActionButton>
            </div>
          </div>
        </Panel>
      </div>

      <div className="space-y-4">
        <Panel title="Tags">
          <div className="flex flex-wrap gap-2">
            {tags.length === 0 ? <span className="text-xs text-zinc-500">No tags.</span> : null}
            {tags.map((tag) => (
              <button key={tag} type="button" disabled={mutationBusy} onClick={() => void patchTags(tags.filter((entry) => entry !== tag))} className="rounded-full border border-cyan-300/20 bg-cyan-300/10 px-2 py-1 text-xs text-cyan-100 disabled:cursor-not-allowed disabled:opacity-50">
                {tag} ×
              </button>
            ))}
          </div>
          <form
            onSubmit={submitForm(async () => {
              if (!tagInput.trim()) return;
              if (await patchTags([...tags, tagInput])) setTagInput("");
            })}
            className="mt-3 flex gap-2"
          >
            <TextInput value={tagInput} onChange={(event) => setTagInput(event.target.value)} list="react-card-tag-suggestions" placeholder="Search or add tag" className="min-w-0 flex-1" />
            <datalist id="react-card-tag-suggestions">
              {tagSuggestions.map((tag) => (
                <option key={tag} value={tag} />
              ))}
            </datalist>
            <ActionButton type="submit" disabled={mutationBusy}>Add</ActionButton>
          </form>
          {tagCatalogLoading ? (
            <p className="mt-2 inline-flex items-center gap-2 text-xs text-cyan-200" role="status" aria-live="polite">
              <span className="h-2 w-2 animate-pulse rounded-full bg-cyan-300" aria-hidden="true" />
              Loading tag catalog…
            </p>
          ) : null}
          {tagCatalogError ? <p className="mt-2 text-xs text-rose-200" role="alert">Tag catalog unavailable: {tagCatalogError}</p> : null}
        </Panel>

        <Panel title="State">
          <dl className="grid grid-cols-2 gap-2 text-xs">
            <dt className="text-zinc-500">Column</dt>
            <dd className="text-zinc-200">{card.column}</dd>
            <dt className="text-zinc-500">Play</dt>
            <dd className="text-zinc-200">{card.play_state || "idle"}</dd>
          </dl>
        </Panel>
      </div>
    </div>
  );
}
