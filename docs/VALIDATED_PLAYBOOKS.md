# Validated Playbooks

Validated Playbooks are procedural memories learned from accepted work.

They are different from ordinary fact memory. A fact memory might say a project uses Go. A validated playbook records a workflow that actually completed under command evidence and validator acceptance.

## Flow

1. A task succeeds.
2. The objective ledger has no pending blocking objectives.
3. The command observations include successful implementation and verification commands.
4. Omnidex extracts a reusable procedure candidate.
5. The playbook is stored as `validated_playbook` memory with procedure tags.
6. Future similar tasks can retrieve the playbook as advisory context.
7. Current scope, workspace evidence, proof plans, command policy, and validators still decide every executable step.

## Stored Shape

Each playbook records:

- task pattern
- required context
- successful command sequence
- validation commands or signals
- known failure modes
- recovery steps
- objective and success evidence
- model/provider used
- duration
- confidence score
- supersession fields for future versioning
- last successful use

## Scope Rule

Validated Playbooks accelerate execution. They do not authorize execution.

A remembered React notes app workflow can suggest inspecting `package.json`, editing `src/App.jsx`, creating components, and running `npm run build`. It cannot force Tailwind, routing, Docker, cloud sync, authentication, or any dependency that the current user request, selected recipe, worksite evidence, or prerequisite objective does not justify.

## Versioning Direction

The intended lifecycle is:

- `Validated Playbook v1`: learned from a successful run
- `Validated Playbook v2`: updated by a later run with fewer failures, faster completion, or better validation
- older versions remain historical but are not preferred

Reuse should always remain evidence-led: retrieve, adapt to current worksite, execute under validation, then improve the playbook only when a later accepted run proves a better workflow.
