# Artifact-adapter contract

An artifact adapter is code-owned deterministic support for one file class.
Code selects it from a normalized path; models never select adapters, parsers,
commands, or validation operations.

A registered leaf validator performs either a real parse or its explicitly
narrower structural check. A recognized suffix does not establish source-generation,
typechecking, project-test, or runtime support. Those require a concrete composer,
stack compiler, and executed verification path.

## Source construction

The construction graph uses `SourceBlueprint`, `SourceDocument`, and
`SourceBlock`. Code owns paths, preambles, declarations, ordered blocks,
dependencies, task ownership, and the direct symbol projection.

One source call receives the selected dialect, exact declaration as lexical scope,
one local behavior, and only required direct declarations. It returns ordinary
implementation-body text. Code supplies every structural byte, validates the node,
composes the document, retains source spans, and invokes the stack's actual checks.

The path-identity boundary covers accepted targets, compiled documents, and static
files before source generation. It keeps file identities out of source-model context;
it is not a content hash, execution receipt, or alternate ownership registry.

TypeScript/TSX, Go, JavaScript, Rust, Java, and unstructured plain text have focused
composers. A composer by itself does not establish a complete application workflow.
Adding a language must not widen the source station into a whole-file generator.

After a real body defect, code must establish the exact mutable span and necessary
semantic question before continuing the same persisted source job and model route.
The result is replacement text for that span. Code compares the actual retained
base, splices only the span, and reruns validation. Surrounding accepted source,
paths, raw diagnostics, and preservation instructions are not correction context.

## Registered leaf validation

These tables are compared with the executable registries by
[the registry documentation test](../internal/worker/v3_registered_adapter_documentation_test.go).
They describe registrations, not proof that a workload was built.

<!-- BEGIN ARTIFACT_ADAPTER_REGISTRY -->
| Adapter | Executable leaf validation |
| --- | --- |
| `cargo_toml` | `parse` |
| `css_tailwind` | `structural_validate` |
| `go_module` | `parse` |
| `go` | `parse` |
| `html` | `parse` |
| `java` | `parse` |
| `javascript` | `parse` |
| `plain_text` | `structural_validate` |
| `rust` | `parse` |
| `structured_json` | `parse` |
| `typescript_react` | `parse` |
| `typescript` | `parse` |
<!-- END ARTIFACT_ADAPTER_REGISTRY -->

The [recognizers](../internal/worker/v3_artifact_adapter.go) own exact path classes,
including verification suffixes. Plain text covers normalized `.txt` leaves and
`.gitignore`; it does not turn arbitrary unknown files into supported source.
CSS/Tailwind's leaf check is structural. The browser stack's complete build runs
the real CSS toolchain; the leaf does not claim a standalone CSS grammar.

There are no registered PHP, Composer, NGINX, Docker, YAML, environment-file, or
PostgreSQL-migration adapters in the current build. Historical tables that listed
them are not runtime capabilities.

## Registered project stacks

A stack supplies executable source construction, ownership checks, static files,
assembly validation, and the relevant verification path. Supported surfaces come
from the registry, not duplicated capability metadata.

<!-- BEGIN PROJECT_STACK_REGISTRY -->
| Stack | Supported surfaces |
| --- | --- |
| `go_command_line_capabilities_v1` | `command_line_application` |
| `java_command_line_capabilities_v1` | `command_line_application` |
| `javascript_command_line_capabilities_v1` | `command_line_application` |
| `rust_command_line_capabilities_v1` | `command_line_application` |
| `typescript_browser_capabilities_v3` | `browser_application` |
<!-- END PROJECT_STACK_REGISTRY -->

No supported stack for the accepted surface is an explicit failure before model
resolution. Natural-language technical constraints are a separate semantic question:
the constraint station sees only the redacted request and technical-format choices,
including unconstrained and unsupported alternatives. Even one registered format
does not mechanically establish that it satisfies an explicit request. Code maps
the opaque response; the unconstrained result uses the first registered compatible
format, while the unsupported result fails. Once code has actually resolved a sole
applicable alternative, it does not call a model merely to select that value again.

## Version profiles

A profile retains code-owned technical component values for one stack. Code derives
its source-dialect label, generated static values, and runtime-version checks.
Unknown components and incompatible observed versions fail explicitly. The current
selection path does not inspect existing manifests to resolve a different profile.

<!-- BEGIN PROJECT_VERSION_PROFILE_REGISTRY -->
| Version profile | Stack | Source dialect |
| --- | --- | --- |
| `go_command_line_versions_v1` | `go_command_line_capabilities_v1` | `Go 1.24.0` |
| `java_command_line_versions_v1` | `java_command_line_capabilities_v1` | `Java 21 source and class-file API release` |
| `javascript_command_line_versions_v1` | `javascript_command_line_capabilities_v1` | `ECMAScript ES2022 modules on Node.js >=22.0.0` |
| `rust_command_line_versions_v1` | `rust_command_line_capabilities_v1` | `Rust 2024 edition with rust-version 1.85` |
| `typescript_browser_versions_v1` | `typescript_browser_capabilities_v3` | `TypeScript with TSX react-jsx targeting ECMAScript ES2022` |
<!-- END PROJECT_VERSION_PROFILE_REGISTRY -->

The actual profile values and [toolchain checks](../internal/worker/v3_project_toolchain_version.go)
determine compatibility. A declared version, image identifier, or dependency digest
is not evidence that the resulting application compiled or worked.

## Verification and limits

The TypeScript browser stack provides React composition, locked npm installation,
tests, typechecking, and a Vite build with the pinned Tailwind plugin. Generated
source cannot invoke arbitrary browser host APIs; no host-capability wrapper or
application host-driver workflow is registered.

The command-line stacks provide their task-neutral runtime and entrypoint.
Go uses tests, vet, and build. JavaScript syntax-checks every assembled module,
loads the implementation graph, and runs the exact task-owned Node test files.
Rust runs locked offline Cargo library checks and task-owned tests, then builds
the complete application. Java uses strict compilation for the selected release,
runs each owned test class with assertions enabled, and creates the complete
application archive. These commands run
in task-local staging, complete staging, and the authoritative workspace; no
registered source executor may omit those verification hooks.

JavaScript owns the test registration and an observation callback which invokes
and normalizes the owned implementation result. Its bounded verification-body
call receives the accepted requirement and code-owned input/result declarations,
not implementation source or declarations. Direct dependency meaning remains
available without exposing its implementation. Code requires one literal-input observation
followed by direct strict assertions against independent expected values; empty,
unreachable, swallowed, self-comparing, or detached assertions fail validation.
The same checks apply in staging and at the authoritative write boundary.

Rust owns one native `#[test]` registration per task module and gives its separate
verification-body call only the accepted requirement, individually declared input,
result and direct-result-map types, an observation function, and `assert_eq!`.
Code parses macro operands and rejects non-executed or self-dependent assertions.
The standard Cargo test runner executes the observations in task-local, complete,
and authoritative verification. No implementation body or declaration enters the
test-generation context, and no reviewer or approval call is added.

Java owns one test class and native assertion runner per task. Its separate
ordinary-text verification body receives only the accepted requirement and one
normalized observation declaration, including the code-owned input/result fields.
Code validates literal inputs and independent value comparisons; `java -ea` executes
the exact owned class. The runner fails explicitly if assertions are disabled.
Implementation calls receive separate result-constructor and, only when needed,
direct-dependency declarations. No implementation source reaches test generation.
Compilation and module loading alone do not prove behavior. Only recorded commands
that actually ran establish their corresponding result.

Current stacks derive their target trees mechanically, with no tree-model call.
TypeScript allocates one neutral source/test pair for the workload. Go,
JavaScript, Rust, and Java allocate task-local pairs with code-owned compiler
support. Code retains coverage, checks
collisions and protected paths, and assembles the required static files. Any future
semantic naming need remains confined to [the raw-tree boundary](TARGET_TREE_PLANNING.md).
Omission alone never authorizes deletion.

Every assembled file passes its selected leaf validator and the stack's current
checks before the authoritative write gate. Code validates again before mutation
and reruns those checks against the actual workspace afterward. Compiler output
and caches remain in owned temporary directories which are removed on success or
failure; source bytes are checked directly after commands, without content hashes.
Unsupported surfaces or artifacts fail; no old adapter, universal coder, or guessed
fallback remains available.

Framework tests establish only their measured scope. An autonomous-build claim
requires [the unsteered production proof](CHARMANDER_PROOF.md), not a registry row.
