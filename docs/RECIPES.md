# Recipes

Recipes are declarative task manifests. They move task-specific workflow knowledge out of the command loop and into reviewable data.

Current recipe manifests live in `recipes/*.json`.

## Purpose

A recipe can define:

- stable objective IDs
- objective descriptions
- objective dependencies (`depends_on`) for DAG-shaped workflows
- allowed command classes
- required evidence
- completion probes

The prompt interpreter can map a user request to a recipe. The deterministic core can then seed the objective ledger, constrain commands, and check completion without adding another task-specific branch to the runtime.

When a selected recipe has `completion_checks`, Omnidex can run those probes and satisfy the recipe objective ledger without another model call when all checks pass.

## Example

`recipes/frontend.stimulus-tailwind-recyclr.json` defines a small npm frontend project workflow:

- initialize npm
- install or verify dependencies
- create page and source files
- wire Stimulus
- account for RecyclrJS
- configure webpack
- verify a bundle

## Contract

Recipe IDs should be stable and namespaced:

```text
frontend.stimulus-tailwind-recyclr
go.cli-demo
docker.smoke
repair.broken-tests
```

Objective IDs should be stable snake_case identifiers. Completion evidence should be observable from commands, files, tests, or logs.

Objective dependencies must form a DAG. The runtime validates unknown dependencies, self-dependencies, and dependency cycles when loading recipes.

Recipes should not contain natural-language prompt matching rules. Prompt interpretation is a specialist job; recipe execution is deterministic core work.
