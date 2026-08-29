import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createUserChannel } from "./channel_api";
import { ChatChannelCreationFlow, mechanicalChannelIdentity } from "./chat_channel_creation_flow";

vi.mock("./channel_api", () => ({ createUserChannel: vi.fn() }));

const identity = { id: "chat-01234567-89ab-4def-8123-456789abcdef", name: "Conversation exact" };
const accepted = {
  ...identity,
  scope: "user" as const,
  tags: ["user-channel"],
  project_id: 7,
  workspace_root: "/workspace",
  mode: "assistant" as const,
  created_at: "2026-08-19T02:00:00Z",
  updated_at: "2026-08-19T02:00:00Z",
};

describe("ChatChannelCreationFlow", () => {
  beforeEach(() => vi.resetAllMocks());
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("omits workspace only for null and preserves assistant data settings", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce({ ...accepted, data_source_id: "ds.primary-1" });
    const flow = new ChatChannelCreationFlow({
      workspaceRoot: () => null,
      newChannelDataSourceID: () => "ds.primary-1",
      newChannelCreationContext: () => ({ mode: "assistant" }),
    }, () => identity);

    await flow.create();

    expect(createUserChannel).toHaveBeenCalledWith({
      ...identity,
      tags: ["user-channel"],
      data_source_id: "ds.primary-1",
      mode: "assistant",
    });
  });

  it("preserves an explicit root and exact roleplay settings without data", async () => {
    vi.mocked(createUserChannel).mockResolvedValueOnce({
      ...accepted,
      mode: "roleplay",
      roleplay_viewpoint_character_id: "rpc_0123456789abcdef0123456789abcdef",
    });
    const flow = new ChatChannelCreationFlow({
      workspaceRoot: () => "/workspace/project",
      newChannelDataSourceID: () => undefined,
      newChannelCreationContext: () => ({
        mode: "roleplay",
        roleplay_world_name: "Harbor Kingdom",
        roleplay_viewpoint_name: "Alice",
      }),
    }, () => identity);

    await flow.create();

    expect(createUserChannel).toHaveBeenCalledWith({
      ...identity,
      tags: ["user-channel"],
      workspace_root: "/workspace/project",
      mode: "roleplay",
      roleplay_world_name: "Harbor Kingdom",
      roleplay_viewpoint_name: "Alice",
    });
  });

  it("issues the exact UUID and timestamp identity and fails without secure UUID support", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-19T02:03:04.567Z"));
    vi.stubGlobal("crypto", { randomUUID: () => "01234567-89ab-4def-8123-456789abcdef" });
    expect(mechanicalChannelIdentity()).toEqual({
      id: "chat-01234567-89ab-4def-8123-456789abcdef",
      name: "Conversation 2026-08-19T02:03:04.567Z",
    });
    vi.stubGlobal("crypto", {});
    expect(() => mechanicalChannelIdentity()).toThrow("Secure random channel identity generation is unavailable");
  });
});
