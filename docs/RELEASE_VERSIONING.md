# Release Versioning

Omnidex uses pride release codenames based on National Dex order.

| Release | Codename | National Dex | Meaning |
| --- | --- | ---: | --- |
| `v0.1.0-alpha` | Bulbasaur | 001 | First alpha release. |
| `v0.2.0` | Ivysaur | 002 | Growth release — memory categories, providers, evidence/playbooks. |
| **`v0.3.0`** | **Venusaur** | 003 | **Current release** — augmented project planner, draft queue, scrum board, human-in-the-loop agent execution. |
| future | Charmander | 004 | Next planned maturity line. |

Notes:

- Use the official spelling **Venusaur** (not "venasaur").
- The release codename is embedded in binaries through `internal/version` and `scripts/build-release.sh`.
- Patch releases keep the same codename unless the release meaning changes substantially.
- Major maturity jumps follow the National Dex progression instead of arbitrary codenames.

## Venusaur (`v0.3.0`) theme

Venusaur marks the shift from "agent runtime only" to **plan → review → execute**:

1. **Project Chat** researches and drafts work (software, learning, creative epics).
2. **Draft queue** holds suggestions until you approve them.
3. **Scrum board** is the execution queue you control.
4. **Play** runs build agents on Ready cards with evidence-led loops underneath.

See [SCRUM_PLANNER.md](SCRUM_PLANNER.md) for the full workflow.
