import { describe, expect, it } from "vitest";
import { roleplayTurnSubmission } from "./roleplay_turn_input";

function persona(value: "narrator" | "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"): HTMLSelectElement {
  document.body.innerHTML = `<select>
    <option value="narrator" data-persona-kind="narrator">Narrator</option>
    <option value="rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" data-persona-kind="character">Gryph</option>
  </select>`;
  const control = document.querySelector("select") as HTMLSelectElement;
  control.value = value;
  return control;
}

describe("roleplay turn input", () => {
  it("composes ordered character parts without interpreting their prose", () => {
    expect(roleplayTurnSubmission("", persona("rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), [
      { kind: "action", text: "I lift the key." },
      { kind: "message", text: "Stay." },
      { kind: "event", text: "The north door opens." },
    ])).toEqual({
      prompt: "[Action]\nI lift the key.\n\n[Message]\nStay.\n\n[Event]\nThe north door opens.",
      turn: {
        persona_kind: "character",
        character_id: "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        contribution_kind: "structured_turn",
        parts: [
          { kind: "action", text: "I lift the key." },
          { kind: "message", text: "Stay." },
          { kind: "event", text: "The north door opens." },
        ],
      },
    });
  });

  it("uses unqueued text as one message and keeps commands deterministic", () => {
    expect(roleplayTurnSubmission("The storm breaks.", persona("narrator"), [])).toEqual({
      prompt: "[Message]\nThe storm breaks.",
      turn: {
        persona_kind: "narrator", contribution_kind: "direction",
        parts: [{ kind: "message", text: "The storm breaks." }],
      },
    });
    expect(roleplayTurnSubmission(`/research "weather"`, persona("narrator"), [])).toEqual({
      prompt: `/research "weather"`,
      turn: { persona_kind: "narrator", contribution_kind: "command" },
    });
  });

  it("rejects blank and mixed command submissions explicitly", () => {
    expect(() => roleplayTurnSubmission("", persona("narrator"), [])).toThrow("Add a message");
    expect(() => roleplayTurnSubmission("/research x", persona("narrator"), [
      { kind: "event", text: "The bell rings." },
    ])).toThrow("cannot be mixed");
  });
});
