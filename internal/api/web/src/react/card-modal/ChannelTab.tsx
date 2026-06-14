import { useEffect, useRef, useState } from "react";
import { chatScrumCard } from "../../lib/scrum_api";
import { ActionButton, EmptyState, Panel, shortDate, submitForm, TextArea } from "./common";
import type { CardModalChildProps } from "./types";

export function ChannelTab({ context, projectID, runMutation, onCardUpdated }: CardModalChildProps) {
  const card = context.card;
  const [message, setMessage] = useState("");
  const streamRef = useRef<HTMLDivElement | null>(null);
  const messages = card.chat ?? [];

  useEffect(() => {
    const node = streamRef.current;
    if (node) node.scrollTop = node.scrollHeight;
  }, [messages.length, card.id]);

  return (
    <Panel title="Card Channel" aside={<span className="text-xs text-zinc-500">{card.play_state || "idle"}</span>}>
      <div className="flex h-[58vh] min-h-[28rem] flex-col gap-3">
        <div ref={streamRef} className="scrollbar min-h-0 flex-1 space-y-3 overflow-y-auto rounded-md border border-white/10 bg-zinc-950/50 p-3">
          {messages.length === 0 ? (
            <EmptyState>No channel messages yet.</EmptyState>
          ) : (
            messages.map((item, index) => (
              <article key={item.id || `${item.created_at}-${index}`} className="rounded-md border border-white/10 bg-zinc-900/70 px-3 py-2">
                <div className="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-[.14em] text-zinc-500">
                  <span>{item.role}</span>
                  <span>{shortDate(item.created_at)}</span>
                </div>
                <p className="whitespace-pre-wrap text-sm leading-6 text-zinc-200">{item.content}</p>
              </article>
            ))
          )}
        </div>
        <form
          onSubmit={submitForm(async () => {
            if (!message.trim()) return;
            const payload = await runMutation("Sending channel message", () => chatScrumCard(card.id, message, projectID));
            if (payload?.card) onCardUpdated(payload.card, { reloadContext: true });
            setMessage("");
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
            placeholder="Steer this card..."
            className="min-w-0 flex-1"
          />
          <ActionButton type="submit" tone="primary">Send</ActionButton>
        </form>
      </div>
    </Panel>
  );
}
