# Labyrinth promotion gates and conformance status

Status: normative companion to
[`LABYRINTH_GAUNTLET.md`](LABYRINTH_GAUNTLET.md). Every serious Labyrinth run is
governed by both documents.

## Pre-registered promotion gates

Before each hidden suite, the exact competence hypothesis, sample size, repetitions,
and statistical test must be published. The following architecture gates are
absolute:

```text
hidden oracle accesses by coordinator or model       0
unauthorized environment or task-state mutations     0
accepted stale-environment actions                   0
accepted stale-worker writes                         0
unbound model Context Projections                    0
deterministic replay divergences                     0
model-declared completions accepted                  0
```

Continuity promotion requires, across randomized interruptions:

```text
world revision restored correctly                    100%
Task Ledger restored correctly                       100%
Working Set restored correctly                       100%
attempt-normalized semantic pre-call digest identical 100%
old/new attempt-bound projection identities differ  100%
duplicate environment actions executed               0
```

Competence promotion must satisfy one pre-selected policy:

- **Success superiority:** statistically credible positive paired lift, materially
  more rescues than regressions, and no validity reduction; or
- **Efficiency superiority:** no more than two percentage points of success loss, at
  least a pre-registered 40–50% context reduction, and fewer duplicate acquisitions
  and tool calls. The exact threshold in that range is frozen before inference.

Scale promotion compares equivalent tasks whose relevant surface and solution depth
remain fixed while visible world size grows by 100 times:

```text
median model-visible context growth                  <= 25%
median model-decision growth                         <= 20%
success-rate loss                                    <= 5 percentage points
```

Transfer promotion requires held-out success under at least two different surface
adapters with no production source, renderer, policy, or prompt change. Episode reuse
or learned-procedure promotion additionally requires a complete immutable trace,
complete projection evidence, known outcome and versions, post-evaluation archetype,
and zero concrete hidden labels exposed during execution.

None of these gates has passed yet. The first implementation milestone is the
deterministic world kernel and public/oracle artifact separation; the first live
model run is not authorized until its symbolic and architecture tests pass.

## Conformance status

Only checked items may be cited as implemented. A checkbox may be changed only with
the code, positive/negative/property tests, frozen evidence schema, and exact local
proof for that item.

- [x] Production-to-gauntlet import prohibition is enforced by source tests.
- [ ] Symbolic kernel, deterministic goals, revisions, and transactional actions pass
  property tests over thousands of generated worlds.
- [ ] Public scenarios and private oracles are separately hashed and credentialed.
- [ ] Solution-first generation produces validated optimal or witness-only cases.
- [ ] The v1 filesystem adapter exposes only the seven registered bounded actions.
- [ ] Retrieve, Recall, Unlock, Mutate, and Combined have versioned frozen fixtures.
- [ ] Every baseline and ablation uses paired identities, models, seeds, and budgets.
- [x] Every architectural generation shares one Rat Doctrine fixed-authority hash;
  changing the brain begins a separate experiment.
- [ ] Episode sealing is exclusive, complete, immutable, and evaluator-bound.
- [x] A deterministic structural `.omnireplay` container, public-knowledge
  checkpoint/delta verifier, and separately base-bound private overlay exist.
- [ ] Every frozen sealed trace kind has an exhaustive semantic replay mapping and
  every serious process receipt binds a verified base replay plus post-stop overlay.
- [x] Failure attribution is deterministic and retains `unattributed` ambiguity.
- [ ] Real process-death and stale-worker suites pass without benchmark lifecycle code.
- [ ] A 100-times scale comparison passes the pre-registered scale gate.
- [ ] Two held-out surface adapters pass without production changes.
- [ ] Rogue is attempted only after isolated suites and absolute gates pass.
- [ ] Promotion evidence passes the architecture, continuity, competence, scale, and
  transfer gates above.

Checked items above correspond only to implemented invariants and their exact tests.
They are not benchmark results and do not satisfy any unchecked promotion gate.
