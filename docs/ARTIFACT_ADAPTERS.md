# Artifact-adapter contract

An artifact adapter is code-owned deterministic support for one file class. It
is selected from the normalized current file path, never by a model. The
adapter declares only mechanics code can actually perform for that class:

* `parse`
* `ast`
* `scope`
* `typecheck`
* `syntax_check`
* `project_test`
* `runtime_verify`

No capability is implied by a file extension. An adapter that lacks scope or
type checking must not manufacture those facts for a model. It exposes only its
real deterministic evidence and leaves the remaining bounded semantic question
to the relevant station.

## Registered leaf classes

| Adapter | Deterministic path class | Declared mechanics |
| --- | --- | --- |
| `typescript_react` | `.tsx`, `.test.tsx` | parse, AST, scope, typecheck, project test, runtime verify |
| `typescript` | `.ts`, `.test.ts` | parse, AST, scope, typecheck, project test |
| `go` | `.go`, `_test.go` | parse, AST, scope, typecheck, project test |
| `php_laravel` | `.php`, `Test.php` | parse, syntax check, project test |
| `blade_html` | `.blade.php` | parse, runtime verify |
| `javascript_stimulus` | `.js`, `.test.js` | parse, AST, syntax check, project test |
| `css_tailwind` | `.css` | parse, syntax check, runtime verify |
| `html` | `.html`, `.test.html` | parse, runtime verify |
| `java` | `.java`, `Test.java` | parse, AST, typecheck, project test |
| `nginx` | `nginx.conf`, NGINX `.conf` | parse, syntax check, runtime verify |
| `dockerfile` | `Dockerfile`, Docker Compose YAML | parse, syntax check, runtime verify |
| `structured_json` | `.json` | parse |
| `structured_yaml` | `.yaml`, `.yml` not claimed by Compose | parse |
| `environment_example` | `.env.example` | parse |

## Project stacks

A project stack is distinct from a leaf adapter. It is the code-owned set of
artifact adapters plus assembly and verification machinery that can complete a
whole application surface. The tree station receives one explicitly selected,
registered stack as technical context and returns paths compatible with it.

An artifact adapter may be useful in an existing mixed-stack project before it
belongs to a complete greenfield stack. Omnidex must fail explicitly if a
requested complete build has no registered project stack; it must never prompt
the tree model to pick an unimplemented runtime or silently substitute another
stack.

Adding a new PHP/Laravel, Java, Docker, NGINX, or other project stack requires
the corresponding code-owned assembler, parser/validator integration,
diagnostic locator, local-context projection, splice logic, and verification
commands. It does not require changing the target-tree model boundary or
creating a universal coder prompt.
