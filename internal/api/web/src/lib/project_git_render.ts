import { escapeHTML } from "./dom";
import type { ProjectGitStatus } from "./project_types";

function gitBadge(label: string, value: string, tone: "cyan" | "emerald" | "amber" | "rose" | "zinc" = "cyan"): string {
  const tones: Record<string, string> = {
    cyan: "border-cyan-300/30 bg-cyan-300/10 text-cyan-200",
    emerald: "border-emerald-300/30 bg-emerald-300/10 text-emerald-200",
    amber: "border-amber-300/30 bg-amber-300/10 text-amber-200",
    rose: "border-rose-400/30 bg-rose-400/10 text-rose-200",
    zinc: "border-white/10 bg-zinc-900/60 text-zinc-300",
  };
  return `
    <span class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wide ${tones[tone]}">
      <span class="opacity-70">${escapeHTML(label)}</span>
      <span>${escapeHTML(value)}</span>
    </span>
  `;
}

function fileStatusTone(status: string): string {
  if (status.includes("?")) return "text-zinc-500";
  if (status.includes("U") || status.includes("A") && status.includes("A")) return "text-rose-300";
  if (status.startsWith("D") || status.endsWith("D")) return "text-amber-300";
  if (status.startsWith("M") || status.endsWith("M")) return "text-cyan-200";
  if (status.startsWith("A") || status.endsWith("A")) return "text-emerald-200";
  return "text-zinc-300";
}

export function renderProjectGitSection(projectID: number, git: ProjectGitStatus | null): string {
  if (!git) {
    return `
      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Git</h3>
            <p class="mt-1 text-xs text-zinc-500">Repository status for this project directory.</p>
          </div>
          <button type="button" data-action="projects#refreshProjectGit" data-project-id="${projectID}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Refresh</button>
        </div>
        <p class="mt-4 text-sm text-zinc-500">Git status not loaded yet.</p>
      </section>
    `;
  }

  if (!git.is_repo) {
    return `
      <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Git</h3>
              ${gitBadge("Not a repo", "—", "zinc")}
            </div>
            <p class="mt-1 text-xs text-zinc-500">${escapeHTML(git.message || "This directory is not a git repository.")}</p>
            ${git.error ? `<p class="mt-2 font-mono text-[11px] text-zinc-600">${escapeHTML(git.error)}</p>` : ""}
          </div>
          <button type="button" data-action="projects#refreshProjectGit" data-project-id="${projectID}" class="rounded-md border border-white/10 px-3 py-2 text-sm text-zinc-200 hover:border-cyan-300/40 hover:bg-cyan-300/10">Refresh</button>
        </div>
      </section>
    `;
  }

  const branchLabel = git.detached ? `detached @ ${git.head_short || "HEAD"}` : git.branch || git.head_short || "unknown";
  const cleanBadge = git.clean ? gitBadge("Clean", "✓", "emerald") : gitBadge("Dirty", "changes", "amber");
  const syncBadges: string[] = [];
  if (git.has_upstream) {
    if ((git.ahead ?? 0) > 0) syncBadges.push(gitBadge("Ahead", String(git.ahead), "cyan"));
    if ((git.behind ?? 0) > 0) syncBadges.push(gitBadge("Behind", String(git.behind), "amber"));
    if ((git.ahead ?? 0) === 0 && (git.behind ?? 0) === 0) syncBadges.push(gitBadge("Synced", "up to date", "emerald"));
  } else {
    syncBadges.push(gitBadge("Upstream", "none", "zinc"));
  }

  const statCards = [
    ["Staged", git.staged_count ?? 0],
    ["Modified", git.modified_count ?? 0],
    ["Untracked", git.untracked_count ?? 0],
    ["Deleted", git.deleted_count ?? 0],
    ["Conflicts", git.conflicted_count ?? 0],
    ["Stashes", git.stash_count ?? 0],
  ];

  const changedRows = (git.changed_files ?? [])
    .map((file) => {
      const status = file.status || `${file.index_status ?? " "}${file.worktree_status ?? " "}`;
      return `
        <li class="flex items-start gap-3 font-mono text-[11px]">
          <span class="shrink-0 ${fileStatusTone(status)}">${escapeHTML(status.trim() || "?")}</span>
          <span class="min-w-0 break-all text-zinc-300">${escapeHTML(file.path)}</span>
        </li>
      `;
    })
    .join("");

  const commitRows = (git.recent_commits ?? [])
    .map((commit) => {
      return `
        <li class="rounded-md border border-white/10 bg-zinc-900/50 px-3 py-2">
          <div class="flex flex-wrap items-center gap-2 text-[11px]">
            <span class="font-mono text-cyan-200">${escapeHTML(commit.hash)}</span>
            <span class="text-zinc-500">${escapeHTML(commit.relative_date || "")}</span>
          </div>
          <div class="mt-1 text-sm text-zinc-200">${escapeHTML(commit.subject)}</div>
          ${commit.author ? `<div class="mt-1 text-[11px] text-zinc-500">${escapeHTML(commit.author)}</div>` : ""}
        </li>
      `;
    })
    .join("");

  return `
    <section class="rounded-xl border border-white/10 bg-zinc-950/60 p-5">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-500">Git</h3>
            ${gitBadge("Branch", branchLabel, git.detached ? "amber" : "cyan")}
            ${cleanBadge}
            ${syncBadges.join("")}
          </div>
          <p class="mt-2 font-mono text-[11px] text-zinc-600">${escapeHTML(git.root || git.location || "")}</p>
          ${git.remote_url ? `<p class="mt-1 font-mono text-[11px] text-zinc-500">origin · ${escapeHTML(git.remote_url)}</p>` : ""}
          ${git.upstream_branch ? `<p class="mt-1 text-[11px] text-zinc-500">Tracking ${escapeHTML(git.upstream_branch)}</p>` : ""}
        </div>
        <button type="button" data-action="projects#refreshProjectGit" data-project-id="${projectID}" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950 hover:bg-cyan-200">Refresh</button>
      </div>

      <div class="mt-4 grid gap-3 sm:grid-cols-3 lg:grid-cols-6">
        ${statCards
          .map(
            ([label, value]) => `
          <div class="rounded-md border border-white/10 bg-zinc-900/60 px-3 py-2">
            <div class="text-[11px] uppercase tracking-wide text-zinc-500">${escapeHTML(String(label))}</div>
            <div class="mt-1 font-mono text-lg text-cyan-200">${value}</div>
          </div>
        `,
          )
          .join("")}
      </div>

      <div class="mt-5 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Working tree</h4>
          ${
            changedRows
              ? `<ul class="mt-2 space-y-1">${changedRows}</ul>`
              : `<p class="mt-2 text-sm text-zinc-500">No uncommitted changes.</p>`
          }
        </div>
        <div>
          <h4 class="text-[11px] font-semibold uppercase tracking-[.18em] text-zinc-500">Recent commits</h4>
          ${
            commitRows
              ? `<ul class="mt-2 space-y-2">${commitRows}</ul>`
              : `<p class="mt-2 text-sm text-zinc-500">No commits yet.</p>`
          }
        </div>
      </div>
    </section>
  `;
}
