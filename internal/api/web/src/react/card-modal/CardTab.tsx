import { useEffect, useMemo, useState } from "react";
import { cardTicketScrumCard, coachScrumCard, fetchScrumTags, patchScrumCard, suggestScrumTags, updateScrumCoachConfig } from "../../lib/scrum_api";
import type { ScrumChecklistItem, ScrumCoachSuggestion, ScrumCard } from "../../lib/scrum_types";
import { ActionButton, EmptyState, Panel, Select, submitForm, TextArea, TextInput } from "./common";
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
  const [iterateNotes, setIterateNotes] = useState("");
  const [ticket, setTicket] = useState(card.card_ticket ?? "");
  const [coachMessage, setCoachMessage] = useState("");
  const [coachSuggestions, setCoachSuggestions] = useState<ScrumCoachSuggestion[]>([]);
  const [coachEnabled, setCoachEnabled] = useState(card.coach_config?.enabled !== false);
  const [coachAutoScan, setCoachAutoScan] = useState(Boolean(card.coach_config?.auto_scan));
  const [coachModel, setCoachModel] = useState(card.coach_config?.model ?? "");

  useEffect(() => {
    setTitle(card.title);
    setDescription(card.description ?? "");
    setCardPrompt(card.card_prompt ?? "");
    setTicket(card.card_ticket ?? "");
    setCoachEnabled(card.coach_config?.enabled !== false);
    setCoachAutoScan(Boolean(card.coach_config?.auto_scan));
    setCoachModel(card.coach_config?.model ?? "");
  }, [card.id, card.updated_at, card.card_ticket, card.card_prompt, card.coach_config]);

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

  async function patchChecklist(checklist: ScrumChecklistItem[]) {
    const updated = await runMutation("Updating checklist", () => patchScrumCard(card.id, { checklist }, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function patchTags(nextTags: string[]) {
    const normalized = [...new Set(nextTags.map(normalizeTag).filter(Boolean))];
    const updated = await runMutation("Updating tags", () => patchScrumCard(card.id, { tags: normalized }, projectID));
    if (updated) onCardUpdated(updated);
  }

  async function queueTicket(iterate: boolean) {
    const payload = await runMutation(iterate ? "Queueing ticket iteration" : "Queueing ticket draft", () =>
      {
        if (iterate && !ticket.trim()) throw new Error("Add a ticket draft to iterate on first");
        return cardTicketScrumCard(
          card.id,
          iterate
            ? { card_prompt: cardPrompt, ticket, iterate: true, iterate_notes: iterateNotes }
            : { prompt: cardPrompt, card_prompt: cardPrompt },
          projectID,
        );
      },
    );
    if (payload?.card) onCardUpdated(payload.card, { reloadContext: true });
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
                    onChange={(event) => patchChecklist((card.checklist ?? []).map((entry) => (entry.id === item.id ? { ...entry, done: event.target.checked } : entry)))}
                    className="mt-1 rounded border-white/20 bg-zinc-900 text-cyan-300"
                  />
                  <span className={item.done ? "line-through decoration-zinc-500" : ""}>{item.text}</span>
                  <button
                    type="button"
                    onClick={() => patchChecklist((card.checklist ?? []).filter((entry) => entry.id !== item.id))}
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
                void patchChecklist([...(card.checklist ?? []), { id: `chk_${Date.now()}`, text: checklistText.trim(), done: false }]);
                setChecklistText("");
              })}
              className="flex gap-2"
            >
              <TextInput value={checklistText} onChange={(event) => setChecklistText(event.target.value)} placeholder="Add checklist item" className="min-w-0 flex-1" />
              <ActionButton type="submit">Add</ActionButton>
            </form>
          </div>
        </Panel>

        <Panel title="Ticket Draft">
          <div className="space-y-3">
            <TextArea rows={3} value={cardPrompt} onChange={(event) => setCardPrompt(event.target.value)} placeholder="Card prompt" className="w-full" />
            <TextArea rows={2} value={iterateNotes} onChange={(event) => setIterateNotes(event.target.value)} placeholder="Iteration notes" className="w-full" />
            <TextArea rows={12} value={ticket} onChange={(event) => setTicket(event.target.value)} placeholder="Generated ticket draft" className="w-full font-mono text-xs" />
            <div className="flex flex-wrap gap-2">
              <ActionButton onClick={() => void queueTicket(false)}>Generate</ActionButton>
              <ActionButton onClick={() => void queueTicket(true)}>Iterate</ActionButton>
              <ActionButton
                tone="primary"
                onClick={async () => {
                  const updated = await runMutation("Saving ticket draft", () => patchScrumCard(card.id, { card_prompt: cardPrompt, card_ticket: ticket } as Partial<ScrumCard>, projectID));
                  if (updated) onCardUpdated(updated);
                }}
              >
                Save draft
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
          <div className="mt-3">
            <ActionButton
              onClick={async () => {
                const payload = await runMutation("Queueing tag suggestions", () => suggestScrumTags(card.id, projectID));
                if (payload?.card) onCardUpdated(payload.card, { reloadContext: true });
              }}
            >
              Suggest tags
            </ActionButton>
          </div>
        </Panel>

        <Panel title="Coach">
          <div className="space-y-3">
            <label className="flex items-center gap-2 text-sm text-zinc-300">
              <input type="checkbox" checked={coachEnabled} onChange={(event) => setCoachEnabled(event.target.checked)} className="rounded border-white/20 bg-zinc-900 text-cyan-300" />
              Enabled
            </label>
            <label className="flex items-center gap-2 text-sm text-zinc-300">
              <input type="checkbox" checked={coachAutoScan} onChange={(event) => setCoachAutoScan(event.target.checked)} className="rounded border-white/20 bg-zinc-900 text-cyan-300" />
              Auto-scan
            </label>
            <label className="block space-y-1 text-xs text-zinc-400">
              <span>Coach model</span>
              <TextInput value={coachModel} onChange={(event) => setCoachModel(event.target.value)} className="w-full font-mono text-xs" />
            </label>
            <ActionButton
              onClick={async () => {
                const payload = await runMutation("Saving coach settings", () => updateScrumCoachConfig(card.id, { enabled: coachEnabled, auto_scan: coachAutoScan, model: coachModel }, projectID));
                if (payload?.card) onCardUpdated(payload.card);
              }}
              disabled={!coachModel.trim()}
            >
              Save coach
            </ActionButton>
            {coachSuggestions.length > 0 ? (
              <div className="space-y-2">
                {coachSuggestions.map((suggestion, index) => (
                  <p key={`${suggestion.text}-${index}`} className="rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2 text-xs text-zinc-300">
                    {suggestion.text}
                  </p>
                ))}
              </div>
            ) : null}
            <form
              onSubmit={submitForm(async () => {
                if (!coachMessage.trim()) return;
                const payload = await runMutation("Coach thinking", () =>
                  coachScrumCard(card.id, { message: coachMessage, snapshot: { title, description } }, projectID),
                );
                if (payload?.card) onCardUpdated(payload.card, { reloadContext: true });
                setCoachSuggestions(payload?.suggestions ?? []);
                setCoachMessage("");
              })}
              className="space-y-2"
            >
              <TextArea rows={3} value={coachMessage} onChange={(event) => setCoachMessage(event.target.value)} placeholder="Talk to the coach..." className="w-full" />
              <ActionButton type="submit">Send</ActionButton>
            </form>
          </div>
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
