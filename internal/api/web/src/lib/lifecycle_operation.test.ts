import { describe, expect, it } from "vitest";
import { LifecycleOperationAttempt, newLifecycleOperationID } from "./lifecycle_operation";

describe("newLifecycleOperationID", () => {
  it("creates explicit opaque identities without binding request content", () => {
    const first = newLifecycleOperationID();
    const second = newLifecycleOperationID();
    expect(first).toMatch(/^lifecycle_operation_[0-9a-f]{64}$/);
    expect(second).toMatch(/^lifecycle_operation_[0-9a-f]{64}$/);
    expect(second).not.toBe(first);
  });
});

describe("LifecycleOperationAttempt", () => {
  it("reuses one identity after an ambiguous commit response", () => {
    const attempt = new LifecycleOperationAttempt();
    const first = attempt.acquire(attemptKey("card-1", "chat", "  Continue once.  "));
    expect(attempt.acquire(attemptKey("card-1", "chat", "Continue once."))).toBe(first);
  });

  it("rotates identity when the submitted text changes", () => {
    const attempt = new LifecycleOperationAttempt();
    const first = attempt.acquire(attemptKey("card-1", "chat", "Continue once."));
    expect(attempt.acquire(attemptKey("card-1", "chat", "Continue differently."))).not.toBe(first);
  });

  it("rotates identity when the card changes", () => {
    const attempt = new LifecycleOperationAttempt();
    const first = attempt.acquire(attemptKey("card-1", "chat", "Continue once."));
    expect(attempt.acquire(attemptKey("card-2", "chat", "Continue once."))).not.toBe(first);
  });

  it("rotates identity when the action changes", () => {
    const attempt = new LifecycleOperationAttempt();
    const first = attempt.acquire(attemptKey("job-1", "interrupt", "Continue once."));
    expect(attempt.acquire(attemptKey("job-1", "replan", "Continue once."))).not.toBe(first);
  });

  it("retains independent ambiguous attempts for different controls", () => {
    const attempt = new LifecycleOperationAttempt();
    const interrupt = attempt.acquire(attemptKey("job-1", "interrupt", "Pause here."));
    const cancel = attempt.acquire(attemptKey("job-2", "cancel", "No longer needed."));

    expect(attempt.acquire(attemptKey("job-1", "interrupt", "Pause here."))).toBe(interrupt);
    expect(attempt.acquire(attemptKey("job-2", "cancel", "No longer needed."))).toBe(cancel);
  });

  it("fails loudly instead of retaining an unbounded number of ambiguous attempts", () => {
    const attempt = new LifecycleOperationAttempt();
    for (let index = 0; index < 64; index++) {
      attempt.acquire(attemptKey(`job-${index}`, "cancel", "No longer needed."));
    }
    expect(() => attempt.acquire(attemptKey("job-overflow", "cancel", "No longer needed."))).toThrow(
      /64 pending-operation limit/i,
    );
  });

  it("resets only after the matching operation is server-confirmed", () => {
    const attempt = new LifecycleOperationAttempt();
    const key = attemptKey("card-1", "chat", "Continue once.");
    const first = attempt.acquire(key);
    expect(attempt.confirm(key, newLifecycleOperationID())).toBe(false);
    expect(attempt.acquire(key)).toBe(first);
    expect(attempt.confirm(key, first)).toBe(true);
    expect(attempt.acquire(key)).not.toBe(first);
  });
});

function attemptKey(scope: string, action: string, content: string) {
  return { scope, action, content };
}
