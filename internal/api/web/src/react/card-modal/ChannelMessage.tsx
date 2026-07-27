import type { ScrumChatMessage } from "../../lib/scrum_types";
import { shortDate } from "./common";

type ChannelActivity = {
  activity: string;
  title?: string;
  status?: string;
  command?: string;
  tool?: string;
  path?: string;
  files?: string[];
  detail?: string;
  diff?: string;
};

function parseActivity(message: ScrumChatMessage): ChannelActivity | null {
  if (message.role !== "tool" || !message.content.trim().startsWith("{")) return null;
  try {
    const value = JSON.parse(message.content) as Partial<ChannelActivity>;
    if (!value || typeof value !== "object" || typeof value.activity !== "string" || !value.activity.trim()) return null;
    return { ...value, activity: value.activity.trim() } as ChannelActivity;
  } catch {
    return null;
  }
}

function activityLabel(activity: ChannelActivity): string {
  if (activity.activity === "command") return "Command";
  if (activity.activity === "file_change") return "File change";
  if (activity.activity === "patch") return "Patch";
  if (activity.activity === "output") return "Output";
  return activity.tool?.trim() || "Tool";
}

function statusClass(status = ""): string {
  if (status === "failed") return "border-rose-400/30 bg-rose-400/10 text-rose-200";
  if (status === "running") return "border-cyan-300/30 bg-cyan-300/10 text-cyan-100";
  return "border-emerald-400/30 bg-emerald-400/10 text-emerald-100";
}

export function ChannelMessage({ message }: { message: ScrumChatMessage }) {
  const activity = parseActivity(message);
  if (!activity) {
    const tone = message.role === "user"
      ? "border-cyan-300/20 bg-cyan-300/5 text-cyan-50"
      : message.role === "error"
        ? "border-rose-400/30 bg-rose-400/10 text-rose-100"
        : message.role === "thinking"
          ? "border-white/5 bg-white/[.02] italic text-zinc-500"
          : "border-white/10 bg-zinc-900/70 text-zinc-200";
    return (
      <article className={`rounded-md border px-3 py-2 ${tone}`}>
        <div className="mb-1 flex items-center justify-between gap-2 text-[11px] uppercase tracking-[.14em] text-zinc-500">
          <span>{message.role}</span>
          <span>{shortDate(message.created_at)}</span>
        </div>
        <p className="whitespace-pre-wrap text-sm leading-6">{message.content}</p>
      </article>
    );
  }

  const hasDetails = Boolean(activity.command || activity.detail || activity.diff || activity.files?.length);
  return (
    <article className="rounded-md border border-white/10 bg-zinc-900/70 px-3 py-2">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-[11px] uppercase tracking-[.14em] text-zinc-500">{activityLabel(activity)}</div>
          <div className="mt-1 text-sm font-medium text-zinc-100">{activity.title?.trim() || activityLabel(activity)}</div>
          {activity.path ? <div className="mt-1 truncate font-mono text-[11px] text-zinc-400">{activity.path}</div> : null}
        </div>
        <div className="flex items-center gap-2">
          <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase ${statusClass(activity.status)}`}>
            {activity.status || "completed"}
          </span>
          <span className="text-[11px] text-zinc-500">{shortDate(message.created_at)}</span>
        </div>
      </div>
      {hasDetails ? (
        <details className="mt-2 rounded border border-white/10 bg-zinc-950/40 px-2 py-1.5 text-xs text-zinc-300">
          <summary className="cursor-pointer select-none font-medium text-zinc-400 hover:text-cyan-100">Details</summary>
          <div className="mt-2 space-y-2">
            {activity.files?.length ? <div className="font-mono text-[11px] text-zinc-400">{activity.files.join(" · ")}</div> : null}
            {activity.command ? <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-[11px] leading-5 text-cyan-100">{activity.command}</pre> : null}
            {activity.detail ? <pre className="overflow-x-auto whitespace-pre-wrap text-xs leading-5 text-zinc-300">{activity.detail}</pre> : null}
            {activity.diff ? <pre className="max-h-64 overflow-auto whitespace-pre font-mono text-[11px] leading-5 text-zinc-300">{activity.diff}</pre> : null}
          </div>
        </details>
      ) : null}
    </article>
  );
}
