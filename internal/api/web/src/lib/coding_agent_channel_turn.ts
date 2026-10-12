import { jsonRequest, readJSON } from "./api";
import type { CodingMode } from "./channel_api";
import { authoritativeControlJobID } from "./chat_jobs_coordinator";
import { LifecycleOperationAttempt } from "./lifecycle_operation";
import { observeRealtimeJob, type RealtimeJobObservation } from "./realtime_job_observer";
import { parseAgentChannelState, type AgentChannelAuthority, type AgentChannelState } from "./coding_agent_channel_contract";
import type { ChannelTranscriptPage } from "./types";

export class CodingAgentChannelTurn {
  private readonly states = new Map<string, AgentChannelState>();
  private readonly observations = new Map<string, RealtimeJobObservation<AgentChannelState>>();
  private readonly operations = new LifecycleOperationAttempt();
  private changed: () => void = () => undefined;

  constructor(private readonly failed: (channelID: string, error: unknown) => void) {}

  onChange(handler: () => void): void { this.changed = handler; }
  state(channelID: string): AgentChannelState | undefined { return this.states.get(channelID); }

  async recover(channelID: string, turn: ChannelTranscriptPage["latest_user_turn"]): Promise<void> {
    const previous = this.states.get(channelID);
    if (!turn || previous && (previous.authority.jobID !== turn.job_id || previous.authority.userMessageID !== turn.message_id)) {
      this.retire(channelID);
    }
    if (!turn || this.observations.has(channelID)) return;
    const active = ["pending", "running", "waiting_input"].includes(turn.status);
    if (!active && !previous) return;
    const state = await this.load(channelID, turn.job_id, undefined, turn.message_id);
    if (!state) { this.retire(channelID); return; }
    this.update(channelID, state);
    if (state.status === "pending" || state.status === "running") {
      void this.watch(channelID, turn.job_id, state.authority).catch((error) => this.failed(channelID, error));
    }
  }

  async watch(channelID: string, jobID: number, expected?: AgentChannelAuthority, messageID?: number): Promise<void> {
    const existing = this.observations.get(channelID);
    if (existing) { await existing.completion; return; }
    let authority = expected;
    const observation = observeRealtimeJob<AgentChannelState>({
      jobID, stopOnWaitingInput: true,
      load: async () => {
        const state = await this.load(channelID, jobID, authority, messageID);
        if (!state) throw new Error("Accepted Agents job lost its frozen execution mode.");
        authority = state.authority;
        return { status: state.status, data: state, error: state.error };
      },
      onUpdate: (snapshot) => this.update(channelID, snapshot.data),
    });
    this.observations.set(channelID, observation);
    try { await observation.completion; }
    catch (error) { if (this.observations.get(channelID) === observation) throw error; }
    finally { if (this.observations.get(channelID) === observation) this.observations.delete(channelID); }
  }

  async answer(channelID: string, feedback: string, mode: CodingMode): Promise<number> {
    const pending = this.states.get(channelID);
    if (!pending || pending.status !== "waiting_input" || !pending.question || !pending.waitingAttempt) throw new Error("This conversation has no pending Agent question.");
    if (mode !== "agents") throw new Error("This job is using Agents; its mode cannot change while awaiting an answer.");
    const { authority } = pending;
    const current = await this.load(channelID, authority.jobID, authority);
    if (!current || current.status !== "waiting_input" && current.status !== "running") {
      throw new Error("The Agent question is no longer waiting; refresh the conversation.");
    }
    if (current.status === "waiting_input" && (current.question !== pending.question || current.waitingAttempt !== pending.waitingAttempt)) {
      throw new Error("The Agent question changed; refresh the conversation before answering.");
    }
    const attempt = {
      scope: JSON.stringify([authority.jobID, authority.generation, pending.waitingAttempt, pending.question]),
      action: "feedback", content: JSON.stringify(feedback),
    };
    const operationID = this.operations.acquire(attempt);
    const response = await fetch(`/v1/jobs/${authority.jobID}/feedback`, jsonRequest({
      operation_id: operationID, feedback, workspace_root: authority.workspaceRoot,
      workspace_identity: authority.workspaceIdentity, expected_generation: authority.generation,
      expected_waiting_attempt: pending.waitingAttempt,
    }));
    const receipt = await readJSON<Record<string, unknown>>(response);
    if (response.status !== 200) throw new Error("Agent answer was not accepted.");
    authoritativeControlJobID(receipt, String(authority.jobID), operationID);
    if (receipt.status !== "running") throw new Error("Agent answer did not resume the same job.");
    this.operations.confirm(attempt, operationID);
    this.update(channelID, { authority, status: "running" });
    return authority.jobID;
  }

  disconnect(): void {
    for (const observation of this.observations.values()) observation.cancel("Conversation disconnected.");
    this.observations.clear();
  }

  private async load(channelID: string, jobID: number, expected?: AgentChannelAuthority, messageID?: number): Promise<AgentChannelState | undefined> {
    const response = await fetch(`/v1/jobs/${jobID}`);
    const payload = await readJSON<Record<string, unknown>>(response);
    if (response.status !== 200) throw new Error("Agent job could not be refreshed.");
    return parseAgentChannelState(payload, channelID, jobID, expected, messageID);
  }

  private update(channelID: string, state: AgentChannelState): void {
    if (state.status === "completed" || state.status === "failed" || state.status === "canceled") this.states.delete(channelID);
    else this.states.set(channelID, state);
    this.changed();
  }

  private retire(channelID: string): void {
    const observation = this.observations.get(channelID);
    this.observations.delete(channelID);
    observation?.cancel("Conversation moved to a different turn.");
    if (this.states.delete(channelID)) this.changed();
  }
}
