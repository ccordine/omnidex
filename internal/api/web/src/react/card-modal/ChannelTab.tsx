import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { chatScrumCard, fetchScrumChannelPage } from "../../lib/scrum_api";
import { LifecycleOperationAttempt } from "../../lib/lifecycle_operation";
import type { ScrumChatMessage } from "../../lib/scrum_types";
import { ActionButton, EmptyState, Panel, submitForm, TextArea } from "./common";
import type { CardModalChildProps } from "./types";
import { ChannelMessage } from "./ChannelMessage";

const MAX_RECENT_MESSAGES = 100;

function messageKey(message: ScrumChatMessage, index: number): string {
  return message.id || `${message.role}:${message.created_at}:${message.content}:${index}`;
}

function mergeMessages(current: ScrumChatMessage[], incoming: ScrumChatMessage[], limit?: number): ScrumChatMessage[] {
  const merged = [...current];
  const indexes = new Map(merged.map((item, index) => [messageKey(item, index), index]));
  incoming.forEach((item, index) => {
    const key = messageKey(item, current.length + index);
    const existing = indexes.get(key);
    if (existing == null) {
      indexes.set(key, merged.length);
      merged.push(item);
      return;
    }
    merged[existing] = item;
  });
  return limit && merged.length > limit ? merged.slice(-limit) : merged;
}

export function ChannelTab({ context, projectID, mutationBusy, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const assemblyLineWorking = card.play_state === "running" || card.play_state === "queued";
  const [message, setMessage] = useState("");
  const [recentMessages, setRecentMessages] = useState<ScrumChatMessage[]>(card.chat);
  const [earlierMessages, setEarlierMessages] = useState<ScrumChatMessage[]>([]);
  const [beforeCursor, setBeforeCursor] = useState(context.channel_before_cursor);
  const [hasMore, setHasMore] = useState(context.channel_has_more);
  const [loadingEarlier, setLoadingEarlier] = useState(false);
  const [historyError, setHistoryError] = useState("");
  const [hasUnseenActivity, setHasUnseenActivity] = useState(false);
  const streamRef = useRef<HTMLDivElement | null>(null);
  const activeCardIDRef = useRef(card.id);
  const pinnedToBottomRef = useRef(true);
  const previousMessageCountRef = useRef(0);
  const historyAnchorRef = useRef<{ scrollHeight: number; scrollTop: number } | null>(null);
  const channelOperationAttemptRef = useRef<LifecycleOperationAttempt | null>(null);
  if (!channelOperationAttemptRef.current) channelOperationAttemptRef.current = new LifecycleOperationAttempt();
  const messages = useMemo(() => mergeMessages(earlierMessages, recentMessages), [earlierMessages, recentMessages]);

  useEffect(() => {
    if (activeCardIDRef.current === card.id) {
      const authoritativeWindow = card.chat;
      if (authoritativeWindow.length > MAX_RECENT_MESSAGES) {
        setHistoryError(`Authoritative channel window exceeds the ${MAX_RECENT_MESSAGES}-message browser bound.`);
        return;
      }
	  // The server cursor describes the boundary immediately before this exact
	  // recent window. Older pages loaded against a superseded boundary cannot
	  // be retained without creating a gap or overlap in durable history.
	  setEarlierMessages([]);
	  historyAnchorRef.current = null;
      setRecentMessages(authoritativeWindow);
	  setBeforeCursor(card.channel_before_cursor);
	  setHasMore(card.channel_has_more);
      return;
    }
    activeCardIDRef.current = card.id;
    setRecentMessages(card.chat);
    setEarlierMessages([]);
    setBeforeCursor(context.channel_before_cursor);
    setHasMore(context.channel_has_more);
    setHistoryError("");
    setHasUnseenActivity(false);
    pinnedToBottomRef.current = true;
    previousMessageCountRef.current = 0;
    historyAnchorRef.current = null;
  }, [card.chat, card.id, context.channel_before_cursor, context.channel_has_more]);

  useLayoutEffect(() => {
    const node = streamRef.current;
    if (!node) return;
    const anchor = historyAnchorRef.current;
    if (anchor) {
      node.scrollTop = anchor.scrollTop + (node.scrollHeight - anchor.scrollHeight);
      historyAnchorRef.current = null;
    } else if (pinnedToBottomRef.current || previousMessageCountRef.current === 0) {
      node.scrollTop = node.scrollHeight;
      setHasUnseenActivity(false);
    } else if (messages.length > previousMessageCountRef.current) {
      setHasUnseenActivity(true);
    }
    previousMessageCountRef.current = messages.length;
  }, [messages.length, card.id]);

  return (
    <Panel
      title="Card Channel"
      aside={assemblyLineWorking ? (
        <span className="inline-flex items-center gap-2 text-xs font-medium text-cyan-200" role="status" aria-live="polite">
          <span className="h-2 w-2 animate-pulse rounded-full bg-cyan-300" aria-hidden="true" />
          Assembly line working
        </span>
      ) : <span className="text-xs text-zinc-500">{card.play_state || "idle"}</span>}
    >
      <div className="flex h-[58vh] min-h-[28rem] flex-col gap-3">
        <div
          ref={streamRef}
          aria-label="Card channel activity"
          onScroll={(event) => {
            const node = event.currentTarget;
            const pinned = node.scrollHeight - node.scrollTop - node.clientHeight <= 80;
            pinnedToBottomRef.current = pinned;
            if (pinned) setHasUnseenActivity(false);
          }}
          className="scrollbar min-h-0 flex-1 space-y-3 overflow-y-auto rounded-md border border-white/10 bg-zinc-950/50 p-3"
        >
          {hasMore ? (
            <div className="flex justify-center pb-1">
              <button
                type="button"
                disabled={mutationBusy || loadingEarlier}
                onClick={async () => {
                  if (loadingEarlier || !beforeCursor) return;
                  const stream = streamRef.current;
                  if (!stream) throw new Error("Card channel scroll container is unavailable.");
                  historyAnchorRef.current = { scrollHeight: stream.scrollHeight, scrollTop: stream.scrollTop };
                  setLoadingEarlier(true);
                  setHistoryError("");
                  try {
                    const page = await fetchScrumChannelPage(card.id, beforeCursor, projectID);
                    setEarlierMessages((current) => mergeMessages(page.messages, current));
                    setBeforeCursor(page.before_cursor);
                    setHasMore(page.has_more);
                  } catch (error) {
                    historyAnchorRef.current = null;
                    setHistoryError(error instanceof Error ? error.message : String(error));
                  } finally {
                    setLoadingEarlier(false);
                  }
                }}
                className="inline-flex items-center gap-2 rounded border border-white/10 px-2.5 py-1 text-xs text-zinc-400 hover:border-cyan-300/30 hover:text-cyan-100 disabled:cursor-wait disabled:opacity-60"
              >
                {loadingEarlier ? <span className="h-3 w-3 animate-spin rounded-full border border-cyan-300/30 border-t-cyan-200" aria-hidden="true" /> : null}
                {loadingEarlier ? "Loading earlier activity…" : "Load earlier activity"}
              </button>
            </div>
          ) : null}
          {historyError ? <p className="rounded border border-rose-400/30 bg-rose-400/10 p-2 text-xs text-rose-100" role="alert">{historyError}</p> : null}
          {messages.length === 0 ? (
            <EmptyState>No channel messages yet.</EmptyState>
          ) : (
            messages.map((item, index) => <ChannelMessage key={item.id || `${item.created_at}-${index}`} message={item} />)
          )}
          {hasUnseenActivity ? (
            <button
              type="button"
              disabled={mutationBusy}
              onClick={() => {
                const stream = streamRef.current;
                if (!stream) throw new Error("Card channel scroll container is unavailable.");
                pinnedToBottomRef.current = true;
                stream.scrollTop = stream.scrollHeight;
                setHasUnseenActivity(false);
              }}
              className="sticky bottom-1 ml-auto block rounded-full border border-cyan-300/30 bg-zinc-950/95 px-3 py-1 text-xs font-medium text-cyan-100 shadow-lg"
            >
              New live activity ↓
            </button>
          ) : null}
        </div>
        <form
          onSubmit={submitForm(async () => {
            const submittedMessage = message;
            if (!submittedMessage.trim()) return;
            const attempt = channelOperationAttemptRef.current;
            if (!attempt) throw new Error("Card channel lifecycle attempt is unavailable.");
            const attemptKey = { scope: card.id, action: "chat", content: submittedMessage };
            const operationID = attempt.acquire(attemptKey);
            const payload = await runMutation(
              "Sending channel message",
              () => chatScrumCard(card.id, submittedMessage, operationID, projectID),
            );
            if (!payload?.card) return;
            onCardUpdated(payload.card, { reloadContext: true });
            if (attempt.confirm(attemptKey, operationID)) {
              setMessage((current) => current === submittedMessage ? "" : current);
            }
          })}
          className="flex gap-2"
        >
          <TextArea
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            onKeyDown={(event) => {
              if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                event.preventDefault();
                event.currentTarget.form?.requestSubmit();
              }
            }}
            rows={3}
            placeholder="Send revision to this job..."
            className="min-w-0 flex-1"
          />
          <ActionButton type="submit" tone="primary" disabled={mutationBusy}>Send</ActionButton>
        </form>
      </div>
    </Panel>
  );
}
