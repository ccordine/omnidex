import { useState } from "react";
import { fetchScrumCardFilePage, patchScrumCard } from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, Select } from "./common";
import type { CardModalChildProps } from "./types";

export function FilesTab({ context, projectID, mutationBusy, runMutation, onCardUpdated, onContextUpdated }: CardModalChildProps) {
  const card = context.card;
  const refs = card.ref_files;
  const [selectedRef, setSelectedRef] = useState("");
  const options = [...context.dirs.map((path) => ({ path, label: `${path}/` })), ...context.files.map((path) => ({ path, label: path }))];

  async function saveRefs(nextRefs: string[]) {
    const updated = await runMutation("Updating references", () => patchScrumCard(card.id, card.updated_at, { ref_files: nextRefs }, projectID));
    if (updated) onCardUpdated(updated, { reloadContext: true });
  }

  async function loadFilePage(path: string, offset = 0) {
    const page = await runMutation("Loading project files", () => fetchScrumCardFilePage(card.id, projectID, path, offset));
    if (page) onContextUpdated({ ...context, ...page });
  }

  return (
    <div className="space-y-4">
      <Panel title="Project Files">
        <div className="flex flex-wrap items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-zinc-400">/{context.file_path}</span>
          <ActionButton
            disabled={mutationBusy || !context.file_has_parent}
            onClick={() => void loadFilePage(context.file_parent, 0)}
          >
            Up
          </ActionButton>
          <ActionButton
            disabled={mutationBusy || !context.file_has_previous}
            onClick={() => void loadFilePage(context.file_path, context.file_previous_offset)}
          >
            Previous
          </ActionButton>
          <ActionButton
            disabled={mutationBusy || !context.file_has_more}
            onClick={() => void loadFilePage(context.file_path, context.file_next_offset)}
          >
            Next
          </ActionButton>
        </div>
        {context.dirs.length > 0 ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {context.dirs.map((path) => (
              <ActionButton disabled={mutationBusy} key={path} onClick={() => void loadFilePage(path, 0)}>
                {path}/
              </ActionButton>
            ))}
          </div>
        ) : null}
      </Panel>

      <Panel title="Reference Files">
        {refs.length === 0 ? (
          <EmptyState>No references attached.</EmptyState>
        ) : (
          <div className="space-y-2">
            {refs.map((ref) => (
              <div key={ref} className="flex items-center gap-2 rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2 text-sm text-zinc-200">
                <span className="min-w-0 flex-1 truncate font-mono text-xs">{ref}</span>
                <ActionButton disabled={mutationBusy} tone="danger" onClick={() => void saveRefs(refs.filter((entry) => entry !== ref))}>
                  Remove
                </ActionButton>
              </div>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap gap-2">
          <Select disabled={mutationBusy} value={selectedRef} onChange={(event) => setSelectedRef(event.target.value)} className="min-w-[16rem] flex-1">
            <option value="">Pick project file or directory...</option>
            {options.map((option) => (
              <option key={option.path} value={option.path}>
                {option.label}
              </option>
            ))}
          </Select>
          <ActionButton
            disabled={mutationBusy}
            onClick={() => {
              if (!selectedRef || refs.includes(selectedRef)) return;
              void saveRefs([...refs, selectedRef]);
              setSelectedRef("");
            }}
          >
            Attach
          </ActionButton>
        </div>
      </Panel>
    </div>
  );
}
