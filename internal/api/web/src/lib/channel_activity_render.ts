import { escapeHTML, formatTime } from "./dom";

export type ChannelActivity = {
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

export function parseChannelActivity(content: string): ChannelActivity | null {
  const trimmed = content.trim();
  if (!trimmed.startsWith("{")) return null;
  try {
    const payload = JSON.parse(trimmed) as ChannelActivity;
    if (!payload?.activity?.trim()) return null;
    return payload;
  } catch {
    return null;
  }
}

function activityStatusLabel(status?: string): string {
  const value = (status || "completed").toLowerCase();
  switch (value) {
    case "running":
      return "Running";
    case "failed":
      return "Failed";
    case "completed":
      return "Done";
    default:
      return value;
  }
}

function activityStatusClass(status?: string): string {
  const value = (status || "completed").toLowerCase();
  switch (value) {
    case "running":
      return "border-amber-300/30 bg-amber-300/10 text-amber-100";
    case "failed":
      return "border-rose-400/30 bg-rose-400/10 text-rose-200";
    default:
      return "border-emerald-400/25 bg-emerald-400/10 text-emerald-200";
  }
}

function activityKindLabel(activity: ChannelActivity): string {
  switch (activity.activity) {
    case "command":
      return "Command";
    case "file_change":
      return "File change";
    case "patch":
      return "Patch";
    case "tool_call":
      return activity.tool || "Tool";
    case "output":
      return "Output";
    default:
      return activity.title || "Activity";
  }
}

function renderDiffBlock(diff: string): string {
  const lines = diff.split("\n");
  const html = lines
    .map((line) => {
      let cls = "text-zinc-300";
      if (line.startsWith("+") && !line.startsWith("+++")) cls = "text-emerald-300";
      else if (line.startsWith("-") && !line.startsWith("---")) cls = "text-rose-300";
      else if (line.startsWith("@@")) cls = "text-cyan-200";
      else if (line.startsWith("+++") || line.startsWith("---")) cls = "text-zinc-500";
      return `<div class="font-mono text-[11px] leading-5 ${cls}">${escapeHTML(line)}</div>`;
    })
    .join("");
  return `<div class="channel-activity-diff scrollbar mt-2 max-h-56 overflow-auto rounded-md border border-white/10 bg-zinc-950/80 p-2">${html}</div>`;
}

function renderFileList(files: string[] = []): string {
  if (!files.length) return "";
  return `
    <ul class="mt-2 space-y-1">
      ${files
        .map(
          (file) =>
            `<li class="flex items-center gap-2 font-mono text-[11px] text-zinc-300"><span class="text-emerald-300">●</span>${escapeHTML(file)}</li>`,
        )
        .join("")}
    </ul>`;
}

export function renderChannelActivityMessage(activity: ChannelActivity, at: string): string {
  const status = activityStatusLabel(activity.status);
  const statusClass = activityStatusClass(activity.status);
  const kind = activityKindLabel(activity);
  const title = activity.title || kind;
  const commandBlock =
    activity.command?.trim()
      ? `<pre class="channel-activity-command mt-2 overflow-x-auto rounded-md border border-white/10 bg-zinc-950/80 p-2 font-mono text-[11px] leading-5 text-cyan-100">${escapeHTML(activity.command)}</pre>`
      : "";
  const pathLine = activity.path?.trim()
    ? `<div class="mt-1 font-mono text-[11px] text-zinc-400">${escapeHTML(activity.path)}</div>`
    : "";
  const detailBlock =
    activity.detail?.trim() && activity.activity !== "command"
      ? `<div class="mt-2 whitespace-pre-wrap text-xs leading-5 text-zinc-400">${escapeHTML(activity.detail)}</div>`
      : "";
  const diffBlock = activity.diff?.trim() ? renderDiffBlock(activity.diff) : "";

  return `
    <article class="message-grid message-tool">
      <div class="message-shell channel-activity-shell">
        <div class="message-meta">
          <span>${escapeHTML(kind)}</span>
          <time>${formatTime(at)}</time>
        </div>
        <div class="message-body">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-zinc-100">${escapeHTML(title)}</span>
            <span class="rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${statusClass}">${escapeHTML(status)}</span>
          </div>
          ${pathLine}
          ${renderFileList(activity.files)}
          ${commandBlock}
          ${detailBlock}
          ${diffBlock}
        </div>
      </div>
    </article>`;
}
