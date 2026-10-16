import { afterEach, describe, expect, it, vi } from "vitest";
import { CodingAgentChannelTurn } from "./coding_agent_channel_turn";
import { parseAgentChannelState } from "./coding_agent_channel_contract";

const workspaceIdentity = "client_0123456789abcdef0123456789abcdef_directory_1_42";

function jobDetails(status = "waiting_input", generation = 3, question = "Which target should this task use?") {
  return {
    job: {
      id: 17, status, current_generation: generation,
      metadata: {
        channel_id: "chat-42", channel_user_message_id: 11, channel_mode: "assistant", client_cwd: "/workspace/project",
        client_workspace_identity: workspaceIdentity,
        coding_execution: { mode: "agents" },
      },
    },
    steps: [{ id: 29, generation, action: "v3_coding", status }],
    agent_pause: { schema: "omnidex-coding-agent-pause-v1", generation, step_id: 29, waiting_attempt: 2, questions: [question] },
  };
}

function response(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), { status, headers: { "Content-Type": "application/json" } });
}

describe("Agents conversation pause and resume", () => {
  const turns: CodingAgentChannelTurn[] = [];
  afterEach(() => { turns.forEach((turn) => turn.disconnect()); turns.length = 0; vi.restoreAllMocks(); });

  function create() {
    const failed = vi.fn();
    const turn = new CodingAgentChannelTurn(failed);
    const changed = vi.fn();
    turn.onChange(changed);
    turns.push(turn);
    return { turn, changed, failed };
  }

  it("recovers a concrete waiting question from the transcript's job backlink", async () => {
    const fetchMock = vi.fn(async () => response(jobDetails()));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();

    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    expect(fetchMock).toHaveBeenCalledWith("/v1/jobs/17");
    expect(turn.state("chat-42")).toMatchObject({ status: "waiting_input", question: "Which target should this task use?" });
    expect(changed).toHaveBeenCalledOnce();
  });

  it("sends exact answer bytes to the same job, generation and workspace with one replayable operation", async () => {
    const bodies: Array<Record<string, unknown>> = [];
    let resumed = false;
    let loseFirstReceipt = true;
    const fetchMock = vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      if (!init) return response(jobDetails(resumed ? "running" : "waiting_input"));
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      bodies.push(body);
      resumed = true;
      if (loseFirstReceipt) { loseFirstReceipt = false; throw new Error("receipt connection lost"); }
      return response({ job_id: 17, operation_id: body.operation_id, status: "running" });
    });
    vi.stubGlobal("fetch", fetchMock);
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    const feedback = "  selected target\nwith trailing space ";

    await expect(turn.answer("chat-42", feedback, "agents")).rejects.toThrow("receipt connection lost");
    await expect(turn.answer("chat-42", feedback, "agents")).resolves.toBe(17);

    expect(bodies).toHaveLength(2);
    expect(bodies[0]).toEqual({
      operation_id: expect.stringMatching(/^lifecycle_operation_[0-9a-f]{64}$/), feedback,
      expected_generation: 3, workspace_root: "/workspace/project", workspace_identity: workspaceIdentity,
      expected_waiting_attempt: 2,
    });
    expect(bodies[1]).toEqual(bodies[0]);
    expect(fetchMock.mock.calls.filter(([, init]) => init)).toHaveLength(2);
    expect(String(fetchMock.mock.calls.at(-1)?.[0])).toBe("/v1/jobs/17/feedback");
    expect(turn.state("chat-42")?.status).toBe("running");
  });

  it("rejects conflicting mode and stale generation before posting an answer", async () => {
    let generation = 3;
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(jobDetails("waiting_input", generation)));
    vi.stubGlobal("fetch", fetchMock);
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    await expect(turn.answer("chat-42", "exact draft", "conversation")).rejects.toThrow("mode cannot change");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    generation = 4;
    await expect(turn.answer("chat-42", "exact draft", "agents")).rejects.toThrow("generation or workspace changed");
    expect(fetchMock.mock.calls.every((args) => args.length === 1)).toBe(true);
  });

  it("stops the Agents observation on a question and preserves the job for resume", async () => {
    let status = "running";
    vi.stubGlobal("fetch", vi.fn(async () => response(jobDetails(status))));
    const { turn } = create();
    const watching = turn.watch("chat-42", 17);
    await vi.waitFor(() => expect(turn.state("chat-42")?.status).toBe("running"));
    status = "waiting_input";
    document.dispatchEvent(new CustomEvent("omni:job-progress", { detail: { jobID: 17 } }));

    await watching;

    expect(turn.state("chat-42")?.status).toBe("waiting_input");
    expect(turn.state("chat-42")?.authority).toMatchObject({ jobID: 17, generation: 3 });
  });

  it("rejects a stale pause even when generation and question text are unchanged", async () => {
    const initial = jobDetails();
    let payload = initial;
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response(payload));
    vi.stubGlobal("fetch", fetchMock);
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    payload = { ...initial, agent_pause: { ...initial.agent_pause, waiting_attempt: 3 } };
    await expect(turn.answer("chat-42", "exact draft", "agents")).rejects.toThrow("question changed");
    expect(fetchMock.mock.calls.every((args) => args.length === 1)).toBe(true);
  });

  it("does not enter agent execution for a Normal job recovered from the same transcript", async () => {
    const details = jobDetails();
    const payload = { ...details, job: { ...details.job, metadata: { channel_id: "chat-42" } } };
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    expect(turn.state("chat-42")).toBeUndefined();
    expect(changed).not.toHaveBeenCalled();
  });

  it.each(["completed", "canceled", "failed"])("retires a retained question when the authoritative job becomes %s", async (status) => {
    let payload = jobDetails();
    const fetchMock = vi.fn(async () => response(payload));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    payload = jobDetails(status);
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: status as "completed" | "canceled" | "failed" });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(turn.state("chat-42")).toBeUndefined();
    expect(changed).toHaveBeenCalledTimes(2);
    await expect(turn.answer("chat-42", "old draft", "agents")).rejects.toThrow("no pending Agent question");
  });

  it("clears a retained question when the newest transcript user turn is Normal", async () => {
    const normal = { job: { id: 18, status: "running", current_generation: 1, metadata: { channel_id: "chat-42" } }, steps: [] };
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => response(input === "/v1/jobs/17" ? jobDetails() : normal));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    await turn.recover("chat-42", { message_id: 12, job_id: 18, status: "running" });

    expect(turn.state("chat-42")).toBeUndefined();
    expect(changed).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenLastCalledWith("/v1/jobs/18");
  });

  it("clears the question when the newest user message has no job backlink", async () => {
    const fetchMock = vi.fn(async () => response(jobDetails()));
    vi.stubGlobal("fetch", fetchMock);
    const { turn, changed } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });

    await turn.recover("chat-42", undefined);

    expect(turn.state("chat-42")).toBeUndefined();
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(changed).toHaveBeenCalledTimes(2);
  });

  it("keeps a paused projection when a claimed terminal refresh returns foreign authority", async () => {
    let payload = jobDetails();
    vi.stubGlobal("fetch", vi.fn(async () => response(payload)));
    const { turn } = create();
    await turn.recover("chat-42", { message_id: 11, job_id: 17, status: "waiting_input" });
    payload = jobDetails("completed"); payload.job.metadata.channel_id = "other-chat";
    await expect(turn.recover("chat-42", { message_id: 11, job_id: 17, status: "completed" })).rejects.toThrow("different conversation");
    expect(turn.state("chat-42")?.status).toBe("waiting_input");
  });

  it("disposes an old observation when an authoritative newer Normal turn replaces it", async () => {
    const normal = { job: { id: 18, status: "running", current_generation: 1, metadata: { channel_id: "chat-42" } }, steps: [] };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => response(input === "/v1/jobs/17" ? jobDetails("running") : normal)));
    const { turn, failed } = create();
    const watching = turn.watch("chat-42", 17);
    await vi.waitFor(() => expect(turn.state("chat-42")?.status).toBe("running"));

    await turn.recover("chat-42", { message_id: 12, job_id: 18, status: "running" });
    await watching;

    expect(turn.state("chat-42")).toBeUndefined();
    expect(failed).not.toHaveBeenCalled();
  });

  it("rejects a foreign conversation and a waiting state without an exact concrete task question", () => {
    const foreign = jobDetails();
    foreign.job.metadata.channel_id = "other-chat";
    expect(() => parseAgentChannelState(foreign, "chat-42", 17)).toThrow("different conversation");
    const missing = jobDetails("waiting_input", 3, " ");
    expect(() => parseAgentChannelState(missing, "chat-42", 17)).toThrow("question is missing");
    const plan = jobDetails();
    plan.steps[0].action = "v3_coding_plan";
    expect(() => parseAgentChannelState(plan, "chat-42", 17)).toThrow("no exact waiting coding task");
  });
});
