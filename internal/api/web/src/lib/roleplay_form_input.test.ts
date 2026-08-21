import { describe, expect, it } from "vitest";
import {
	researchCapabilityInput,
	roleplayGenerationInput,
	sceneCreateInput,
	sceneDraftParticipantInput,
	sceneInput,
	sceneUpdateInput,
} from "./roleplay_form_input";

const first = "rpc_11111111111111111111111111111111";
const second = "rpc_22222222222222222222222222222222";

function sceneForm(): HTMLFormElement {
  const form = document.createElement("form");
	form.innerHTML = `
		<input name="expected_draft_revision" value="5">
		<input name="title" value="Signal Room">
		<textarea name="description">A bounded scene.</textarea>
		<input type="hidden" name="participant_id" value="${second}">
		<input type="hidden" name="participant_id" value="${first}">
	`;
  return form;
}

describe("roleplay form input", () => {
	it("preserves the exact ordered identities projected from the server draft", () => {
		expect(sceneCreateInput(sceneForm())).toEqual({
			expected_draft_revision: 5,
			title: "Signal Room",
			description: "A bounded scene.",
			participant_ids: [second, first],
		});
	});

	it("rejects duplicate server-draft identities before dispatch", () => {
		const form = sceneForm();
		const participants = form.querySelectorAll<HTMLInputElement>('input[name="participant_id"]');
		participants[1]!.value = second;

		expect(() => sceneInput(form)).toThrow("duplicate participant");
	});

	it("binds a configured scene update to the observed server draft revision", () => {
		expect(sceneUpdateInput(sceneForm())).toEqual({
			expected_draft_revision: 5,
			title: "Signal Room",
			description: "A bounded scene.",
			participant_ids: [second, first],
		});
	});

	it("rejects the removed browser-owned checkbox ordering path", () => {
		const form = sceneForm();
		form.querySelectorAll<HTMLInputElement>('input[name="participant_id"]').forEach((input) => input.remove());

		expect(() => sceneInput(form)).toThrow("server-rendered scene draft");
	});

	it("reads one revisioned participant selection from server-rendered form authority", () => {
		const form = document.createElement("form");
		form.dataset.draftRevision = "7";
		form.dataset.charactersOffset = "4";
		form.innerHTML = '<input type="checkbox" name="selected" checked>';

		expect(sceneDraftParticipantInput(form)).toEqual({
			expected_revision: 7, selected: true, characters_offset: 4,
		});
	});

  it("reads only the exact research-access boolean", () => {
    const form = document.createElement("form");
    form.innerHTML = '<input type="checkbox" name="enabled" checked>';

    expect(researchCapabilityInput(form)).toEqual({ enabled: true });
  });

	it("reads the exact local response-model choice", () => {
		const form = document.createElement("form");
		form.innerHTML = `
			<input name="expected_revision" value="3">
			<input name="narrative_model" value="dolphin3:latest">
		`;

		expect(roleplayGenerationInput(form)).toEqual({
			expected_revision: 3,
			narrative_model: "dolphin3:latest",
		});
	});
});
