import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchChatDataSourceOptionsPage } from "./chat_component_api";
import { ChatDataSourceCoordinator, type ChatDataSourceHost } from "./chat_data_source_coordinator";

vi.mock("./chat_component_api", () => ({ fetchChatDataSourceOptionsPage: vi.fn() }));

function dataSourceBundle(location = "innerHTML"): string {
	return `<template data-recyclr-target="new-channel-data-source-options" data-recyclr-location="${location}">` +
		'<option value="">No data</option>' +
		'<option value="ds.primary-1">Customer database</option></template>';
}

function createHost() {
	const select = document.createElement("select");
	select.disabled = true;
	const renderComponentBundle = vi.fn(async (bundle: string) => {
		const documentFragment = new DOMParser().parseFromString(bundle, "text/html");
		for (const template of documentFragment.querySelectorAll<HTMLTemplateElement>("template[data-recyclr-target]")) {
			if (template.dataset.recyclrTarget !== "new-channel-data-source-options") continue;
			if (template.dataset.recyclrLocation === "beforeend") {
				select.insertAdjacentHTML("beforeend", template.innerHTML);
			} else {
				select.innerHTML = template.innerHTML;
			}
		}
	});
	const host: ChatDataSourceHost = {
		hasSelect: () => true,
		select: () => select,
		renderComponentBundle,
		setStatus: vi.fn(),
		addEvent: vi.fn(),
	};
	return { host, select, renderComponentBundle };
}

describe("ChatDataSourceCoordinator", () => {
	beforeEach(() => {
		vi.resetAllMocks();
	});

	it("applies server-rendered choices and returns only a selected authoritative option", async () => {
		const bundle = dataSourceBundle();
		vi.mocked(fetchChatDataSourceOptionsPage).mockResolvedValueOnce({
			has_more: false,
			html: { bundle },
		});
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();
		fixture.select.value = "ds.primary-1";

		expect(fixture.renderComponentBundle).toHaveBeenCalledWith(bundle);
		expect(fixture.select.disabled).toBe(false);
		expect(coordinator.selectedForCreation()).toBe("ds.primary-1");
	});

	it("keeps the explicit no-evidence choice when no source is selected", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage).mockResolvedValueOnce({
			has_more: false,
			html: { bundle: dataSourceBundle() },
		});
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();

		expect(coordinator.selectedForCreation()).toBeUndefined();
	});

	it("clears and disables real-world evidence authority for roleplay creation", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage).mockResolvedValueOnce({
			has_more: false,
			html: { bundle: dataSourceBundle() },
		});
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);
		await coordinator.load();
		fixture.select.value = "ds.primary-1";

		coordinator.setCreationMode("roleplay");

		expect(fixture.select.value).toBe("");
		expect(fixture.select.disabled).toBe(true);
		expect(coordinator.selectedForCreation()).toBeUndefined();
		coordinator.setCreationMode("assistant");
		expect(fixture.select.disabled).toBe(false);
	});

	it("fails closed when the server component cannot be loaded", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage).mockRejectedValueOnce(new Error("database unavailable"));
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();

		expect(fixture.select.disabled).toBe(true);
		expect(fixture.host.setStatus).toHaveBeenCalledWith("data connections unavailable", "error");
		expect(fixture.host.addEvent).toHaveBeenCalledWith("chat_data_sources_load_failed", {
			error: "database unavailable",
		});
	});

	it("automatically consumes server-issued pages without a visible pagination control", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage)
			.mockResolvedValueOnce({
				has_more: true,
				next_offset: 20,
				html: { bundle: dataSourceBundle() },
			})
			.mockResolvedValueOnce({
				has_more: false,
				html: { bundle: dataSourceBundle("beforeend") },
			});
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();

		expect(fetchChatDataSourceOptionsPage).toHaveBeenNthCalledWith(1, 0);
		expect(fetchChatDataSourceOptionsPage).toHaveBeenNthCalledWith(2, 20);
		expect(fixture.renderComponentBundle).toHaveBeenCalledTimes(2);
		expect(fixture.select.disabled).toBe(false);
	});

	it("fails closed when an automatic data-connection cursor does not advance", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage).mockResolvedValueOnce({
			has_more: true,
			next_offset: 0,
			html: { bundle: dataSourceBundle() },
		});
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();

		expect(fixture.select.disabled).toBe(true);
		expect(fixture.host.addEvent).toHaveBeenCalledWith("chat_data_sources_load_failed", {
			error: "The server data-connection page cursor did not advance.",
		});
	});

	it("fails closed when automatic data-connection pagination exceeds its page cap", async () => {
		vi.mocked(fetchChatDataSourceOptionsPage).mockImplementation(async (offset) => ({
			has_more: true,
			next_offset: offset + 20,
			html: { bundle: dataSourceBundle(offset === 0 ? "innerHTML" : "beforeend") },
		}));
		const fixture = createHost();
		const coordinator = new ChatDataSourceCoordinator(fixture.host);

		await coordinator.load();

		expect(fetchChatDataSourceOptionsPage).toHaveBeenCalledTimes(100);
		expect(fixture.select.disabled).toBe(true);
		expect(fixture.host.addEvent).toHaveBeenCalledWith("chat_data_sources_load_failed", {
			error: "Data connection pagination exceeded 100 server pages.",
		});
	});
});
