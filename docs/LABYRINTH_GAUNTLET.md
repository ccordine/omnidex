# Labyrinth cognition gauntlet contract

Status: normative offline benchmark contract; unchecked behavior has no implementation or promotion claim.

Labyrinth is an offline procedural laboratory for the domain-neutral Cognition Runtime
defined in [`CHARMELEON_COGNITION_RUNTIME.md`](CHARMELEON_COGNITION_RUNTIME.md). It is
not a production runtime, product mode, agent, or source of workload logic. A maze is
only its first controlled surface. Its versioned suites and reports are Cognition
Gauntlets; Rogue is the final combined long-horizon suite.

Labyrinth isolates nine claims before programming is used as evidence:

| Capability | Question |
| --- | --- |
| Discovery | Can relevant evidence be located without exposing the whole world? |
| Continuity | Do goals and progress survive loss of model and worker state? |
| Working memory | Is evidence retained while causal and released afterward? |
| Epistemic discipline | Are observations, facts, hypotheses, decisions, and rejections distinct? |
| Planning | Can bounded dependent obligations be maintained? |
| Action grounding | Are actions valid and supported by current evidence? |
| Revision and recovery | Can beliefs and plans change after contradiction or interruption? |
| Scaling | Does context follow relevant surface rather than world size? |
| Transfer | Does unchanged cognition operate through a different surface? |

Code editing is excluded from the foundational proof because its retrieval, language, framework, generation, compiler, dependency, and test failures confound these claims.

## The Rat Doctrine

During one cognition-architecture experiment, the intelligence provider is frozen.
Procedural environments vary; the brain does not. A failure may justify changes only
to domain-neutral sensing, evidence classification, state, memory, attention,
obligations, action grounding, validation, contradiction handling, recovery, or
instrumentation. It may not justify benchmark nouns, fixture knowledge, oracle access,
or a prompt tuned to one generated world.

Every experimental generation therefore seals one fixed authority containing the
model name and digest, quantization, sampling digest, native context limit, inference
backend and version, hardware class, effective context ceiling, Environment Contract
version, evaluator version, authority-policy version, and separate-process
oracle-isolation version. Paired generations must have
the same fixed-authority hash and different Cognition Runtime identities. A model
change starts a different experiment; its results cannot be presented as an
architectural improvement to the same organism.

The model ceiling is reached only when the trace proves all of the following at the
failed decision boundary: necessary evidence was acquired, recorded, retained, and
projected; the active obligation and legal action catalog were correct; the revision
and authority fences were current; and the model still repeatedly selected the wrong
bounded action. Until then, attribution names the failed body mechanism rather than
blaming or replacing the model.

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

`internal/labyrinth` may implement contracts from `internal/cognition`, and
`internal/cognitiongauntlet` may import both. Production cognition, worker, API, core,
and normal runtime packages may import neither benchmark package. The production model
renderer may contain only generic cognition vocabulary and registered environment
schemas; it may not contain Labyrinth nouns, oracle labels, fixture hints, or score
logic.

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

## Symbolic world kernel

The filesystem is a renderer, not the conceptual world. The benchmark kernel uses
typed entities, locations, connections, objects, predicates, action schemas,
preconditions, effects, documents, mutable targets, and goal expressions.
Conceptually:

```go
type Predicate struct { Name PredicateName; Args []EntityID }
type ActionSchema struct {
    ID ActionSpecID; Preconditions []Predicate; Effects []Effect; Cost int64
}
type GoalExpression struct { All, Any, Not []Predicate }
```

The complete predicate set stays inside the environment host. Legal transitions emit
only registered observations. Files, text-adventure descriptions, records, and
repository-like objects are alternate renderings of the same symbolic class, not
separate cognition implementations.

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

Dynamic maps, combat, health, crafting, random effects, scarcity, irreversible traps,
multiple agents, web, learned skills, and code generation are excluded from v1.

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

## Extended cognition suites

Only after the initial five are individually diagnosable may frozen suites add:

| Suite | Isolated capability |
| --- | --- |
| Traverse | Partial map construction and backtracking |
| Bind | Combine evidence from distant sources |
| Revise | Reject a contradicted belief and replan |
| Order | Respect ordered or irreversible actions |
| Resume | Recover under real process interruption and attempt takeover |
| Scale | Hold relevant surface fixed while adding irrelevant artifacts |
| Transfer | Reuse unchanged cognition through a different environment skin |
| Rogue | Combine long dependencies, multiple goals, dynamic state, and limited resources |

Game decoration is never a suite objective; a new mechanic must isolate a declared cognitive property and first pass symbolic validity tests.

## Baselines and controlled ablations

Paired variants use the same model, seed, scenario, action API, sampling parameters,
and budgets. Registered offline variants are:

| Variant | Purpose |
| --- | --- |
| Deterministic oracle | Validate the world and establish the optimal or witness bound |
| Raw observation only | Expose only the public goal and current observation |
| Full transcript | Conventional agent baseline |
| Transcript plus compaction | Conventional long-context baseline |
| Task Ledger only | Isolate external continuity |
| Ledger plus Working Set | Isolate retention lifecycle |
| Ledger plus Context Projection | Isolate software-defined context |
| Full Cognition Runtime | Add obligations, attention requests, and recovery |
| Oracle evidence packet | Estimate model-policy ceiling with perfect next-step evidence |
| Raw shell agent | Compare typed operations with shell wandering |

Every preregistered variant coordinate executes blind before any evaluator loads a
score or selects a winner. After all model calls have stopped, code derives the
following comparisons sequentially from the complete sealed matrix:

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

Sealing writes one exclusive episode manifest that hashes the complete ordered trace,
all projection and model-call evidence, final public revision, runtime versions, and
resource counters. Nothing may append to a sealed trace. Evaluation creates a
separate hash-bound record; it does not rewrite the episode or expose private fields
to production storage during execution.

## Scoring and failure attribution

There is no aggregate intelligence score. Reports preserve paired outcome, validity,
stability, efficiency, memory behavior, planning behavior, recovery, and scale
metrics. Repetitions measure stability and are not independent cases.

Required metric families are:

- **Primary:** goal success, valid terminal state, authority violations, and
  cross-repetition stability.
- **Efficiency:** model decisions, environment actions, low-level transitions, model
  calls, input/output tokens, model and wall time, context bytes, peak Working Set
  bytes, search/read counts, and per-station clean-desk budget use.
- **Memory:** critical evidence acquired and available when needed, projection misses,
  stale and irrelevant resident bytes, release latency, reacquisitions, and thrashing.
- **Planning:** obligations created/completed, plan churn, unnecessary subgoals,
  unsupported or invalid actions, dead-end revisits, and backtracks.
- **Recovery:** restoration mismatches, duplicate suppression, stale-attempt
  rejections, and projection identity after restart.
- **Scale:** world and relevant-surface size, model context, decisions, retrieval
  rounds, and success.

Every call also reports context concentration as code-auditable projected relevant
bytes divided by total model-visible bytes. Required, useful supporting, omitted
critical, and irrelevant selected evidence are separate counters derived from sealed
projection/oracle joins after execution; an LLM judge does not label them. The metric
never rewards simply shrinking a projection that omitted necessary evidence.

For `optimal` cases, `decision_regret = actual decision cost / optimal decision cost`.
For `witness_only` cases, `witness_overhead = actual decision cost / witness decision
cost`. Reports never compare one label as though it were the other.

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

## Restart and stale-worker gauntlet

The Resume suite uses the real production lifecycle, PostgreSQL state, environment
host, process boundary, and attempt lease. It does not substitute a benchmark-only
coordinator. Schedules include no interruption, one and five seeded random kills,
kill after every model decision, lease expiry during inference, and an old worker
waking after replacement.

Immediately before the next model call, interrupted and uninterrupted executions at
the same transition boundary must have identical environment revision, Task Ledger
replay/materialized hashes, Working Set version and members, active obligation and
generation, rendered model-visible projection bytes, action catalog, action receipts,
and all non-actor budget and policy fields. The code-derived continuity comparison is
an attempt-normalized semantic digest over those exact values and evidence refs. The
replacement's immutable Context Projection and snapshot identities must differ from
the stale worker's identities because they bind the new attempt and worker; both old
identities remain fenced. The next stochastic response is outside this equality check.

After takeover, the stale worker attempts a ledger write, Working Set mutation,
model-call evidence write, environment action, and goal completion. Every attempt must
return the one typed stale-attempt failure and produce no state change. A previously
committed action retried by the replacement uses the same `ActionID` and returns its
recorded transition without executing twice.

## Scale and transfer gauntlets

Scale cases preserve solution depth, relevant evidence count, semantic decisions,
action catalog, and public goal while growing irrelevant visible artifacts:

```text
World A       100 artifacts
World B    10,000 artifacts
World C 1,000,000 artifacts
```

Index, storage, and bounded search implementation cost may grow. Working Set size,
model-visible context, model decisions, and correctness should follow the relevant
surface. Reports make `context bytes / relevant surface bytes` primary and do not
normalize prompt size by total world size.

Transfer uses the same latent causal cases through at least two of these independently
versioned surfaces: filesystem, text adventure, knowledge base, and repository-like.
Only the environment adapter and public rendering change. A production source,
renderer, prompt, retention-policy, obligation-policy, or action-decision schema
change invalidates the transfer claim.

## Experience normalization boundary

Sealed traces may later be normalized into generic operation motifs such as search for
prerequisite evidence, resolve a target, acquire a prerequisite, and apply it to a
blocked transition. Normalization runs only after evaluation and cannot change the
source episode.

A reusable procedure is invalid if it contains a seed, concrete entity or room ID,
file path, item name, exact clue wording, hidden label, or oracle fact. A candidate is
a typed DAG of registered operations, variables, preconditions, provenance, and
validation evidence. It remains unavailable until historical replay, held-out
Labyrinth, live shadow, and paired rescue/regression gates accept it through the
durable skill registry. Learning is not part of the initial cognition proof.

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

## Hard anti-goals

The following invalidate an implementation or run:

- a maze-specific branch, noun, action, or planner in production cognition code;
- coordinator or model access to hidden state, an oracle, a seed, or live scores;
- LLM-generated worlds, labels, success judgments, or failure attribution in the
  foundational suites;
- full transcript, raw shell, or increased context budget as a production fallback;
- direct model mutation of Task Ledger, Working Set, action catalog, or completion;
- accepting a model claim as fact without evidence and a code-owned policy;
- silently replacing failed actions or outcomes on retry;
- one aggregate intelligence score or prompt tuning against a frozen seed;
- learning before immutable experience sealing and replay validation exist;
- dynamic game complexity before isolated suites pass;
- a benchmark-only coordinator, planner, lease, or evaluator presented as production
  cognition evidence.

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
- [x] Failure attribution is deterministic and retains `unattributed` ambiguity.
- [ ] Real process-death and stale-worker suites pass without benchmark lifecycle code.
- [ ] A 100-times scale comparison passes the pre-registered scale gate.
- [ ] Two held-out surface adapters pass without production changes.
- [ ] Rogue is attempted only after isolated suites and absolute gates pass.
- [ ] Promotion evidence passes the architecture, continuity, competence, scale, and
  transfer gates above.

Checked items above correspond only to implemented invariants and their exact tests.
They are not benchmark results and do not satisfy any unchecked promotion gate.
