import { useState } from "react";
import { patchScrumCard, uploadScrumCardFiles } from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, Select } from "./common";
import type { CardModalChildProps } from "./types";

export function FilesTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const refs = card.ref_files ?? [];
  const [selectedRef, setSelectedRef] = useState("");
  const [uploadFiles, setUploadFiles] = useState<FileList | null>(null);
  const options = [...(context.dirs ?? []).map((path) => ({ path, label: `${path}/` })), ...(context.files ?? []).map((path) => ({ path, label: path }))];

  async function saveRefs(nextRefs: string[]) {
    const updated = await runMutation("Updating references", () => patchScrumCard(card.id, { ref_files: nextRefs }, projectID));
    if (updated) onCardUpdated(updated, { reloadContext: true });
  }

  return (
    <div className="space-y-4">
      <Panel title="Reference Files">
        {refs.length === 0 ? (
          <EmptyState>No references attached.</EmptyState>
        ) : (
          <div className="space-y-2">
            {refs.map((ref) => (
              <div key={ref} className="flex items-center gap-2 rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2 text-sm text-zinc-200">
                <span className="min-w-0 flex-1 truncate font-mono text-xs">{ref}</span>
                <ActionButton tone="danger" onClick={() => void saveRefs(refs.filter((entry) => entry !== ref))}>
                  Remove
                </ActionButton>
              </div>
            ))}
          </div>
        )}
        <div className="mt-3 flex flex-wrap gap-2">
          <Select value={selectedRef} onChange={(event) => setSelectedRef(event.target.value)} className="min-w-[16rem] flex-1">
            <option value="">Pick project file or directory...</option>
            {options.map((option) => (
              <option key={option.path} value={option.path}>
                {option.label}
              </option>
            ))}
          </Select>
          <ActionButton
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

      <Panel title="Upload Files">
        <div className="flex flex-wrap items-center gap-2">
          <input
            type="file"
            multiple
            onChange={(event) => setUploadFiles(event.target.files)}
            className="min-w-0 flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-xs text-zinc-100 file:mr-3 file:rounded file:border-0 file:bg-cyan-300 file:px-2 file:py-1 file:text-xs file:font-semibold file:text-zinc-950"
          />
          <ActionButton
            tone="primary"
            onClick={async () => {
              if (!uploadFiles || uploadFiles.length === 0) return;
              const payload = await runMutation("Uploading files", () => uploadScrumCardFiles(card.id, uploadFiles, projectID));
              if (payload?.card) onCardUpdated(payload.card, { reloadContext: true });
              setUploadFiles(null);
            }}
          >
            Upload
          </ActionButton>
        </div>
      </Panel>
    </div>
  );
}
