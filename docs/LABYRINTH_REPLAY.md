# Labyrinth deterministic replay protocol

Status: normative replay boundary. The collision-free structural container core is
implemented. Semantic mapping from every production cognition trace kind is not yet
complete, so no serious Labyrinth execution is replay-qualified.

Replay is a debugging protocol, not an animation recording. A renderer must consume
immutable cognitive events after the run. It must never infer cognition from log
strings, terminal output, or a frontend's local state.

## Required artifacts

One run eventually produces two independently immutable artifacts:

1. `<episode>.omnireplay` is the public base replay. It may be created only from the
   sealed production cognition trace and the exact public run/world authority after
   inference stops.
2. `<episode>.omnireplay.private` is an optional private overlay. The evaluator may
   create it only after inference stops and private evaluation begins. It binds the
   byte digest of one base replay plus the exact episode seal, private oracle, and
   evaluation artifacts.

The overlay never modifies, replaces, or becomes a fallback for the base. A
replay-qualified base cannot contain a seed, oracle, hidden predicate state, private
relevance label, evaluator result, or private-world checkpoint.

## Deterministic container

Both artifacts use a stdlib-only ZIP container with stored entries. Compression,
filesystem timestamps, host-dependent permissions, comments, extra fields, and
arbitrary entry order are forbidden. Entry metadata, mode, DOS timestamp, and order
are fixed by code. Re-encoding a verified container must reproduce every byte.

The base order is:

```text
manifest.json
sources/page-000000.jsonl
sources/page-000001.jsonl
...
events/page-000000.jsonl
...
checkpoints/page-000000.jsonl
...
blobs/sha256/<digest>       sorted by digest
```

The private overlay uses the same order under `private/sources`, `private/events`,
and `private/frames`, followed by its own content-addressed blobs.

JSON objects are canonical code-owned structures. JSONL pages end with exactly one
newline per record. Unknown fields, alternate JSON encodings, alternate ZIP metadata,
unsafe paths, duplicate paths, and trailing data fail verification.

Current hard structural limits are:

- 64 ordered records per page;
- 8 MiB per page;
- 2 MiB per content-addressed blob;
- 256 MiB per logical chunked blob;
- 512 MiB per container;
- 1,000,000 sealed source records;
- 1,000,000 derived events;
- at most 100 events between public knowledge checkpoints.

A later limit change requires a protocol review. It is not an automatic fallback for
an oversized run.

## Public base manifest

The `omnidex-replay/v2` manifest binds:

- one strict terminal authority: either an exact sealed episode (episode ID, seal
  digest, trace digest, start time, and seal time) or one durable pre-episode Brain
  bootstrap failure (requested episode ID, actor, failure authority, receipt, and raw
  provider evidence);
- the digest of that complete terminal authority;
- public world and public run authority digests;
- semantic coverage status;
- ordered source, event, checkpoint, and chunk-manifest index digests;
- record and blob counts;
- every page/blob path, class, byte count, range, and digest;
- one derived mapping record for every sealed source kind present.

The currently implemented status is `structural_only`. Its generic construction input
can carry arbitrary opaque bytes and `private_data: false` is therefore not a security
proof. It is useful only for validating
container identity, ordering, paging, source citations, public checkpoint replay, and
private separation. It is permanently ineligible for a serious execution claim.

## Bounded large evidence

An exact source body larger than 2 MiB is never placed in an oversized blob and is
never omitted. Code reads one exact regular file through one open handle, rejects
symlinks, and verifies inode, size, and modification time before and after the read.
It then creates a canonical `omnidex.replay-chunked-blob.v1` manifest over fixed
2 MiB chunks (with one bounded final chunk).

The manifest binds a code-owned ID, logical media type, privacy role, total byte
count, complete-content digest, chunk count, and every ordered chunk ordinal,
offset, byte count, media type, and digest. The manifest is itself a
content-addressed JSON blob cited by a source/event/frame. Its chunks are ordinary
content-addressed blobs. Verification reassembles and rehashes the exact bytes and
rejects a missing, extra, duplicated, reordered, altered, self-referential, or
orphaned manifest/chunk.

Public bases accept only `public_agent_knowledge` manifests. Private overlays accept
only `private_world` manifests. The role is inside the hashed manifest, so a
manifest cannot be moved across that boundary by changing container metadata.

## Canonical navigation clock

The required replay clock is the event sequence: `1, 2, 3, ...`.

The production sealed trace does not provide one universal timestamp for every
record. Replay therefore must not invent `time_ms`, distribute wall time across
events, substitute zero, or copy an unrelated timestamp.

A future semantic adapter may attach optional timing only when it decodes a registered
typed timestamp from one exact cited source record. The timing binds that source,
the exact timestamp, and exact nanoseconds elapsed from the sealed episode start. It
must fall between episode start and seal. Structural events reject timing entirely.
If universal wall-time animation becomes necessary, it requires a production trace
schema migration.

## Sealed sources and derived events

Each sealed source record preserves the production order tuple:

```text
(call_ordinal, phase, sequence, kind, id)
```

It binds the exact payload as a content-addressed blob. A large payload instead
binds its role-specific chunk manifest, whose exact ordered chunks close the same
source. Reordered tuples, duplicate identities, changed payloads, missing blobs, and
orphan sources fail.

Every derived event has:

- one canonical event sequence;
- one registered task-neutral event kind;
- one mapping schema;
- zero or one public revision;
- one or more exact source-record references;
- one bounded content-addressed payload;
- a previous-event digest and its own digest.

Registered event distinctions include world start/transition, goal activation and
terminal state, observation and evidence acquisition, accepted fact materialization,
hypothesis creation/rejection, accepted/rejected decision, Working Set
attach/release/reacquisition, action selection, obligation change, plan revision,
Context Projection, model call and disposition, provider process observation,
provider-request disposition, failure, restart, lease change, stale-write rejection,
episode cancellation, and episode seal.

Fact acceptance is not generic evidence acquisition. Decision rejection is not a
hypothesis rejection. Episode sealing is not goal satisfaction. The protocol keeps
those epistemic and lifecycle boundaries separate.

## Semantic completeness gate

Structural replay carries changing source payloads opaquely and labels their mapping
`structural_opaque`. That mapping is never serious evidence.

Before the first serious maze execution, code must register an exhaustive mapping for
every source kind that can occur in the frozen sealed production trace. A mapping may:

- decode one exact typed payload schema into one or more registered cognitive events;
  or
- deliberately preserve a source as registered opaque evidence when exposing its
  bytes would add no semantic state.

Both cases retain the source kind, ID, digest, payload blob, and event citations.
Unknown kinds, missing mappings, schema drift, invalid typed payloads, uncited source
records, or private fields fail the eventual serious export. That exporter must not
accept the generic structural `BaseInput`; it accepts only the verified sealed
production trace and exact public bundle. There is no generic log parser and no
best-effort event.

Migration 060 is changing exact provider identity, dispatch, response, and usage
evidence. The replay core therefore binds those records opaquely today. The semantic
model-call/provider adapters cannot freeze until 060 freezes its Go and SQL schemas.
Until that work lands, `RequireSeriousExecution` fails with the typed incomplete-
mapping error.

## Public agent-knowledge checkpoints

The base stores only what Omnidex could know at the event boundary. A checkpoint may
contain public revisions and typed entries for goals, observations, evidence,
beliefs, obligations, Working Set items, Context Projections, and failures. Each entry
binds status, epistemic authority, content blob, and the exact events supporting it.

The first checkpoint is explicit empty public knowledge at event zero. Every later
checkpoint contains one forward delta covering the exact event interval since the
previous checkpoint. Upserts and releases cite events in that interval. Code applies
the delta and requires exact equality with the next full checkpoint. The final
checkpoint covers the final event.

These checkpoints support deterministic scrubbing without replaying an entire long
episode from zero. They cannot contain hidden world state.

## Private truth overlay

The `omnidex-replay-private/v2` manifest binds the exact base replay digest, terminal
authority digest, oracle artifact digest, and evaluation artifact digest. It requires one
exact oracle source and one exact evaluation source. World-truth events cite only
oracle/private-world sources; separately typed evaluation events cite the evaluation
source. Both event classes are required. Private frames partition that event stream
into bounded snapshots and deltas.

The base verifier rejects the private schema and private event kinds. The overlay
verifier requires the exact base bytes, not only a caller-supplied terminal identity. A
different, reordered, altered, or missing base fails.

A private overlay requires the base terminal authority to be a sealed episode. A
pre-episode provider failure has no hidden-world execution or evaluator episode, so
attaching an oracle or evaluation overlay to it fails loudly.

## Verifier obligations

Verification fails on any:

- byte-noncanonical container or JSON representation;
- altered, reordered, duplicated, missing, or extra container entry;
- changed manifest/page/blob digest or byte count;
- reordered or duplicated sealed source record;
- derived event without exact existing source references;
- sealed source kind without derived mapping coverage;
- event-chain or checkpoint-chain divergence;
- checkpoint gap, oversized interval, or delta/snapshot mismatch;
- missing or orphan content-addressed blob;
- missing, extra, reordered, role-mismatched, or non-reconstructable chunked blob;
- private manifest/event data in the public base;
- private overlay with the wrong base, episode seal, oracle, or evaluation;
- structural replay presented as serious-execution evidence.

## Remaining integration sequence

1. Freeze migration 060 provider/process/call evidence.
2. Add one cognition-gauntlet adapter that reads the sealed production trace through
   its existing bounded page API and the exact public inference bundle. No alternate
   trace reader is permitted.
3. Register and test exhaustive semantic mappings for all frozen source kinds,
   including explicit provider/process/call dispositions.
4. Derive public knowledge state and deltas from typed events, then prove source-kind,
   event, checkpoint, and blob closure over real sealed database fixtures.
5. Make every serious Matrix, Resume, Transfer, Scale, and later Rogue child-process
   receipt bind a verified base replay. The evaluator separately binds the private
   overlay after stop.
6. Remove the structural-only execution gate only in the same change that proves all
   mappings and process integration. Individual and global promotion flags remain
   false.

No UI, animation renderer, fog-of-war view, or ghost-race comparison is part of this
milestone. Those are projections over the portable protocol after its authority is
complete.
