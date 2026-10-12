import { requireJobDetails, type JobStatus } from "./chat_execution_contract";
import { requireWorkspaceRoot } from "./channel_api";
import { requireWorkspaceIdentity } from "./job_workspace_contract";

export type AgentChannelAuthority = Readonly<{
  channelID: string;
  jobID: number;
  userMessageID: number;
  generation: number;
  workspaceRoot: string;
  workspaceIdentity: string;
}>;

export type AgentChannelState = Readonly<{
  authority: AgentChannelAuthority;
  status: JobStatus;
  question?: string;
  waitingAttempt?: number;
  error?: string;
}>;

export function parseAgentChannelState(
  payload: Record<string, unknown>, channelID: string, jobID: number,
  expected?: AgentChannelAuthority,
  expectedMessageID?: number,
): AgentChannelState | undefined {
  const rawJob = record(payload.job, "Agent channel job");
  if (rawJob.id !== jobID) throw new Error("Channel job refresh changed the requested job.");
  const metadata = rawJob.metadata === undefined ? undefined : record(rawJob.metadata, "Agent channel metadata");
  if (metadata?.coding_execution === undefined) return undefined;
  const execution = record(metadata.coding_execution, "Agent coding execution");
  if (execution.mode !== "agents") throw new Error("Channel coding execution changed the frozen mode.");
  if (metadata.channel_id !== channelID || metadata.channel_mode !== "assistant") {
    throw new Error("Agent job belongs to a different conversation.");
  }
  const userMessageID = metadata.channel_user_message_id;
  if (typeof userMessageID !== "number" || !Number.isSafeInteger(userMessageID) || userMessageID < 1 ||
    expectedMessageID !== undefined && userMessageID !== expectedMessageID) {
    throw new Error("Agent job changed the originating channel message.");
  }
  const workspaceRoot = requireWorkspaceRoot(metadata.client_cwd, "Agent workspace");
  const workspaceIdentity = requireWorkspaceIdentity(metadata.client_workspace_identity, "Agent workspace identity");
  if (!workspaceIdentity.startsWith("client_")) {
    throw new Error("Agent job has no exact client workspace binding.");
  }
  const details = requireJobDetails(payload, jobID);
  const authority: AgentChannelAuthority = { channelID, jobID, userMessageID, generation: details.job.current_generation, workspaceRoot, workspaceIdentity };
  if (expected !== undefined && (authority.channelID !== expected.channelID || authority.jobID !== expected.jobID ||
    authority.userMessageID !== expected.userMessageID || authority.generation !== expected.generation || authority.workspaceRoot !== expected.workspaceRoot ||
    authority.workspaceIdentity !== expected.workspaceIdentity)) {
    throw new Error("Agent job generation or workspace changed; refresh the conversation before answering.");
  }
  if (details.job.status !== "waiting_input") return {
    authority, status: details.job.status,
    ...(typeof rawJob.error === "string" ? { error: rawJob.error } : {}),
  };
  const waiting = (payload.steps as unknown[]).map((value) => record(value, "Agent step"))
    .filter((step) => step.action === "v3_coding" && step.status === "waiting_input" && step.generation === authority.generation);
  if (waiting.length !== 1) throw new Error("Agent input has no exact waiting coding task.");
  const pause = record(payload.agent_pause, "Agent pause");
  if (pause.schema !== "omnidex-coding-agent-pause-v1" || pause.generation !== authority.generation || pause.step_id !== waiting[0].id) {
    throw new Error("Agent pause changed its exact generation or waiting task.");
  }
  const waitingAttempt = pause.waiting_attempt;
  if (typeof waitingAttempt !== "number" || !Number.isSafeInteger(waitingAttempt) || waitingAttempt < 1) {
    throw new Error("Agent question has no exact waiting attempt.");
  }
  if (!Array.isArray(pause.questions) || pause.questions.length === 0) throw new Error("Agent pause has no concrete questions.");
  const questions = pause.questions.map((question) => exactString(question, "Agent question"));
  return { authority, status: "waiting_input", waitingAttempt, question: questions.join("\n\n") };
}

function record(value: unknown, source: string): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error(`${source} must be one typed object.`);
  return value as Record<string, unknown>;
}

function exactString(value: unknown, source: string): string {
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${source} is missing.`);
  return value;
}
