import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { deleteScrumCard, doneScrumCard, fetchScrumCardModal, moveScrumCard, pauseScrumCard, playScrumCard, type ScrumCardMutationResult } from "../../lib/scrum_api";
import type { ScrumCard, ScrumCardModalResponse } from "../../lib/scrum_types";
import { closeModalShell } from "../../lib/modal";
import { scrumModalHref } from "../../lib/panel_routing";
import type { RealtimeSyncDetail } from "../../lib/realtime_sync";
import { validateScrumRealtimeCardUpdate } from "../../lib/scrum_realtime_response";
import { ActionButton, Select, SpinnerLabel } from "./common";
import { CardTab } from "./CardTab";
import { ChannelTab } from "./ChannelTab";
import { FilesTab } from "./FilesTab";
import { TestsTab } from "./TestsTab";
import { CARD_MODAL_TABS, normalizeCardModalTab, type CardModalTab, type RunMutation } from "./types";

export type CardModalAppProps = {
  cardID: string;
  projectID: number;
  initialTab?: string;
};

export function CardModalApp({ cardID, projectID, initialTab = "card" }: CardModalAppProps) {
  const [activeTab, setActiveTab] = useState<CardModalTab>(() => normalizeCardModalTab(initialTab));
  const [context, setContext] = useState<ScrumCardModalResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [busyLabel, setBusyLabel] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const mutationLockRef = useRef(false);

  const loadContext = useCallback(
    async (tab = activeTab, options: { silent?: boolean; strict?: boolean } = {}) => {
      if (!options.silent) {
        setLoading(true);
      }
      setError("");
      try {
        const payload = await fetchScrumCardModal(
          cardID,
          projectID,
          tab === "files" ? { tab, filePath: "", fileOffset: 0 } : { tab },
        );
        setContext(payload);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        if (options.strict) throw err;
      } finally {
        setLoading(false);
      }
    },
    [activeTab, cardID, projectID],
  );

  useEffect(() => {
    void loadContext(activeTab);
  }, [activeTab, cardID, projectID]);

  useEffect(() => {
    const handler = (event: Event) => {
      let detail;
      try {
        detail = validateScrumRealtimeCardUpdate((event as CustomEvent<unknown>).detail);
      } catch (validationError) {
        const message = validationError instanceof Error ? validationError.message : String(validationError);
        document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: projectID } }));
        void (async () => {
          try {
            await loadContext(activeTab, { silent: true, strict: true });
            setError(`${message} Authoritative card state was reloaded after rejecting the live update.`);
          } catch (reconcileError) {
            const reconcileMessage = reconcileError instanceof Error ? reconcileError.message : String(reconcileError);
            setError(`${message} Live-update reconciliation also failed: ${reconcileMessage}`);
          }
        })();
        return;
      }
      if (detail.projectID !== projectID || detail.cardID !== cardID) return;
      handleCardUpdated(detail.card, { refreshBoard: false });
    };
    const syncHandler = (event: Event) => {
      const detail = (event as CustomEvent<RealtimeSyncDetail>).detail;
      if (!detail || typeof detail.waitUntil !== "function") {
        throw new Error("Realtime synchronization event is missing waitUntil().");
      }
      detail.waitUntil(loadContext(activeTab, { silent: true, strict: true }));
    };
    document.addEventListener("omni:scrum-card-updated", handler);
    document.addEventListener("omni:realtime-sync-required", syncHandler);
    return () => {
      document.removeEventListener("omni:scrum-card-updated", handler);
      document.removeEventListener("omni:realtime-sync-required", syncHandler);
    };
  }, [activeTab, cardID, loadContext, projectID]);

  const refreshBoard = useCallback(() => {
    document.dispatchEvent(new CustomEvent("omni:scrum-refresh", { detail: { project_id: projectID } }));
  }, [projectID]);

  const runMutation: RunMutation = useCallback(async (label, fn) => {
    if (mutationLockRef.current) {
      setError("A card operation is already in progress.");
      return null;
    }
    mutationLockRef.current = true;
    setBusyLabel(label);
    setError("");
    setNotice("");
    try {
      const result = await fn();
      setNotice(`${label} complete`);
      return result;
    } catch (err) {
      const mutationError = err instanceof Error ? err.message : String(err);
      refreshBoard();
      try {
        await loadContext(activeTab, { silent: true, strict: true });
        setError(`${mutationError} Authoritative card state was reloaded after the failed response.`);
      } catch (reconcileError) {
        const detail = reconcileError instanceof Error ? reconcileError.message : String(reconcileError);
        setError(`${mutationError} Server reconciliation also failed: ${detail}`);
      }
      return null;
    } finally {
      mutationLockRef.current = false;
      setBusyLabel("");
    }
  }, [activeTab, loadContext, refreshBoard]);

  const handleCardUpdated = useCallback(
    (card: ScrumCard, options: { reloadContext?: boolean; refreshBoard?: boolean } = {}) => {
      setContext((current) => {
        if (!current) return current;
        const cards = current.board.cards.map((entry) => (entry.id === card.id ? card : entry));
        return { ...current, card, board: { ...current.board, cards } };
      });
      if (options.refreshBoard !== false) refreshBoard();
      if (options.reloadContext) {
        void loadContext(activeTab, { silent: true });
      }
    },
    [activeTab, loadContext, refreshBoard],
  );

  const childProps = useMemo(() => {
    if (!context) return null;
    return { context, projectID, mutationBusy: busyLabel !== "", runMutation, onCardUpdated: handleCardUpdated, onContextUpdated: setContext };
  }, [busyLabel, context, handleCardUpdated, projectID, runMutation]);

  function selectTab(tab: CardModalTab) {
    setActiveTab(tab);
    try {
      history.replaceState(null, document.title, scrumModalHref(cardID, tab));
    } catch {
      setError("Could not update card modal route");
    }
    document.dispatchEvent(new CustomEvent("omni:card-modal-tab-changed", { detail: { card_id: cardID, tab } }));
  }

  async function runHeaderAction(label: string, fn: () => Promise<ScrumCard | ScrumCardMutationResult | void>, options: { close?: boolean; reload?: boolean; refreshBoard?: boolean } = {}) {
    const result = await runMutation(label, fn);
    if (result === null) return;
    if (result && typeof result === "object" && "commit_state" in result) {
      handleCardUpdated(result.card);
      if (result.commit_state === "committed_degraded") {
        if (!result.operation_error) throw new Error("Degraded card mutation response lost its typed post-commit failure.");
        setError(result.operation_error);
      }
    } else if (result && typeof result === "object" && "id" in result) {
      handleCardUpdated(result, { reloadContext: options.reload });
    } else if (options.refreshBoard) {
      refreshBoard();
    }
    if (options.close) {
      closeModalShell();
    }
  }

  if (loading && !context) {
    return (
      <div className="flex min-h-[32rem] items-center justify-center">
        <SpinnerLabel label="Loading card..." />
      </div>
    );
  }

  if (!context || !childProps) {
    return (
      <div className="space-y-4 p-5">
        <p className="rounded-md border border-rose-400/30 bg-rose-400/10 px-3 py-2 text-sm text-rose-100">{error || "Card modal context unavailable"}</p>
        <ActionButton onClick={() => closeModalShell()}>Close</ActionButton>
      </div>
    );
  }

  const card = context.card;
  const columns = context.board.columns;
  const canPause = card.play_state === "running" || card.play_state === "queued";

  return (
    <div data-react-card-modal-card-id={card.id} className="flex max-h-[88vh] min-h-[32rem] flex-col">
      <header className="shrink-0 border-b border-white/10 p-4 md:p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs uppercase tracking-[.18em] text-zinc-500">Card</p>
            <h2 className="mt-1 truncate text-xl font-semibold text-zinc-100">{card.title}</h2>
            <p className="mt-1 text-xs text-zinc-500">
              {card.column} {card.play_state ? `· ${card.play_state}` : ""}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={card.column}
              disabled={busyLabel !== ""}
              onChange={(event) => void runHeaderAction("Moving card", () => moveScrumCard(card.id, event.target.value, card.updated_at, projectID), { reload: true })}
              className="max-w-[11rem]"
            >
              {columns.map((column) => (
                <option key={column} value={column}>
                  {column.replace(/_/g, " ")}
                </option>
              ))}
            </Select>
            {canPause ? (
              <ActionButton disabled={busyLabel !== ""} onClick={() => void runHeaderAction("Pausing play", () => pauseScrumCard(card.id, card.updated_at, projectID), { reload: true })}>Pause</ActionButton>
            ) : (
              <ActionButton disabled={busyLabel !== ""} tone="primary" onClick={() => void runHeaderAction("Queueing play", () => playScrumCard(card.id, card.updated_at, projectID, { pivot: false }), { reload: true })}>Play</ActionButton>
            )}
            <ActionButton disabled={busyLabel !== ""} tone="ok" onClick={() => void runHeaderAction("Marking done", () => doneScrumCard(card.id, card.updated_at, projectID), { reload: true })}>Done</ActionButton>
            <ActionButton
              disabled={busyLabel !== ""}
              tone="danger"
              onClick={() => {
                if (!window.confirm("Delete this scrum card?")) return;
                void runHeaderAction("Deleting card", () => deleteScrumCard(card.id, card.updated_at, projectID), { close: true, refreshBoard: true });
              }}
            >
              Delete
            </ActionButton>
            <ActionButton disabled={busyLabel !== ""} onClick={() => closeModalShell()}>Close</ActionButton>
          </div>
        </div>
        {busyLabel ? (
          <p className="mt-3 inline-flex items-center gap-2 rounded-md border border-cyan-300/30 bg-cyan-300/10 px-3 py-2 text-xs text-cyan-100" role="status" aria-live="polite">
            <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-cyan-300/25 border-t-cyan-200" aria-hidden="true" />
            {busyLabel}...
          </p>
        ) : null}
        {error ? <p className="mt-3 rounded-md border border-rose-400/30 bg-rose-400/10 px-3 py-2 text-xs text-rose-100">{error}</p> : null}
        {notice && !error ? <p className="mt-3 rounded-md border border-emerald-400/30 bg-emerald-400/10 px-3 py-2 text-xs text-emerald-100">{notice}</p> : null}
      </header>

      <nav className="shrink-0 border-b border-white/10 px-4 py-3 md:px-5" aria-label="Card sections">
        <div className="flex flex-wrap gap-2">
          {CARD_MODAL_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              disabled={busyLabel !== ""}
              onClick={() => selectTab(tab.id)}
              className={`rounded-md border px-3 py-1.5 text-xs font-medium transition ${
                activeTab === tab.id ? "border-cyan-300/40 bg-cyan-300/10 text-cyan-100" : "border-white/10 text-zinc-400 hover:border-cyan-300/30 hover:text-zinc-100"
              }`}
              aria-pressed={activeTab === tab.id}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </nav>

      <main className="omni-modal-body scrollbar min-h-0 flex-1 overflow-y-auto p-4 md:p-5">
        {activeTab === "card" ? <CardTab {...childProps} /> : null}
        {activeTab === "files" ? <FilesTab {...childProps} /> : null}
        {activeTab === "tests" ? <TestsTab {...childProps} /> : null}
        {activeTab === "channel" ? <ChannelTab {...childProps} /> : null}
      </main>
    </div>
  );
}
