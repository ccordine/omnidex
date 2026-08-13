import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createUserChannel,
  fetchChannelTranscript,
  sendChannelMessage,
} from "./channel_api";

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const channel = {
  id: "chat-42",
  scope: "user",
  name: "Exact chat",
  tags: ["user-channel"],
  project_id: 42,
  workspace_root: "/workspace/project",
  created_at: "2026-08-12T10:00:00Z",
  updated_at: "2026-08-12T10:00:00Z",
} as const;

const userMessage = {
  id: 91,
  channel_id: "chat-42",
  role: "user",
  content: "  exact request\n",
  created_at: "2026-08-12T10:01:00Z",
} as const;

const job = {
  id: 73,
  instruction: userMessage.content,
  pipeline: "chat",
  status: "pending",
} as const;

describe("channel API authority", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("accepts only a typed server-rendered transcript bundle", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) => response({
      channel_id: "chat-42",
      has_more: true,
      next_before_id: 41,
      html: { bundle: '<template data-recyclr-target="channel-transcript-messages"></template>' },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchChannelTranscript("chat-42", { limit: 24, beforeID: 91, requiredMessageID: 92 }))
      .resolves.toEqual({
        channel_id: "chat-42",
        has_more: true,
        next_before_id: 41,
        html: { bundle: '<template data-recyclr-target="channel-transcript-messages"></template>' },
      });
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("limit=24&before_id=91&required_message_id=92");
  });

  it("rejects malformed or contradictory transcript bundles", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(response({ channel_id: "chat-42", has_more: true, html: { bundle: "server" } }))
      .mockResolvedValueOnce(response({
        channel_id: "another-channel",
        has_more: false,
        html: { bundle: "server" },
      }))
      .mockResolvedValueOnce(response({
        channel_id: "chat-42",
        has_more: false,
        html: {},
      })));

    await expect(fetchChannelTranscript("chat-42")).rejects.toThrow("pagination fields are contradictory");
    await expect(fetchChannelTranscript("chat-42")).rejects.toThrow("changed the requested channel identity");
    await expect(fetchChannelTranscript("chat-42")).rejects.toThrow("bundle");
  });

  it("requires the authoritative 202 turn receipt", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response({
      channel,
      user_message: userMessage,
      job,
    }, 202));
    vi.stubGlobal("fetch", fetchMock);

    await expect(sendChannelMessage("chat-42", userMessage.content)).resolves.toEqual({
      channel,
      user_message: userMessage,
      job,
    });
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({ prompt: userMessage.content });
  });

  it("rejects a successful but non-202 turn response", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      channel,
      user_message: userMessage,
      job,
    }, 200)));

    await expect(sendChannelMessage("chat-42", "request")).rejects.toThrow("expected HTTP 202");
  });

  it("enforces the user transport bound before dispatch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(sendChannelMessage("chat-42", "x".repeat(4097))).rejects.toThrow("4096 UTF-8 bytes");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects a turn receipt for a different pipeline or instruction", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(response({
        channel,
        user_message: userMessage,
        job: { ...job, pipeline: "assistant" },
      }, 202))
      .mockResolvedValueOnce(response({
        channel,
        user_message: userMessage,
        job: { ...job, instruction: "changed" },
      }, 202)));

    await expect(sendChannelMessage("chat-42", userMessage.content)).rejects.toThrow('pipeline must be exactly "chat"');
    await expect(sendChannelMessage("chat-42", userMessage.content)).rejects.toThrow("exact prompt bytes");
  });

  it("requires create to return the exact typed user channel", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => response({ channel }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await expect(createUserChannel({
      id: "chat-42", name: "Exact chat", tags: ["user-channel"], workspace_root: "/workspace/project",
    }))
      .resolves.toEqual(channel);
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({
      id: "chat-42",
      name: "Exact chat",
      tags: ["user-channel"],
      workspace_root: "/workspace/project",
    });
  });

  it("rejects a channel response without a durable workspace binding", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => response({
      channel: { ...channel, project_id: 0, workspace_root: "relative/project" },
    }, 201)));

    await expect(createUserChannel({
      id: channel.id,
      name: channel.name,
      tags: [...channel.tags],
      workspace_root: channel.workspace_root,
    })).rejects.toThrow("project_id");
  });
});
