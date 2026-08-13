import { useEffect, useMemo, useState } from "react";
import {
  elaborateScrumCardTicket,
  fetchScrumTags,
  generateScrumCardTicket,
	mutateScrumCardItem,
  patchScrumCard,
} from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, submitForm, TextArea, TextInput } from "./common";
import type { CardModalChildProps } from "./types";

function normalizeTag(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/g, "-");
}

export function CardTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const [title, setTitle] = useState(card.title);
  const [description, setDescription] = useState(card.description ?? "");
  const [checklistText, setChecklistText] = useState("");
  const [tagInput, setTagInput] = useState("");
  const [tagSuggestions, setTagSuggestions] = useState<string[]>([]);
  const [cardPrompt, setCardPrompt] = useState(card.card_prompt ?? "");
  const [elaboration, setElaboration] = useState("");
  const [ticket, setTicket] = useState(card.card_ticket ?? "");

  useEffect(() => {
    setTitle(card.title);
    setDescription(card.description ?? "");
    setCardPrompt(card.card_prompt ?? "");
    setElaboration("");
    setTicket(card.card_ticket ?? "");
  }, [card.id, card.updated_at, card.card_ticket, card.card_prompt]);

  useEffect(() => {
    const timer = window.setTimeout(async () => {
      try {
        setTagSuggestions(await fetchScrumTags(tagInput, projectID));
      } catch {
        setTagSuggestions([]);
      }
    }, 200);
    return () => window.clearTimeout(timer);
  }, [tagInput, projectID]);

  const tags = useMemo(() => card.tags ?? [], [card.tags]);

  async function saveDetails() {
    const updated = await runMutation("Saving card", () => patchScrumCard(card.id, { title: title.trim(), description }, projectID));
    if (updated) onCardUpdated(updated);
  }

	async function mutateChecklist(mutation: Parameters<typeof mutateScrumCardItem>[2]) {
		const updated = await runMutation("Updating checklist", () => mutateScrumCardItem(card.id, "checklist", mutation, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function patchTags(nextTags: string[]) {
    const normalized = [...new Set(nextTags.map(normalizeTag).filter(Boolean))];
    const updated = await runMutation("Updating tags", () => patchScrumCard(card.id, { tags: normalized }, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function generateTicket() {
    const updated = await runMutation("Generating ticket", () =>
      generateScrumCardTicket(card.id, card.updated_at, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function elaborateTicket() {
    const updated = await runMutation("Elaborating ticket", () =>
      elaborateScrumCardTicket(card.id, card.updated_at, elaboration, projectID));
    if (updated) onCardUpdated(updated);
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
      <div className="space-y-4">
        <Panel title="Details">
          <div className="space-y-3">
            <TextInput value={title} onChange={(event) => setTitle(event.target.value)} className="w-full text-base font-semibold" />
            <TextArea rows={8} value={description} onChange={(event) => setDescription(event.target.value)} className="w-full" />
            <ActionButton tone="primary" onClick={saveDetails} disabled={!title.trim()}>
              Save details
            </ActionButton>
          </div>
        </Panel>

        <Panel title="Checklist">
          <div className="space-y-2">
            {(card.checklist ?? []).length === 0 ? (
              <EmptyState>No checklist items.</EmptyState>
            ) : (
              (card.checklist ?? []).map((item) => (
                <label key={item.id} className="flex items-start gap-2 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2 text-sm text-zinc-200">
                  <input
                    type="checkbox"
                    checked={item.done}
						onChange={(event) => void mutateChecklist({ action: "toggle", expected_updated_at: card.updated_at, item_id: item.id, done: event.target.checked })}
                    className="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"
                  />
                  <span className={item.done ? "line-through decoration-zinc-500" : ""}>{item.text}</span>
                  <button
                    type="button"
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
				void mutateChecklist({ action: "add", expected_updated_at: card.updated_at, text: checklistText.trim() });
                setChecklistText("");
              })}
              className="flex gap-2"
            >
              <TextInput value={checklistText} onChange={(event) => setChecklistText(event.target.value)} placeholder="Add checklist item" className="min-w-0 flex-1" />
              <ActionButton type="submit">Add</ActionButton>
            </form>
          </div>
        </Panel>

        <Panel title="Ticket">
          <div className="space-y-3">
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
              <ActionButton tone="primary" onClick={() => void generateTicket()}>
                Generate ticket
              </ActionButton>
              <ActionButton onClick={() => void elaborateTicket()} disabled={!elaboration.trim()}>
                Elaborate ticket
              </ActionButton>
              <ActionButton
                onClick={async () => {
                  const updated = await runMutation("Saving ticket draft", () => patchScrumCard(card.id, { card_prompt: cardPrompt, card_ticket: ticket }, projectID));
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
              <button key={tag} type="button" onClick={() => void patchTags(tags.filter((entry) => entry !== tag))} className="rounded-full border border-cyan-300/20 bg-cyan-300/10 px-2 py-1 text-xs text-cyan-100">
                {tag} ×
              </button>
            ))}
          </div>
          <form
            onSubmit={submitForm(() => {
              const tag = normalizeTag(tagInput);
              if (!tag || tags.includes(tag)) return;
              void patchTags([...tags, tag]);
              setTagInput("");
            })}
            className="mt-3 flex gap-2"
          >
            <TextInput value={tagInput} onChange={(event) => setTagInput(event.target.value)} list="react-card-tag-suggestions" placeholder="Search or add tag" className="min-w-0 flex-1" />
            <datalist id="react-card-tag-suggestions">
              {tagSuggestions.map((tag) => (
                <option key={tag} value={tag} />
              ))}
            </datalist>
            <ActionButton type="submit">Add</ActionButton>
          </form>
        </Panel>

        <Panel title="State">
          <dl className="grid grid-cols-2 gap-2 text-xs">
            <dt className="text-zinc-500">Column</dt>
            <dd className="text-zinc-200">{card.column}</dd>
            <dt className="text-zinc-500">Play</dt>
            <dd className="text-zinc-200">{card.play_state || "idle"}</dd>
            <dt className="text-zinc-500">Agent</dt>
            <dd className="text-zinc-200">{card.agent_config?.agent_system || context.agent_system || "omnidex"}</dd>
          </dl>
        </Panel>
      </div>
    </div>
  );
}
