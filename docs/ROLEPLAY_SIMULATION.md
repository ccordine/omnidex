# Roleplay Simulation Authority

Omnidex roleplay is a server-authoritative fictional simulation with bounded
narrative generation. It is not an agent harness and it has no roleplay tool
interface.

## Invariant

Code owns every identity, rule, command, meter, inventory entry, use count,
scene participant, turn position, transition, memory record, knowledge grant,
and durable write. A model cannot invoke, propose, or apply an operation. It
cannot change fictional reality by describing a change.

A narrative model receives only a bounded projection of authoritative
fictional reality plus any exact deterministic transition preview for this
turn, and returns one prose response. The preview has no mutation authority.
When a separate semantic leaf is genuinely required, such as extracting newly
established canon from that response, a separate station receives only the
bytes needed for that question and returns only that leaf. Code validates and
persists it.

There are no model-visible functions such as `give`, `take`, `use_item`,
`set_meter`, `remember`, or `search`. Prompts must not warn models against
calling unavailable functions. Operation catalogs, command schemas, transition
rules, and mutation controls are not model context.

## Durable authorities

The roleplay runtime keeps these authorities distinct:

- user-turn authority: the selected user persona, its exact scene character
  identity when applicable, and whether the exact bytes are dialogue, action,
  action plus dialogue, narrator-established fiction, narrator direction, or
  an explicit command. The initiative cursor is separate authority and may be
  selected as the user's persona when another scene participant remains to
  respond; the selected acting character is excluded from that response round.
- `FICTIONAL_CANON`: events established in one fictional world.
- `CHARACTER_KNOWLEDGE`: the subset of canon a specific character may know.
- `CHARACTER_MEMORY`: character-scoped retained subjective information.
- per-character ongoing-action state: the latest resolved activity for one
  exact character, with append-only source and replay provenance.
- simulation state: character and scene sheets, meters, inventories,
  interactions, item rules, participants, and turn order.
- real-world evidence: separately sourced research evidence that can be
  projected into a permitted character's narrative context without becoming
  fictional canon merely because it was retrieved.

Authority does not cross namespaces implicitly. A character cannot receive a
hidden world fact. Fictional canon cannot ground a real-world research answer.
A model response is not a state transition.

## Code-owned turn flow

1. Persist the exact user turn together with its selected persona and
   contribution kind. Code validates that a selected character is a current
   participant and that at least one AI responder remains. Slash input is
   deterministically recorded as a narrator command.
2. If the roleplay turn uses the explicit slash-command grammar, code parses it
   exactly and rejects malformed or unknown commands before mutation.
3. Inside a nested transaction, code resolves the configured interaction or
   inventory operation, applies exact effects, clamps meters, evaluates finite
   uses, and runs the deterministic automatic-use sieve. It projects the exact
   post-transition narrative, then rolls every preview write back.
4. Code persists only the immutable preparation: base revision, pending
   transition, safe narrative projection, active participant, and exact
   provenance. Meters, inventory, scene revision, and transition history remain
   unchanged.
5. Before inference, code re-derives the prepared transition and narrative from
   current authority. A changed scene, cast, initiative cursor, meter,
   inventory, canon fact, memory, or simulation event fails immediately with
   the exact changed category and an explicit restore-and-retry path.
6. Code deterministically compiles the smallest relevant continuity context.
   Candidate relevance or minification is invoked only when one corresponding
   semantic uncertainty actually remains; acquisition queries and arguments
   remain exact code-owned authority. Context that already fits is retained
   without a minification call.
7. For each code-ordered responder, one response station receives the exact
   typed user contribution, that responding character's current identity and voice, the prepared
   narrative projection, and only the selected continuity. It returns the
   final visible prose once. There is no post-response voice rewrite,
   preservation review, or narrative restatement chain.
8. Separate bounded semantic calls extract newly established canon once from
   the exact fictional user contribution and once from each final response.
   Canon coverage returns only `CANON_FACT_REMAINS` or
   `NO_UNCOVERED_CANON_FACT`; code alone interprets that relation and decides
   whether the separate one-fact call runs again. Code validates, deduplicates,
   grants, and persists only each returned source-local semantic leaf.
9. At terminal completion, code locks the unchanged base revision, reapplies
   the transition, and verifies that its result and narrative fingerprint equal
   the immutable preview.
10. Code atomically commits the verified transition, assistant message,
    validated semantic leaves, provenance, and the next turn position.

Canon extraction is split by exact source. Code invokes one user-contribution
canon station exactly once for the typed fictional user turn and invokes one
assistant-response canon station separately for each accepted response. Each
station receives one `exact_contribution`; an assistant station may receive the
typed user turn only as antecedent context for resolving references, never as a
second fact source. Exact user facts retain the user message as provenance;
exact response facts retain that responder's assistant message as provenance.
Explicit commands do not enter user-contribution canon extraction.

Code alone assigns recipients for user-established facts. Character-user facts
are granted only to the explicit acting character. Narrator facts are granted
to the exact frozen, ordered participant snapshot for that prepared turn. An
empty user fact set always has an empty recipient set. Before persistence, code
filters candidate strings against world-global exact canon without exposing
hidden world facts to any model; a concurrent database conflict still fails
loudly.

One submitted user turn is one atomic response round. Code orders its bounded
responder calls synchronously from the persisted initiative cursor, wraps the
participant order deterministically, and excludes a selected acting character.
Every responder in that round receives the same exact pre-advance round, turn,
and fictional-time tick; later responders may receive only the explicitly
selected earlier responses from that same code-ordered round. No individual
model call advances time. Only terminal publication advances the initiative
cursor, global turn, and fictional-time tick once. The round increments only
when that cursor crosses the end of the persisted participant order.

Per-character ongoing action is another separate semantic leaf. For a selected
character turn, code supplies the ongoing-action station only that character's
exact persisted `[Action]` parts and previous current action. For an assistant
response, it supplies only that responder's final prose and previous current
action. The station returns one complete current-action value or its exact
absence; code validates and appends the result under the exact character and
source message. Narrator contributions have no unambiguous actor target, and
typed Event or dialogue parts are not character actions, so those inputs do
not invoke or mutate per-character ongoing-action state. Their scene
continuity remains in canon and observer-scoped history.

When a retained character memory is exactly the newly granted visible canon
fact, code copies those already-validated bytes into `CHARACTER_MEMORY` in the
same transaction. That exact relationship creates no semantic uncertainty and
therefore no memory model call. Newly extracted canon is granted
conservatively only to the character whose response established it; scene
participation alone never grants knowledge.

Failure at any step leaves no partial transition and does not fall back to
free-form interpretation or a second roleplay runtime.

## Dynamic composition boundary

World, scene, cast, persona sheets, responder model selection, and the user's
selected persona/contribution are server authority. The composer displays the
current scene, initiative cursor, round, turn, and fictional-time tick beside
the input. Per-responder models remain server-bound in the immutable turn
preparation; no singular model label is presented for a multi-responder round.

Inline persona creation commits the new identity and its scene membership in
one database transaction, then returns only the exact channel/character
receipt. Component projection is a separate authoritative GET, optionally
bound to that received character identity. A projection or rendering failure
after the receipt is reported as a view-refresh failure; it cannot recast the
committed creation as a retryable mutation or invite a duplicate identity.

Each submitted turn is composed from the latest committed values and snapshots
them atomically with the exact user message. That snapshot is immutable for the
duration of the turn so one response cannot mix two scene revisions or two
model configurations. A committed edit is therefore visible to the next
submitted turn; it does not silently rewrite a turn already in flight. If a
narrative-affecting edit races an in-flight turn, the freshness fence fails
loudly before inference when possible and again at terminal publication, and
the transcript preserves the exact failed turn for explicit retry.

Only completed user/assistant exchanges enter narrative continuity. Each
exchange preserves explicit labels for the user persona, contribution kind,
and ordered responding characters; unanswered failed turns are visible in the
transcript but do not become fictional history. A failed or canceled turn
retains its exact error and can be restored into the composer for an explicit
new attempt without mutating or silently replaying the original turn.

## Generic data model

Character and scene behavior is configured as data. Framework code must not
encode genre or benchmark nouns.

- A character sheet contains identity, persona, meters, inventory, and durable
  character-scoped context.
- A scene sheet contains its description, participants, order, active position,
  and current conditions.
- A meter has explicit bounds and a current value.
- An interaction has one exact command identity, an explicit argument shape,
  and typed meter deltas.
- An item definition has finite positive uses or an explicit infinite-use
  policy, typed meter effects, and optional deterministic trigger rules.
- An inventory entry binds an item definition to one character.
- Every applied transition records its source authority and exact effects.

An example involving appetite is only configured data. The same implementation
must prove unrelated meters, interactions, items, and scenes without adding a
new Go branch.

## External research

A character may receive real-world research only through an explicit persisted
capability with an actual code-owned consumer. Code formulates or accepts the
typed evidence need, invokes the registered resolver, validates provenance, and
projects the bounded result. The character model never receives a search
operation or resolver catalog and never claims that it performed the search.

The production research sieve has one fixed shape. The explicit typed research
request supplies the exact question, and code binds it as the acquisition query,
invokes the configured providers, and fetches the bounded candidate set. One
relevance relation call receives one bounded excerpt without its code-owned ID.
The final roleplay response station receives only the exact question, minimal
character identity, compiled context, and selected bounded evidence. The full
simulation projection remains server authority and is never research-response
context. Code alone binds returned evidence IDs to exact citations. There is no
search-term, review, or correction model station in this path.

Do not expose a research checkbox until that end-to-end consumer exists and is
tested. Write-only capability metadata is forbidden.

## Proof requirements

The production path must prove:

- two unrelated data configurations use the same simulation code;
- malformed and unknown slash commands make no state change;
- exact meter clamping and multi-meter effects;
- finite and infinite item use behavior;
- deterministic automatic-use priority and tie-breaking;
- atomic inventory and transition persistence;
- multiple characters and durable turn advancement;
- character knowledge and memory isolation across sessions;
- narrative context contains state projections but no operation catalog;
- each user message preserves the selected persona and contribution kind, and
  the response prompt keeps that persona distinct from the responder;
- a committed scene, persona, or response-model edit is projected into the
  next turn while an already-submitted turn keeps its immutable snapshot;
- stale prepared narrative authority is rejected before response inference and
  names the changed authority in the recoverable failure;
- rendered narrative and canon prompts contain no raw slash-command bytes;
- newly extracted canon reaches only its responding character unless a later,
  separately authoritative visibility mechanism grants it;
- assistant prose cannot mutate authoritative state;
- replay is idempotent and altered replay fails loudly;
- the browser renders server state and does not maintain a competing simulation.
