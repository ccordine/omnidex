import { requireWorkspaceRoot } from "./channel_api";
import { requireJobDetails } from "./chat_execution_contract";

export type JobWorkspaceBinding = Readonly<{
  workspace_root?: string;
  workspace_identity?: string;
}>;

export function requireJobWorkspaceBinding(payload: Record<string, unknown>, jobID: number): JobWorkspaceBinding {
  requireJobDetails(payload, jobID);
  const job = payload.job as Record<string, unknown>;
  if (job.metadata === undefined) {
    if (job.pipeline === "coding") throw new Error("Coding job requires its frozen workspace binding.");
    return {};
  }
  if (job.metadata === null || typeof job.metadata !== "object" || Array.isArray(job.metadata)) {
    throw new Error("Job workspace metadata must be one typed object.");
  }
  const metadata = job.metadata as Record<string, unknown>;
  if (metadata.client_workspace_identity === undefined || metadata.client_workspace_identity === "") {
    const execution = metadata.coding_execution as { mode?: unknown } | undefined;
    if (job.pipeline === "coding" || execution?.mode === "agents") throw new Error("Coding job requires its frozen workspace binding.");
    return {};
  }
  return {
    workspace_root: requireWorkspaceRoot(metadata.client_cwd, "Job workspace"),
    workspace_identity: requireWorkspaceIdentity(metadata.client_workspace_identity, "Job workspace identity"),
  };
}

export function requireWorkspaceIdentity(value: unknown, source: string): string {
  if (typeof value !== "string") throw new Error(`${source} must be an exact directory or client identity.`);
  const parts = /^(?:client_[0-9a-f]{32}_)?directory_(0|[1-9][0-9]*)_([1-9][0-9]*)$/.exec(value);
  const maximum = (1n << 64n) - 1n;
  if (!parts || parts[1].length > 20 || parts[2].length > 20 || BigInt(parts[1]) > maximum || BigInt(parts[2]) > maximum) {
    throw new Error(`${source} must be an exact directory or client identity.`);
  }
  return value;
}
