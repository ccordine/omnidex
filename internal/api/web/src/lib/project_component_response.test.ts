import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchProjectDetailComponent, fetchProjectsComponent } from "./operational_component_api";

describe("project server component response authority", () => {
	afterEach(() => vi.unstubAllGlobals());

	it("accepts exact list and detail identities", async () => {
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(
			String(input).includes("/v1/ui/projects/7")
				? { html: { bundle: "detail" }, project_id: 7, project_name: "Project", project_location: "/srv/project", tab: "settings" }
				: { html: { bundle: "list" }, count: 2 },
		), { status: 200 })));
		await expect(fetchProjectsComponent(0)).resolves.toMatchObject({ count: 2 });
		await expect(fetchProjectDetailComponent(7, "settings")).resolves.toMatchObject({ project_id: 7, tab: "settings" });
	});

	it.each([
		["list unknown field", { html: { bundle: "list" }, count: 2, fallback: true }, "list"],
		["list invalid count", { html: { bundle: "list" }, count: 21 }, "list"],
		["detail wrong project", { html: { bundle: "detail" }, project_id: 8, project_name: "Project", project_location: "/srv/project", tab: "settings" }, "detail"],
		["detail wrong tab", { html: { bundle: "detail" }, project_id: 7, project_name: "Project", project_location: "/srv/project", tab: "scrum" }, "detail"],
		["nested html fallback", { html: { bundle: "detail", fallback: "client" }, project_id: 7, project_name: "Project", project_location: "/srv/project", tab: "settings" }, "detail"],
	])("rejects %s", async (_name, payload, kind) => {
		vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(payload), { status: 200 })));
		const call = kind === "list" ? fetchProjectsComponent(0) : fetchProjectDetailComponent(7, "settings");
		await expect(call).rejects.toThrow();
	});

	it.each([[-1, "settings"], [0, "settings"], [7, "Settings"]] as const)(
		"rejects invalid request identity %s/%s before transport",
		async (id, tab) => {
			const fetchMock = vi.fn();
			vi.stubGlobal("fetch", fetchMock);
			await expect(fetchProjectDetailComponent(id, tab)).rejects.toThrow();
			expect(fetchMock).not.toHaveBeenCalled();
		},
	);
});
