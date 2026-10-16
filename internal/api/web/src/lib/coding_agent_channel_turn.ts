import { readJSON } from "./api";
import { observeRealtimeJob, type RealtimeJobObservation } from "./realtime_job_observer";
import { parseAgentChannelState, type AgentChannelAuthority, type AgentChannelState } from "./coding_agent_channel_contract";
import type { ChannelTranscriptPage } from "./types";
import { LifecycleOperationAttempt } from "./lifecycle_operation";

export class CodingAgentChannelTurn {
  private readonly states = new Map<string, AgentChannelState>();
  private readonly observations = new Map<string, RealtimeJobObservation<AgentChannelState>>();
  private changed: () => void = () => undefined;
  private readonly planningOperations = new LifecycleOperationAttempt();

  constructor(private readonly failed: (channelID: string, error: unknown) => void) {}

  onChange(handler: () => void): void { this.changed = handler; }
  state(channelID: string): AgentChannelState | undefined { return this.states.get(channelID); }

  async continueConversation(channelID: string, text: string): Promise<number> {
    const state = this.states.get(channelID);
    if (!state || state.status !== "waiting_input") throw new Error("The conversation is not ready for another turn.");
    const previous = this.observations.get(channelID);
    if (previous) await previous.completion;
    if (state.inputDisposition === undefined) throw new Error("Coding reply lacks its server-owned disposition.");
    const planning = state.inputDisposition === "replan";
    const authority = state.authority;
    const key = { scope: JSON.stringify({ authority, inputID: state.executionInputID }), action: "conversation_turn", content: text };
    const operationID = this.planningOperations.acquire(key);
    const response = await fetch(`/v1/channels/${encodeURIComponent(channelID)}/session/turn`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operation_id: operationID, text, coding_mode: "agents",
        workspace_root: authority.workspaceRoot, workspace_identity: authority.workspaceIdentity,
        ...(state.executionInputID === undefined ? {} : { expected_input_id: state.executionInputID }) }),
    });
    const receipt = await readJSON<Record<string, unknown>>(response);
    if (response.status !== 202 || receipt.operation_id !== operationID || receipt.channel_id !== channelID ||
      receipt.job_id !== authority.jobID || receipt.disposition !== (planning ? "replanned" : "feedback_submitted") ||
      typeof receipt.generation !== "number" || !Number.isSafeInteger(receipt.generation) ||
      (planning ? receipt.generation <= authority.generation : receipt.generation !== authority.generation)) {
      throw new Error("The reply did not return its conversation authority.");
    }
    const current = await this.load(channelID, authority.jobID, { ...authority, generation: receipt.generation });
    if (!current) throw new Error("The conversation lost its coding configuration.");
    this.update(channelID, current);
    this.planningOperations.confirm(key, operationID);
    return authority.jobID;
  }

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
