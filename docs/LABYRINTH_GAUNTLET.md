# Labyrinth cognition gauntlet contract

Status: benchmark design only. No Labyrinth world engine, generator, environment
adapter, cognition runner, frozen fixture, oracle, score, or promotion result is
implemented or claimed by this document.

Labyrinth is an offline procedural cognition laboratory for the domain-neutral
Omnidex Cognition Runtime defined in
[`CHARMELEON_COGNITION_RUNTIME.md`](CHARMELEON_COGNITION_RUNTIME.md). It is not a
production runtime, a product mode, an agent, or a source of workload logic. A maze is
only the first controlled environment surface.

## Boundary and dependency direction

```text
benchmark-only generator ──> sealed execution scenario
             │                        │
             └──> private oracle      ▼
                    (unavailable)  environment host
                                      │ Environment Contract
──────────────────── production contract boundary ────────────────────
                                      ▼
                              Cognition Runtime
                                      │
                              bounded decisions
                                      ▼
                              environment host

after every model call stops:
sealed trace + private oracle ──> separate evaluator ──> report
```

`internal/cognitiongauntlet` may import `internal/cognition`. Production cognition,
worker, API, core, and normal runtime packages may not import the gauntlet. The
production model renderer may contain only generic cognition vocabulary and registered
environment schemas; it may not contain Labyrinth nouns, oracle labels, fixture hints,
or score logic.

The benchmark does not prove autonomy merely by exercising an interface. A promotion
run must use frozen checked-in production code, an ordinary public request boundary,
an isolated coordinator, exact call evidence, no human steering, and an evaluator
that remains unavailable until the run stops.

## Three separate states

The world kernel will model three states without collapsing them into text chunks:

| State | Meaning | Authority |
| --- | --- | --- |
| World state | Complete predicates and effects that are actually true | Environment host only |
| Observed state | Bounded facts exposed by legal transitions | Environment evidence |
| Belief/task state | Recorded facts, hypotheses, decisions, failures, and obligations | Task Ledger with provenance |

A clue is an observation. A proposed interpretation is a hypothesis. A failed action
may contradict the hypothesis without changing the historical clue. The evaluator
uses world truth; the model never receives it directly.

## Public scenario and private oracle

Generation emits two independently hashed artifacts:

**Public execution scenario**

- opaque scenario ID and format version;
- public goal statement;
- environment adapter and action-catalog versions;
- initial public observation contract;
- difficulty coordinates that are safe to disclose, if the suite declares them
  public;
- public scenario digest.

It contains no seed, latent predicate inventory, solution graph, relevance label,
shortest path, goal-state serialization, hidden task archetype, or score.

**Private oracle**

- generator version and seed;
- complete latent world identity;
- generated causal solution graph and witness plan;
- exact goal predicate and relevant evidence labels;
- optimal plan and cost when exhaustively proven;
- otherwise a witness cost and proven lower bound;
- oracle quality, oracle digest, and evaluator version.

The two artifacts are written to separate storage with separate credentials. A serious
run gives the coordinator neither path nor credential for either private oracle or
evaluator output. The environment host receives only the sealed state needed to apply
legal transitions. The evaluator loads the oracle only after the episode is sealed and
all model calls have stopped.

Oracle quality is exactly one of:

- `optimal`: BFS or A* has proven the minimum cost in a finite world;
- `witness_only`: generation has proven at least one valid solution but not optimality.

Only `optimal` cases report decision regret. A `witness_only` case reports witness
overhead and must not imply optimality.

## Solution-first generation

The deterministic generator will:

1. select one declared cognition capability and difficulty vector;
2. construct a causal solution DAG;
3. instantiate the required entities, predicates, actions, and effects;
4. place that graph into a connected topology;
5. render bounded documents and observations with deterministic grammars;
6. add controlled distractors and false leads;
7. symbolically validate the witness and every completion predicate;
8. seal public and private artifacts separately.

The initial generator will not use an LLM. Frozen paraphrase suites may later test
language robustness, but they receive a distinct fixture version and cannot alter the
foundational symbolic suite.

Property tests are required before a model run. Across thousands of generated cases
they must prove that a witness succeeds, public serialization contains no hidden
state, exact action replay is idempotent, conflicting replay and stale revision fail,
invalid actions have no effect, and repeated goal evaluation is deterministic.

## Labyrinth v1 surface

The first filesystem-backed surface is deliberately small:

- a static connected topology;
- bounded local observations and navigation;
- searchable deterministic documents;
- inventory and one or two prerequisite mechanisms;
- delayed clues;
- one hash-bound authorized text mutation;
- a deterministic terminal predicate.

Its registered macro-action catalog is `observe`, `search`, `read`, `navigate`,
`take`, `use`, and `write`. Search may use `rg --json` internally, but the model does
not receive a shell. Every result is bounded, identified, and bound to a world
revision. Write additionally requires the exact current target-content hash.

Initial cases contain 25–250 visible artifacts, 3–8 relevant artifacts, 2–5 causal
dependency edges, and a target of 4–10 meaningful model decisions. These are fixture
generation bounds, not model context entitlements.

Dynamic maps, combat, health, crafting, random effects, resource scarcity,
irreversible traps, multiple agents, web access, learned skills, and code generation
are excluded from v1. They cannot be added merely to make a benchmark look harder.

## Initial five microgauntlets

Each suite isolates a failure class and has its own versioned cases, private labels,
difficulty coordinates, and report. The small Combined suite is not a substitute for
passing the four isolated suites.

| Suite | Required behavior | Primary diagnostic |
| --- | --- | --- |
| Retrieve | Find and read a bounded relevant artifact among controlled distractors | Acquisition and irrelevant-byte cost |
| Recall | Preserve an early evidence-bound fact until a later obligation needs it | Retention, omission, release, and reacquisition |
| Unlock | Discover and satisfy a two-to-five-edge prerequisite graph | Obligation validity, blocked/ready transitions, and backtracking |
| Mutate | Perform one authorized exact-hash write after acquiring its value | Evidence grounding, stale-hash rejection, and terminal recognition |
| Combined | Retrieve a prerequisite clue, acquire access, bind delayed evidence, and complete one mutation | End-to-end continuity under a small causal graph |

For Retrieve, success requires the exact relevant observation to be acquired without
oracle knowledge; top-k labels alone are not terminal success. Recall places the
needed fact early and requires it only after intervening irrelevant work. Unlock uses
a symbolically known prerequisite DAG and records unnecessary subgoals and dead-end
revisits. Mutate includes negative stale-revision and stale-target-hash cases. Combined
uses the same production loop and action catalog without suite-specific prompt text.

Difficulty is a vector, not a tier number. At minimum reports preserve world size,
branching factor, solution depth, distractor ratio, semantic ambiguity, dependency
count, delayed-fact count, simultaneous obligations, irreversible-action count,
Working Set budget, context budget, tool budget, and restart schedule.

## Baselines and controlled ablations

Paired variants use the same model, seed, scenario, action API, sampling parameters,
and budgets. Initial comparisons are sequential rather than an all-variants sweep:

1. raw current observation versus full transcript;
2. the winner versus Task Ledger;
3. the winner versus Ledger plus Working Set;
4. the winner versus immutable Context Projection and full cognition coordination;
5. the final architecture versus an isolated raw-shell agent.

Transcript, transcript compaction, raw shell, and oracle-evidence-packet variants are
offline baselines only. None is a production fallback. The oracle evidence packet may
expose the exact minimal evidence for the next decision only in a separately labelled
model-ceiling run; it never shares results with a normal episode.

## Immutable identities and episode record

Every scenario bundle records:

- generator, public format, surface adapter, and action-catalog versions;
- seed in private metadata only;
- public scenario digest and private oracle digest;
- difficulty vector and oracle quality.

Every sealed episode records:

- episode, job, run, Task Ledger generation, and lifecycle-attempt identities;
- Omnidex commit and Cognition Runtime version;
- Task Ledger schema, Working Set policy, Context Projection spec, and renderer
  versions;
- model name, digest, quantization, sampling parameters, context limit, hardware, and
  backend;
- every exact model-visible projection, prompt, response, validation, rejection, and
  token/byte/timing record;
- every action ID, request hash, pre/post revision, observation identity,
  transition result, and low-level transition count;
- every obligation, ledger event, Working Set mutation, projection ID, failure,
  restart, lease change, and stale-write rejection;
- goal outcome, oracle or witness metrics, resource use, and deterministic failure
  attribution.

The hidden task archetype and mechanics may be attached only after sealing and
evaluation. Failed and partial episodes remain immutable evidence.

## Scoring and failure attribution

There is no aggregate intelligence score. Reports preserve paired outcome, validity,
stability, efficiency, memory behavior, planning behavior, recovery, and scale
metrics. Repetitions measure stability and are not independent cases.

Attribution is deterministic from the trace:

| Trace condition | Attribution |
| --- | --- |
| Necessary evidence was never acquired | acquisition/retrieval failure |
| Evidence was acquired but no belief entry referenced it | state-recording failure |
| An active fact was released before its dependent obligation | retention failure |
| Resident required evidence was omitted from a projection | Context Projection failure |
| Required evidence was visible but the decision was wrong | model-policy failure |
| A schema-valid, evidence-supported action was incorrectly rejected | contract/runtime failure |
| An action used an obsolete environment revision | stale-state failure |
| Restart reconstructed different deterministic state | continuity failure |
| The goal predicate was true but terminal state was not recorded | completion failure |

The classifier records the exact acquisition action, entry, release event, projection,
decision, or transition that establishes the label. Ambiguous traces remain
`unattributed`; an LLM judge does not guess a category.

## Experimental discipline

- Public cases and private oracles/labels are versioned and hashed separately.
- Paired variants use identical seeds; scores load only after all model calls stop.
- Prompts and policies are frozen before hidden evaluation.
- New or corrected cases require a new fixture version; old evidence is retained.
- Holdouts include topology, vocabulary grammar, aliases, dependency composition,
  goal composition, and surface skin, not only random seeds.
- A task is the statistical unit. Paired pass-to-fail regressions and fail-to-pass
  rescues are reported explicitly.
- Evaluation is symbolic and code-owned; no LLM judge or model-authored success claim
  is accepted.
- A benchmark failure may justify only a task-neutral production change proven on at
  least two unrelated fixtures before a completely new run.
- Runtime code, model prompts, and fixtures are not patched during a run.

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
pre-call Context Projection hash identical           100%
duplicate environment actions executed               0
```

Competence promotion must satisfy one pre-selected policy:

- **Success superiority:** statistically credible positive paired lift, materially
  more rescues than regressions, and no validity reduction; or
- **Efficiency superiority:** no more than two percentage points of success loss, at
  least 40% context reduction, and fewer duplicate acquisitions and tool calls.

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
