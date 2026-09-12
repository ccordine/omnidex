# Roadmap

The target is functional, verified completion of the user's current objective
through code-owned workflow and narrowly bounded semantic calls. Hash receipts,
review activity, and preserved internal history are not completion measures.

## Verified framework foundations

- One current internal database setup; service startup recreates its dedicated
  schema rather than migrating or preserving earlier internal state.
- Records of actual model calls, verification commands, database reads, and web
  acquisitions, with job-owned retrieval and citation checks.
- Exact-value simulation preparation, narrative freshness, atomic publication,
  and replay without repeated effects or content fingerprints.
- Fresh bounded workspace/map displays without persistent index files or hashed
  workspace identities.
- Workspace-local technical package names independent of host directory names,
  with product titles preserved separately.

The scope and limitations of these tests are recorded in
[EVIDENCE_LEDGER.md](EVIDENCE_LEDGER.md) and
[CHARMANDER_PROOF.md](CHARMANDER_PROOF.md). Passing framework tests does not
establish autonomous construction of an unfamiliar application.

## Remaining work

- Finish auditing current semantic calls and their actual model-visible inputs
  against the one-unresolved-question boundary.
- Finish reconciling architectural documents with the actual current-value
  runtime contracts and measured implementation limits.
- Demonstrate the ordinary production request boundary through a fresh,
  unsteered build and evaluate its resulting user-visible behavior only after
  the build stops.

The former hash-backed index, command cache, failure-fingerprint, and local
ledger/trace CLI tracks are retired, not features to restore. Current CLI
entrypoints are `omni chat` and `omni version --json`; explicit API transports
own other supported operations.
