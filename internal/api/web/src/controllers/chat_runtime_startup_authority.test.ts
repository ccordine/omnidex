import { expect, it, vi } from "vitest";
import { ChatRuntimeController } from "./chat_runtime_controller";

it("binds project authority before the first asynchronous startup boundary", async () => {
  let releaseTransport!: () => void;
  const transport = new Promise<void>((resolve) => { releaseTransport = resolve; });
  const order: string[] = [];
  const bindDocumentEvents = Reflect.get(
    ChatRuntimeController.prototype,
    "bindDocumentEvents",
  ) as (this: Record<string, unknown>) => void;
  const harness = {
    application: { getControllerForElementAndIdentifier: vi.fn(() => ({})) },
    element: document.createElement("div"),
    recyclrController: null,
    openedProjectID: null as number | null,
    openedProjectLocation: null as string | null,
    initializeViewState: vi.fn(),
    focusComposer: vi.fn(),
    addEvent: vi.fn(),
    wireCoordinators: vi.fn(),
    bindDocumentEvents() {
      order.push("bind");
      bindDocumentEvents.call(this);
    },
    ollamaDownloadHandler: vi.fn(),
    jobProgressHandler: vi.fn(),
    realtimeSyncHandler: vi.fn(),
    realtimeActivityHandler: vi.fn(),
    realtimeStatusHandler: vi.fn(),
    disconnectSystemActivity: vi.fn(),
    channel: {
      detectTransport: vi.fn(() => { order.push("detect"); return transport; }),
      loadChannels: vi.fn(async () => undefined),
    },
    panels: { activate: vi.fn(async () => undefined), isCurrent: vi.fn(() => false) },
    creation: { synchronize: vi.fn(), selectedMode: vi.fn(() => "assistant") },
    dataSources: { setCreationMode: vi.fn(), load: vi.fn(async () => undefined) },
    system: { loadStatus: vi.fn(async () => undefined), loadMetrics: vi.fn(async () => undefined) },
    memory: { loadGlobalActivity: vi.fn(async () => undefined), load: vi.fn(async () => undefined) },
    execution: { disconnect: vi.fn() },
  };

  const connecting = ChatRuntimeController.prototype.connect.call(harness as never);

  expect(order).toEqual(["bind", "detect"]);
  expect(harness.channel.loadChannels).not.toHaveBeenCalled();
  document.dispatchEvent(new CustomEvent("omni:project-opened", {
    detail: { project_id: 73, location: "/workspace/exact-project" },
  }));
  expect(harness.openedProjectID).toBe(73);
  expect(harness.openedProjectLocation).toBe("/workspace/exact-project");
  releaseTransport();
  await connecting;
  ChatRuntimeController.prototype.disconnect.call(harness as never);
});
