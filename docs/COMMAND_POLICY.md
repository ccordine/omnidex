# Command Policy

Omnidex command execution is intentionally narrower than a normal shell session. Models propose actions; deterministic policy decides whether the action may execute.

## Core Rules

- Execute in the active working directory by default.
- Preserve user-owned state.
- Creation is additive.
- Commands must produce evidence or requested filesystem state.
- Do not repeat exact commands that already failed.
- Do not repeat exact commands that already succeeded; advance, verify, or finish.
- Do not pretend to use tools by emitting names such as `web.search`.
- Do not use placeholder credentials.

## Active Directory Protection

The active working directory is protected. Commands are rejected when they attempt to:

- remove it
- move it
- remove one of its parent directories
- delete and recreate the same path
- use recursive-force deletion

## Package Manager Scripts

Package-manager work must be observable one step at a time. Multi-line scripts with multiple `npm`, `npx`, `pnpm`, or `yarn` actions are rejected because they hide partial success/failure.

Preferred shape:

```bash
npm install @hotwired/stimulus
```

Then inspect/record evidence before the next package or build command.

## Evidence

A command is useful only if it moves the task forward or gathers evidence:

- creates requested files
- updates requested files
- runs tests/builds
- inspects local state
- queries a public source for current facts
- verifies an objective

Commands that only print an apology, fake final answer, or shell launcher are rejected.

## Continuation

When verification fails, Omnidex should treat the failure as the next observation, not as a reason to restart blindly.

Expected behavior:

- extract the concrete failure mode
- add or run a targeted regression check when practical
- choose the next smallest useful probe or patch
- keep completed objectives complete
- keep unsatisfied objectives pending
- report the observed failure and next inspection point
