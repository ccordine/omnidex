import { useState } from "react";
import { patchScrumCard } from "../../lib/scrum_api";
import type { ScrumTestCriterion } from "../../lib/scrum_types";
import { ActionButton, EmptyState, Panel, submitForm, TextInput } from "./common";
import type { CardModalChildProps } from "./types";

export function TestsTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const tests = card.test_criteria ?? [];
  const [text, setText] = useState("");

  async function patchTests(nextTests: ScrumTestCriterion[]) {
    const updated = await runMutation("Updating tests", () => patchScrumCard(card.id, { test_criteria: nextTests }, projectID));
    if (updated) onCardUpdated(updated);
  }

  return (
    <Panel title="Test Criteria">
      <div className="space-y-2">
        {tests.length === 0 ? (
          <EmptyState>No test criteria.</EmptyState>
        ) : (
          tests.map((item) => (
            <label key={item.id} className="flex items-start gap-2 rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2 text-sm text-zinc-200">
              <input
                type="checkbox"
                checked={item.done}
                onChange={(event) => void patchTests(tests.map((entry) => (entry.id === item.id ? { ...entry, done: event.target.checked } : entry)))}
                className="mt-1 rounded border-white/20 bg-zinc-900 text-emerald-300"
              />
              <span className={item.done ? "line-through decoration-zinc-500" : ""}>{item.text}</span>
              <button type="button" onClick={() => void patchTests(tests.filter((entry) => entry.id !== item.id))} className="ml-auto text-xs text-zinc-500 hover:text-rose-200">
                Remove
              </button>
            </label>
          ))
        )}
        <form
          onSubmit={submitForm(() => {
            if (!text.trim()) return;
            void patchTests([...tests, { id: `test_${Date.now()}`, text: text.trim(), done: false }]);
            setText("");
          })}
          className="flex gap-2"
        >
          <TextInput value={text} onChange={(event) => setText(event.target.value)} placeholder="e.g. go test ./internal/api passes" className="min-w-0 flex-1" />
          <ActionButton type="submit">Add</ActionButton>
        </form>
      </div>
    </Panel>
  );
}
