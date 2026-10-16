import { afterEach, describe, expect, it, vi } from "vitest";
import { CodingAgentChannelTurn } from "./coding_agent_channel_turn";
import { parseAgentChannelState } from "./coding_agent_channel_contract";

const workspaceIdentity = "client_0123456789abcdef0123456789abcdef_directory_1_42";

function jobDetails(status = "running", generation = 3, waitingAction = "v3_coding_plan") {
  return {
    job: {
      id: 17, status, current_generation: generation,
      metadata: {
        channel_id: "chat-42", channel_user_message_id: 11, channel_mode: "assistant", client_cwd: "/workspace/project",
        client_workspace_identity: workspaceIdentity,
        coding_execution: { mode: "agents" },
      },
    },
    steps: [{ id: 29, generation, action: status === "waiting_input" ? waitingAction : "v3_coding", status }],
    ...(status === "waiting_input" ? { coding_input_disposition: waitingAction === "v3_coding_plan" ? "replan" : "reply" } : {}),
  };
}

function response(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "Content-Type": "application/json" } });
}

describe("Agents conversation observation", () => {
  const turns: CodingAgentChannelTurn[] = [];
  afterEach(() => { turns.forEach((turn) => turn.disconnect()); turns.length = 0; vi.restoreAllMocks(); });

  function create() {
    const failed = vi.fn();
    const turn = new CodingAgentChannelTurn(failed);
    const changed = vi.fn();
    turn.onChange(changed);
    turns.push(turn);
    return { turn, changed };
  }

  it("recovers the exact active Agents job from the transcript backlink", async () => {
    const fetchMock = vi.fn(async () => response(jobDetails()));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();

    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "running" });

    expect(fetchMock).toHaveBeenCalledWith("/v1/jobs/17");
    expect(turn.state("chat-42")).toMatchObject({ status: "running", authority: { jobID: 17, generation: 3 } });
    expect(changed).toHaveBeenCalledOnce();
  });

  it("distinguishes planning discussion from a parsed execution request", () => {
    expect(parseAgentChannelState(jobDetails("waiting_input"), "chat-42", 17)).toMatchObject({ status: "waiting_input", waitingFor: "planning" });
    expect(parseAgentChannelState(jobDetails("waiting_input", 3, "v3_coding"), "chat-42", 17))
      .toMatchObject({ status: "waiting_input", waitingFor: "execution" });
  });

  it("retains the exact native input identity and rejects invalid projections", () => {
    const waiting = { ...jobDetails("waiting_input", 3, "v3_coding"), execution_input_id: 91 };
    expect(parseAgentChannelState(waiting, "chat-42", 17)).toMatchObject({ executionInputID: 91 });
    expect(() => parseAgentChannelState({ ...waiting, execution_input_id: 0 }, "chat-42", 17)).toThrow("exact request identity");
    expect(() => parseAgentChannelState({ ...jobDetails("waiting_input"), execution_input_id: 91 }, "chat-42", 17)).toThrow("exact request identity");
    expect(parseAgentChannelState({ ...jobDetails("waiting_input"), coding_input_disposition: "reply", execution_input_id: 91 }, "chat-42", 17))
      .toMatchObject({ waitingFor: "planning", inputDisposition: "reply", executionInputID: 91 });
    expect(parseAgentChannelState({ ...waiting, coding_input_disposition: "replan", execution_input_id: undefined }, "chat-42", 17))
      .toMatchObject({ waitingFor: "planning", inputDisposition: "replan" });
    expect(() => parseAgentChannelState({ ...waiting, coding_input_disposition: undefined }, "chat-42", 17)).toThrow("server-owned reply disposition");
  });

  it.each(["native planning reply", "incomplete execution handoff"])("consumes the server disposition for %s", async (kind) => {
    const replan = kind === "incomplete execution handoff";
    const waiting = { ...jobDetails("waiting_input", 3, replan ? "v3_coding" : "v3_coding_plan"),
      coding_input_disposition: replan ? "replan" : "reply", ...(replan ? {} : { execution_input_id: 91 }) };
    vi.stubGlobal("fetch", vi.fn(async () => response(waiting)));
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    const fetchMock = vi.fn(async (_url: string, options?: RequestInit) => {
      if (options?.method === "POST") {
        const body = JSON.parse(String(options.body)) as { operation_id: string; expected_input_id?: number };
        expect(body.expected_input_id).toBe(replan ? undefined : 91);
        return response({ operation_id: body.operation_id, channel_id: "chat-42", job_id: 17,
          disposition: replan ? "replanned" : "feedback_submitted", generation: replan ? 4 : 3 }, 202);
      }
      return response(jobDetails("completed", replan ? 4 : 3));
    });
    vi.stubGlobal("fetch", fetchMock);
    await expect(turn.continueConversation("chat-42", "Continue within the approved scope.")).resolves.toBe(17);
  });

  it("sends the observed native prompt identity with its owner reply", async () => {
    const fetchMock = vi.fn(async (_url: string, options?: RequestInit) => {
      if (options?.method === "POST") {
        const body = JSON.parse(String(options.body)) as { operation_id: string; expected_input_id: number };
        expect(body.expected_input_id).toBe(91);
        return response({ operation_id: body.operation_id, channel_id: "chat-42", job_id: 17,
          disposition: "feedback_submitted", generation: 3 }, 202);
      }
      return response(jobDetails("completed"));
    });
    vi.stubGlobal("fetch", vi.fn(async () => response({ ...jobDetails("waiting_input", 3, "v3_coding"), execution_input_id: 91 })));
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    vi.stubGlobal("fetch", fetchMock);
    await turn.continueConversation("chat-42", "approve");
    expect(fetchMock.mock.calls.some((call) => call[1]?.method === "POST")).toBe(true);
  });

  it("stops observation at owner plan review without creating an answer route", async () => {
    let status = "running";
    vi.stubGlobal("fetch", vi.fn(async () => response(jobDetails(status))));
    const { turn } = create();
    const watching = turn.watch("chat-42", 17);
    await vi.waitFor(() => expect(turn.state("chat-42")?.status).toBe("running"));
    status = "waiting_input";
    document.dispatchEvent(new CustomEvent("omni:job-progress", { detail: { jobID: 17 } }));

    await watching;

    expect(turn.state("chat-42")?.status).toBe("waiting_input");
    expect(turn.state("chat-42")).not.toHaveProperty("question");
    expect(turn.state("chat-42")).not.toHaveProperty("waitingAttempt");
  });

  it.each(["completed", "canceled", "failed"])("retires an observed job when it becomes %s", async (status) => {
    let payload = jobDetails("waiting_input");
    const fetchMock = vi.fn(async () => response(payload));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    payload = jobDetails(status);
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: status as "completed" | "canceled" | "failed" });

    expect(turn.state("chat-42")).toBeUndefined();
    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("does not enter Agents observation for a Normal job and rejects foreign authority", async () => {
    const normal = jobDetails();
    normal.job.metadata = { channel_id: "chat-42" } as typeof normal.job.metadata;
    vi.stubGlobal("fetch", vi.fn(async () => response(normal)));
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "running" });
    expect(turn.state("chat-42")).toBeUndefined();
    expect(changed).not.toHaveBeenCalled();

    const foreign = jobDetails();
    foreign.job.metadata.channel_id = "other-chat";
    expect(() => parseAgentChannelState(foreign, "chat-42", 17)).toThrow("different conversation");
  });
});
