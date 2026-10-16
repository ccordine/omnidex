import { afterEach, describe, expect, it, vi } from "vitest";
import { authoritativeControlJobID, ChatJobsCoordinator, type ChatJobsHost } from "./chat_jobs_coordinator";

afterEach(() => {
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("authoritativeControlJobID", () => {
	it("accepts only the job the user controlled", () => {
		const operationID = `lifecycle_operation_${"a".repeat(64)}`;
		expect(authoritativeControlJobID({ job_id: 42, operation_id: operationID, status: "running" }, "42", operationID)).toBe("42");
		expect(() => authoritativeControlJobID({ job_id: 43, operation_id: operationID, status: "running" }, "42", operationID)).toThrow(/expected job 42/i);
	});

	it("rejects missing and malformed authority instead of watching the old job", () => {
		const operationID = `lifecycle_operation_${"a".repeat(64)}`;
		for (const payload of [
			{},
			{ job_id: 42, operation_id: operationID, status: "running", job: {} },
			{ job_id: 0, operation_id: operationID, status: "running" },
			{ job_id: 42, operation_id: `lifecycle_operation_${"b".repeat(64)}`, status: "running" },
			{ job_id: 42, operation_id: operationID, status: "invented" },
		]) {
			expect(() => authoritativeControlJobID(payload, "42", operationID)).toThrow();
		}
	});
});

describe("ChatJobsCoordinator lifecycle retries", () => {
	it("rejects malformed bounded job state before applying server markup", async () => {
		vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({
			job: { id: 42, status: "running", current_generation: 2 },
			steps: [{ id: 9, action: "v3_coding", status: "running", generation: 1 }],
			progress: { latest_context_id: 1, count: 25 },
			html: { bundle: "must-not-render" },
		})));
		const host = chatJobsTestHost();
		const coordinator = new ChatJobsCoordinator(host);
		const button = document.createElement("button");
		button.dataset.jobId = "42";

		await expect(coordinator.select({ currentTarget: button } as unknown as Event))
			.rejects.toThrow(/current generation|bounded progress/i);
		expect(host.renderComponentBundle).not.toHaveBeenCalled();
		expect(host.setCurrentJobID).not.toHaveBeenCalled();
	});

	it("submits one replan operation per interaction", async () => {
		const controlRequests: RequestInit[] = [];
		const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
			if (input === "/v1/jobs/42") return jsonResponse(jobDetails());
			if (input === "/v1/jobs/42/replan") {
				if (!init) throw new Error("Replan request options are required.");
				controlRequests.push(init);
				const body = JSON.parse(String(init.body));
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: "running" });
			}
			if (input === "/v1/ui/chat/jobs/42") return jsonResponse({
				job: { id: 42, status: "running", current_generation: 1 }, steps: [],
				progress: { latest_context_id: 0, count: 0 }, html: { bundle: "server-job-details" },
			});
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  use the corrected plan\nwith trailing space  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";
		button.textContent = "Replan";

		await coordinator.replan({ currentTarget: button } as unknown as Event);

		expect(controlRequests).toHaveLength(1);
		const request = JSON.parse(String(controlRequests[0]?.body));
		expect(request.feedback).toBe("  use the corrected plan\nwith trailing space  ");
		expect(request.operation_id).toMatch(/^lifecycle_operation_[0-9a-f]{64}$/);
		expect(request.observed_ledger_revision).toBe(0);
		expect(Object.keys(request).sort()).toEqual(["feedback", "observed_ledger_revision", "operation_id"]);
		expect(button.disabled).toBe(false);
		expect(button.getAttribute("aria-busy")).toBe("false");
		expect(button.textContent).toBe("Replan");
	});

	it("reuses the cancellation identity after an ambiguous response", async () => {
		const cancelRequests: RequestInit[] = [];
		let cancelCalls = 0;
		const fetchMock = vi.fn(async (input: string, init?: RequestInit) => {
			if (input === "/v1/jobs/42") return jsonResponse(jobDetails());
			if (input === "/v1/jobs/42/cancel") {
				if (!init) throw new Error("Cancellation request options are required.");
				cancelRequests.push(init);
				cancelCalls++;
				if (cancelCalls === 1) throw new Error("response lost after commit");
				const body = JSON.parse(String(init.body));
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: "canceled" });
			}
			if (input === "/v1/ui/chat/jobs?limit=20&offset=0") {
				return jsonResponse({ has_more: false, html: { bundle: "server-job-list" } });
			}
			throw new Error(`Unexpected request ${input}`);
		});
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("  deliberate cancellation\n  ");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button");
		button.dataset.jobId = "42";
		button.textContent = "Cancel";
		const event = { currentTarget: button } as unknown as Event;

		await expect(coordinator.cancel(event)).rejects.toThrow(/response lost after commit/i);
		await coordinator.cancel(event);

		expect(cancelRequests).toHaveLength(2);
		const first = JSON.parse(String(cancelRequests[0]?.body));
		const second = JSON.parse(String(cancelRequests[1]?.body));
		expect(second.operation_id).toBe(first.operation_id);
		expect(second.reason).toBe("  deliberate cancellation\n  ");
		expect(Object.keys(second).sort()).toEqual(["operation_id", "reason"]);
		expect(button.disabled).toBe(false);
		expect(button.getAttribute("aria-busy")).toBe("false");
		expect(button.textContent).toBe("Cancel");
	});

	it.each(["cancel", "interrupt", "replan"] as const)("binds %s to the exact selected Agents workspace", async (action) => {
		const bodies: Record<string, unknown>[] = [];
		const workspaceIdentity = "client_0123456789abcdef0123456789abcdef_directory_0_42";
		vi.stubGlobal("fetch", vi.fn(async (input: string, init?: RequestInit) => {
			if (input === "/v1/jobs/42") return jsonResponse(jobDetails({ client_cwd: "C:\\Projects\\target", client_workspace_identity: workspaceIdentity, coding_execution: { mode: "agents" } }));
			if (input.startsWith("/v1/jobs/42/plan?")) return jsonResponse({ job_id: 42, revision: 7, ledger: { revision: 7 } });
			if (input === `/v1/jobs/42/${action}`) {
				const body = JSON.parse(String(init?.body)); bodies.push(body);
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: action === "cancel" ? "canceled" : "running" });
			}
			if (input === "/v1/ui/chat/jobs/42") return jsonResponse({ ...jobDetails(), html: { bundle: "server-details" } });
			if (input === "/v1/ui/chat/jobs?limit=20&offset=0") return jsonResponse({ has_more: false, html: { bundle: "server-list" } });
			throw new Error(`Unexpected request ${input}`);
		}));
		vi.spyOn(window, "prompt").mockReturnValue("  exact user instruction  ");
		const button = document.createElement("button"); button.dataset.jobId = "42";
		await new ChatJobsCoordinator(chatJobsTestHost())[action]({ currentTarget: button } as unknown as Event);
		expect(bodies).toEqual([{
			operation_id: expect.stringMatching(/^lifecycle_operation_[0-9a-f]{64}$/),
			[action === "cancel" ? "reason" : "feedback"]: "  exact user instruction  ",
			...(action === "replan" ? { observed_ledger_revision: 7 } : {}),
			workspace_root: "C:\\Projects\\target", workspace_identity: workspaceIdentity,
		}]);
	});

	it("replays a bound replan after a lost response advances generation", async () => {
		let generation = 1;
		const bodies: Record<string, unknown>[] = [];
		vi.stubGlobal("fetch", vi.fn(async (input: string, init?: RequestInit) => {
			const details = jobDetails({ client_cwd: "/workspace/target", client_workspace_identity: "directory_1_42" });
			details.job.current_generation = generation;
			if (input === "/v1/jobs/42") return jsonResponse(details);
			if (input.startsWith("/v1/jobs/42/plan?")) return jsonResponse({ job_id: 42, revision: 3, ledger: { revision: 3 } });
			if (input === "/v1/jobs/42/replan") {
				const body = JSON.parse(String(init?.body)); bodies.push(body);
				if (bodies.length === 1) { generation++; throw new Error("response lost after commit"); }
				return jsonResponse({ job_id: 42, operation_id: body.operation_id, status: "running" });
			}
			if (input === "/v1/ui/chat/jobs/42") return jsonResponse({ ...details, html: { bundle: "server-details" } });
			throw new Error(`Unexpected request ${input}`);
		}));
		vi.spyOn(window, "prompt").mockReturnValue("same exact instruction");
		const coordinator = new ChatJobsCoordinator(chatJobsTestHost());
		const button = document.createElement("button"); button.dataset.jobId = "42";
		const event = { currentTarget: button } as unknown as Event;
		await expect(coordinator.replan(event)).rejects.toThrow("response lost after commit");
		await coordinator.replan(event);
		expect(bodies).toHaveLength(2);
		expect(bodies[1]).toEqual(bodies[0]);
	});

	it.each([
		{ client_cwd: "/workspace/target", client_workspace_identity: null },
		{ client_cwd: "../other", client_workspace_identity: "directory_1_42" },
		{ client_cwd: "/workspace/target", client_workspace_identity: "directory_01_42" },
		{ client_cwd: "/workspace/target", coding_execution: { mode: "agents" } },
	])("rejects malformed workspace authority before sending a cancellation", async (metadata) => {
		const fetchMock = vi.fn(async () => jsonResponse(jobDetails(metadata)));
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("cancel");
		const button = document.createElement("button"); button.dataset.jobId = "42";
		await expect(new ChatJobsCoordinator(chatJobsTestHost()).cancel({ currentTarget: button } as unknown as Event)).rejects.toThrow();
		expect(fetchMock.mock.calls).toHaveLength(1);
		expect(fetchMock.mock.calls[0]).toEqual(["/v1/jobs/42"]);
	});

	it("rejects a foreign job before sending a lifecycle control", async () => {
		const details = jobDetails(); details.job.id = 43;
		const fetchMock = vi.fn(async () => jsonResponse(details));
		vi.stubGlobal("fetch", fetchMock);
		vi.spyOn(window, "prompt").mockReturnValue("cancel");
		const button = document.createElement("button"); button.dataset.jobId = "42";
		await expect(new ChatJobsCoordinator(chatJobsTestHost()).cancel({ currentTarget: button } as unknown as Event)).rejects.toThrow(/requested #42/i);
		expect(fetchMock.mock.calls).toHaveLength(1);
	});
});

function jobDetails(metadata?: Record<string, unknown>) {
	return { job: { id: 42, status: "running", current_generation: 1, ...(metadata === undefined ? {} : { metadata }) }, steps: [] };
}

function chatJobsTestHost(): ChatJobsHost {
	const filter = document.createElement("select");
	return {
		queueEnabled: () => true,
		jobFilter: () => filter,
		hasJobBadge: () => false,
		jobBadge: () => document.createElement("div"),
		setCurrentJobID: vi.fn(),
		renderComponentBundle: vi.fn(async () => undefined),
		addEvent: vi.fn(),
	};
}

function jsonResponse(payload: unknown): Response {
	return new Response(JSON.stringify(payload), {
		status: 200,
		headers: { "Content-Type": "application/json" },
	});
}
