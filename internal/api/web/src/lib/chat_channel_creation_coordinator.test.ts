import { describe, expect, it } from "vitest";
import {
  ChatChannelCreationCoordinator,
  type ChatChannelCreationHost,
} from "./chat_channel_creation_coordinator";

function createHost() {
  const mode = document.createElement("select");
  mode.innerHTML = '<option value="assistant">Assistant</option><option value="roleplay">Roleplay</option>';
  const fields = document.createElement("div");
  fields.classList.add("hidden");
  const worldName = document.createElement("input");
  const viewpointName = document.createElement("input");
  const host: ChatChannelCreationHost = {
    hasMode: () => true,
    mode: () => mode,
    hasRoleplayFields: () => true,
    roleplayFields: () => fields,
    hasWorldName: () => true,
    worldName: () => worldName,
    hasViewpointName: () => true,
    viewpointName: () => viewpointName,
  };
  return { host, mode, fields, worldName, viewpointName };
}

describe("ChatChannelCreationCoordinator", () => {
  it("keeps assistant mode explicit and disables roleplay-only inputs", () => {
    const fixture = createHost();
    const coordinator = new ChatChannelCreationCoordinator(fixture.host);

    coordinator.synchronize();

    expect(coordinator.parameters()).toEqual({ mode: "assistant" });
    expect(fixture.fields.classList.contains("hidden")).toBe(true);
    expect(fixture.worldName.disabled).toBe(true);
    expect(fixture.viewpointName.disabled).toBe(true);
  });

  it("projects exact roleplay names from the server-rendered inputs", () => {
    const fixture = createHost();
    fixture.mode.value = "roleplay";
    fixture.worldName.value = "Harbor Kingdom";
    fixture.viewpointName.value = "Alice";
    const coordinator = new ChatChannelCreationCoordinator(fixture.host);

    coordinator.synchronize();

    expect(coordinator.parameters()).toEqual({
      mode: "roleplay",
      roleplay_world_name: "Harbor Kingdom",
      roleplay_viewpoint_name: "Alice",
    });
    expect(fixture.fields.classList.contains("hidden")).toBe(false);
    expect(fixture.worldName.required).toBe(true);
    expect(fixture.viewpointName.required).toBe(true);
  });

  it("rejects invalid mode authority and inexact roleplay names", () => {
    const fixture = createHost();
    const invalid = document.createElement("option");
    invalid.value = "agent";
    fixture.mode.append(invalid);
    fixture.mode.value = "agent";
    const coordinator = new ChatChannelCreationCoordinator(fixture.host);

    expect(() => coordinator.parameters()).toThrow("not server-authorized");
    fixture.mode.value = "roleplay";
    fixture.worldName.value = " Harbor Kingdom";
    fixture.viewpointName.value = "Alice";
    expect(() => coordinator.parameters()).toThrow("exact nonblank text");
  });
});
