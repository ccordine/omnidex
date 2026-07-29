# Release Versioning

Omnidex uses pride release codenames based on National Dex order.

| Release | Codename | National Dex | Meaning |
| --- | --- | ---: | --- |
| `v0.1.0-alpha` | Bulbasaur | 001 | First alpha release. |
| `v0.2.0` | Ivysaur | 002 | Growth release — memory categories, providers, evidence/playbooks. |
| `v0.3.0` | Venusaur | 003 | Augmented project planner, draft queue, scrum board, human-in-the-loop agent execution. |
| **`v0.4.0`** | **Charmander** | 004 | **Current development release** — deterministic AST assembly line, typed semantics, isolated function transforms, and concise live execution. |
| future | Charmeleon | 005 | Next planned maturity line. |

Notes:

- Use the official spellings **Venusaur** and **Charmander**.
- The release codename is embedded in binaries through `internal/version` and `scripts/build-release.sh`.
- Patch releases keep the same codename unless the release meaning changes substantially.
- Major maturity jumps follow the National Dex progression instead of arbitrary codenames.

## Charmander (`v0.4.0`) theme

Charmander replaces the broad, self-confusing coding agent with a server-owned assembly line:

1. Two tiny path-blind semantic calls emit a five-field behavior classification and one shape-specific label seed, never an implementation plan or expanded contract.
2. Code validates and expands the seed, selects an adapter, and owns every type, relationship, document, declaration, dependency, import, test, command, and mutation.
3. Models are optional isolated function transformers and receive no file or orchestration identity.
4. The complete graph is composed and tested in an isolated stage before authoritative writes.
5. Exact failures can correct only one declared generated owner while all other accepted nodes remain intact.
6. Completion is code-authoritative and requires exact workspace reconciliation plus current successful tests.

The initial unattended three-application baseline passes; its exact measurements and explicit limits are in [CHARMANDER_PROOF.md](CHARMANDER_PROOF.md). This proves the three registered Go CLI shapes, not arbitrary-project autonomy. See [`internal/worker/RUNTIME.md`](../internal/worker/RUNTIME.md) for the execution contract. The Venusaur planner remains documented in [SCRUM_PLANNER.md](SCRUM_PLANNER.md).
