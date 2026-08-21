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
  an explicit command. The current responding character is a different
  authority and cannot simultaneously be selected as the user's persona.
- `FICTIONAL_CANON`: events established in one fictional world.
- `CHARACTER_KNOWLEDGE`: the subset of canon a specific character may know.
- `CHARACTER_MEMORY`: character-scoped retained subjective information.
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
   participant distinct from the responding character. Slash input is
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
   current authority. A changed scene, cast, responding character, meter,
   inventory, canon fact, memory, or simulation event fails immediately with
   the exact changed category and an explicit restore-and-retry path.
6. Code deterministically compiles the smallest relevant continuity context.
   Search-term interpretation, relevance, or minification is invoked only when
   one corresponding semantic uncertainty actually remains; context that
   already fits is retained without a minification call.
7. One response station receives the exact typed user contribution, the
   distinct responding character's current identity and voice, the prepared
   narrative projection, and only the selected continuity. It returns the
   final visible prose once. There is no post-response voice rewrite,
   preservation review, or narrative restatement chain.
8. A separate bounded semantic station extracts newly established canon from
   the final visible prose. Code validates, deduplicates, grants, and persists
   only that returned semantic leaf.
9. At terminal completion, code locks the unchanged base revision, reapplies
   the transition, and verifies that its result and narrative fingerprint equal
   the immutable preview.
10. Code atomically commits the verified transition, assistant message,
    validated semantic leaves, provenance, and the next turn position.

When a retained character memory is exactly the newly granted visible canon
fact, code copies those already-validated bytes into `CHARACTER_MEMORY` in the
same transaction. That exact relationship creates no semantic uncertainty and
therefore no memory model call. Newly extracted canon is granted
conservatively to the active viewpoint only; scene participation alone never
grants knowledge.

Failure at any step leaves no partial transition and does not fall back to
free-form interpretation or a second roleplay runtime.

## Dynamic composition boundary

World, scene, cast, persona sheets, responder model selection, and the user's
selected persona/contribution are server authority. The composer displays the
current responding character, effective response model, world, scene, and
revision beside the input.

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
and responding character; unanswered failed turns are visible in the
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

The production research sieve has one fixed shape. A search-term station sees
only the exact question and compiled minimal context and returns bounded query
strings. Code invokes the configured acquisition providers, fetches the bounded
candidate set, and gives an ID-only relevance station bounded excerpts. The
final roleplay response station receives only the exact question, minimal
character identity, compiled context, and selected bounded evidence. The full
simulation projection remains server authority and is never research-response
context. Code alone binds returned evidence IDs to exact citations.

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
- newly extracted canon reaches only the active viewpoint unless a later,
  separately authoritative visibility mechanism grants it;
- assistant prose cannot mutate authoritative state;
- replay is idempotent and altered replay fails loudly;
- the browser renders server state and does not maintain a competing simulation.
