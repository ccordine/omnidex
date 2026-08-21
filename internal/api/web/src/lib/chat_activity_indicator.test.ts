import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ChatActivityIndicator, type ChatActivityIndicatorHost } from "./chat_activity_indicator";

function fixture() {
  const indicator = document.createElement("button");
  const text = document.createElement("span");
  const dot = document.createElement("span");
  const problems = document.createElement("span");
  indicator.append(dot, text, problems);
  document.body.append(indicator);
  const host: ChatActivityIndicatorHost = {
    hasIndicator: () => true,
    indicator: () => indicator,
    text: () => text,
    dot: () => dot,
    problems: () => problems,
  };
  return { activity: new ChatActivityIndicator(host), indicator, text, problems };
}

describe("ChatActivityIndicator", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    document.body.replaceChildren();
  });

  it("shows generic activity without exposing operation output", () => {
    const item = fixture();

    item.activity.begin("turn");

    expect(item.indicator.dataset.state).toBe("active");
    expect(item.text.textContent).toBe("Active");
    expect(item.indicator.textContent).not.toContain("turn");
    item.activity.end("turn");
    expect(item.indicator.dataset.state).toBe("ready");
  });

  it("retains reported problems until the user acknowledges them", () => {
    const item = fixture();

    item.activity.problem("Web synchronization failed.");

    expect(item.indicator.dataset.state).toBe("problem");
    expect(item.text.textContent).toBe("Problem");
    expect(item.problems.textContent).toBe("1");
    expect(item.indicator.title).toContain("Web synchronization failed.");
    item.activity.pulse();
    expect(item.indicator.dataset.state).toBe("active");
    vi.advanceTimersByTime(1200);
    expect(item.indicator.dataset.state).toBe("problem");
    item.activity.acknowledge();
    expect(item.indicator.dataset.state).toBe("ready");
    expect(item.problems.classList.contains("hidden")).toBe(true);
  });

  it("maps transport lifecycle into active, ready, and problem states", () => {
    const item = fixture();

    item.activity.reportTransport("connecting", "Connecting");
    expect(item.indicator.dataset.state).toBe("active");
    item.activity.reportTransport("live", "Live");
    expect(item.indicator.dataset.state).toBe("ready");
    item.activity.reportTransport("error", "Socket failed");
    expect(item.indicator.dataset.state).toBe("problem");
    expect(item.indicator.title).toContain("Socket failed");
  });
});
