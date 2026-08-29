import { expect, it, vi } from "vitest";
import { handleOllamaDownload, type ChatOllamaDownloadHost } from "./chat_ollama_download";

function hostFixture() {
  const host: ChatOllamaDownloadHost = {
    roleplayIsCurrent: vi.fn(() => true),
    hasSelectedChannel: vi.fn(() => true),
    setStatus: vi.fn(),
    refreshRoleplay: vi.fn(async () => undefined),
    addEvent: vi.fn(),
    reportError: vi.fn(),
  };
  return host;
}

it("refreshes roleplay model selectors only after a download completes", async () => {
  const host = hostFixture();
  handleOllamaDownload(new CustomEvent("omni:ollama-download", {
    detail: { reason: "running", summary: "model: downloading" },
  }), host);
  expect(host.refreshRoleplay).not.toHaveBeenCalled();

  handleOllamaDownload(new CustomEvent("omni:ollama-download", {
    detail: { reason: "completed", summary: "model: success" },
  }), host);
  await Promise.resolve();

  expect(host.setStatus).toHaveBeenLastCalledWith("model: success", "ready");
  expect(host.refreshRoleplay).toHaveBeenCalledTimes(1);
});

it("rejects an unregistered download state instead of guessing", () => {
  const host = hostFixture();
  expect(() => handleOllamaDownload(new CustomEvent("omni:ollama-download", {
    detail: { reason: "mystery", summary: "unknown" },
  }), host)).toThrow("unregistered reason");
  expect(host.refreshRoleplay).not.toHaveBeenCalled();
});
