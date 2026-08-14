import { useState } from "react";
import { mutateScrumCardItem } from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, submitForm, TextInput } from "./common";
import type { CardModalChildProps } from "./types";

export function TestsTab({ context, projectID, mutationBusy, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const tests = card.test_criteria;
  const [text, setText] = useState("");

	async function mutateTests(mutation: Parameters<typeof mutateScrumCardItem>[2]) {
		const updated = await runMutation("Updating tests", () => mutateScrumCardItem(card.id, "test-criteria", mutation, projectID));
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
				disabled={mutationBusy}
				onChange={(event) => void mutateTests({ action: "toggle", expected_updated_at: card.updated_at, item_id: item.id, done: event.target.checked })}
                className="mt-1 rounded border-white/20 bg-zinc-900 text-emerald-300"
              />
              <span className={item.done ? "line-through decoration-zinc-500" : ""}>{item.text}</span>
			  <button type="button" disabled={mutationBusy} onClick={() => void mutateTests({ action: "remove", expected_updated_at: card.updated_at, item_id: item.id })} className="ml-auto text-xs text-zinc-500 hover:text-rose-200 disabled:cursor-not-allowed disabled:opacity-50">
                Remove
              </button>
            </label>
          ))
        )}
        <form
          onSubmit={submitForm(() => {
            if (!text.trim()) return;
			void mutateTests({ action: "add", expected_updated_at: card.updated_at, text });
            setText("");
          })}
          className="flex gap-2"
        >
          <TextInput value={text} onChange={(event) => setText(event.target.value)} placeholder="e.g. go test ./internal/api passes" className="min-w-0 flex-1" />
          <ActionButton type="submit" disabled={mutationBusy}>Add</ActionButton>
        </form>
      </div>
    </Panel>
  );
}
