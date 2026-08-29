# Artifact-adapter contract

An artifact adapter is code-owned deterministic support for one file class.
Code selects it from a normalized path; models never select adapters, parsers,
commands, or validation operations.

Every adapter registers exactly one executable leaf-validation mechanism:

- `parse`: code parses the complete bytes with the adapter's registered
  grammar or structured-document parser; or
- `structural_validate`: code proves only the narrower structural properties
  implemented by the registered validator.

These are executable registrations, not capability labels. Omnidex does not
infer AST, scope, typecheck, syntax-check, project-test, or runtime authority
from a suffix or from metadata. Those mechanics exist only where a concrete
language validator, source composer, stack compiler, or exact verification
command implements them.

After code assembles files in memory, every path is resolved through the
selected stack and its exact bytes pass the leaf validator. Stack-wide
cross-file invariants and exact verification commands then run separately. The
same leaf and assembly checks run again at the authoritative write gate.

## Neutral source nodes and composition

The construction graph uses `SourceBlueprint`, `SourceDocument`, and
`SourceBlock`. A document owns one normalized path, adapter identity, code-owned
preamble, and ordered blocks. A block has exactly one static or generated
authority, one bounded API, explicit direct dependencies, and optional
code-owned frozen-task ownership and source role.

A generated-source call receives one exact signature, local behavioral
contract, direct declarations, and permitted symbols. It never receives the
document path. Code validates the returned node, inserts it into the neutral
document, composes the complete bytes, records spans, and invokes the selected
stack's compiler and tests.

Code binds every accepted target, compiled document, and static-file path into
the exact artifact-identity provenance boundary before the first generated
source call. The boundary is rebuilt and revalidated mechanically as paths are
added; it is not reconstructed from model text or filename heuristics.

TypeScript/TSX, Go, JavaScript, Rust, Java, PHP, and unstructured plain text have focused document
composers. Parse-only or structural-only leaves cannot enter source composition
without such a composer. Adding a language requires a real fragment boundary,
composer, stack compiler, and verification implementation; it does not widen a
universal coding prompt.

## Registered leaf validation

This block is checked against the executable registry by tests.

<!-- BEGIN ARTIFACT_ADAPTER_REGISTRY -->
| Adapter | Executable leaf validation |
| --- | --- |
| `cargo_toml` | `parse` |
| `composer_lock` | `parse` |
| `css_tailwind` | `structural_validate` |
| `dockerfile` | `parse` |
| `environment_example` | `parse` |
| `go` | `parse` |
| `go_module` | `parse` |
| `html` | `parse` |
| `java` | `parse` |
| `javascript` | `parse` |
| `nginx` | `parse` |
| `php` | `parse` |
| `php_executable` | `parse` |
| `plain_text` | `structural_validate` |
| `postgresql_migration` | `structural_validate` |
| `rust` | `parse` |
| `structured_json` | `parse` |
| `structured_yaml` | `parse` |
| `typescript` | `parse` |
| `typescript_react` | `parse` |
<!-- END ARTIFACT_ADAPTER_REGISTRY -->

The corresponding path classes are focused and non-overlapping:

| Adapter | Deterministic path class |
| --- | --- |
| `composer_lock` | `composer.lock` |
| `typescript_react` | `.tsx` and `.test.tsx` |
| `typescript` | `.ts` and `.test.ts` |
| `go` | `.go` and `_test.go` |
| `go_module` | `go.mod` |
| `php` | `.php`, with `Test.php` recognized as verification |
| `php_executable` | the code-owned root `artisan` executable |
| `javascript` | `.js`, `.jsx`, `.mjs`, `.cjs`, including test/spec variants |
| `css_tailwind` | `.css` |
| `html` | `.html` and `.test.html` |
| `java` | `.java`, with `Test.java` recognized as verification |
| `rust` | `.rs` and `_test.rs` |
| `cargo_toml` | `Cargo.toml` and `Cargo.lock` |
| `nginx` | `nginx.conf` and NGINX `.conf` leaves |
| `dockerfile` | Dockerfiles and Docker Compose YAML |
| `structured_json` | `.json` |
| `structured_yaml` | non-Compose `.yaml` and `.yml` |
| `environment_example` | `.env.example` |
| `plain_text` | `.txt` (case-insensitive extension), `.gitignore`, and `.dockerignore` |
| `postgresql_migration` | normalized `database/migrations/*.sql` leaves |

CSS/Tailwind validation is intentionally labeled structural: the leaf validator
checks bounded text structure, while the complete TypeScript and HTML-producing
PHP stacks run the real CSS toolchain during their locked build. It does not
claim a standalone CSS grammar it does not possess.

## Registered project stacks

A project stack is the code-owned compiler, source-ownership rules, static
artifacts, assembly validator, stage executor, and exact verification command
set for one or more explicitly registered application surfaces. Supported and
default surfaces are validated sets; stack fields contain executable hooks, not
duplicated capability claims.

Tool and language versions are separate code-owned project-version profiles.
Every profile belongs to exactly one stack and binds its source dialect, parser
qualification, manifest compatibility rule, runtime probes, dependency and lock
authority, generated static values, and complete-assembly validation. Existing
manifests may select a profile only when exactly one registered compatibility
rule matches. For a fresh project, the existing bounded technical-format
station chooses one opaque code-enumerated stack/profile candidate only when
the accepted authority explicitly requires it; `UNCONSTRAINED` receives the
surface's default stack and that stack's explicit default profile. The selected
profile identity is mapped by code and then retained by the target tree,
compiled program, task projection, and in-memory assembly; it is never model
output.

Parser qualifications execute real leaf-parser probes and enumerate the exact
source-dialect labels they prove. Before any source inference, code also runs
only the profile's bounded, allowlisted runtime-version probes. Initial source
stations receive the one selected dialect label; repair execution remains blind
to it and receives only its already-derived repair instruction and mutable node.
Manifests, compiler flags, runtime guards, container images, package versions,
and lock formats are generated from the selected profile rather than from stack
conditionals.

Version support is deliberately closed and additive. A new compatible version
is registered as another independently validated profile for the existing
technical stack. Code projects it into the same bounded opaque technical-format
candidate set; it does not require another station or a central version switch.
An unknown, malformed, or ambiguously matching manifest,
an unqualified parser dialect, a runtime outside the registered constraint, or
a lock graph without exact integrity evidence fails explicitly. Omnidex does not
claim that an unobserved future syntax or toolchain is compatible.

This block is also checked against the executable registry by tests.

<!-- BEGIN PROJECT_STACK_REGISTRY -->
| Stack | Supported surfaces | Default surfaces |
| --- | --- | --- |
| `go_command_line_capabilities_v1` | `command_line_application` | `command_line_application` |
| `java_command_line_capabilities_v1` | `command_line_application` | `none` |
| `javascript_command_line_capabilities_v1` | `command_line_application` | `none` |
| `laravel_http_service_capabilities_v1` | `browser_application, service_application` | `none` |
| `php_http_service_capabilities_v1` | `browser_application, service_application` | `service_application` |
| `rust_command_line_capabilities_v1` | `command_line_application` | `none` |
| `typescript_browser_capabilities_v3` | `browser_application` | `browser_application` |
<!-- END PROJECT_STACK_REGISTRY -->

<!-- BEGIN PROJECT_VERSION_PROFILE_REGISTRY -->
| Version profile | Stack | Source dialect | Manifest evidence | Stack default |
| --- | --- | --- | --- | --- |
| `go_command_line_versions_v1` | `go_command_line_capabilities_v1` | `Go 1.24.0` | `go.mod` | `yes` |
| `java_command_line_versions_v1` | `java_command_line_capabilities_v1` | `Java 21 source and class-file API release` | `none` | `yes` |
| `javascript_command_line_versions_v1` | `javascript_command_line_capabilities_v1` | `ECMAScript 2022 modules on Node.js >=22.0.0` | `package.json` | `yes` |
| `laravel_http_service_versions_v1` | `laravel_http_service_capabilities_v1` | `PHP 8.3.30 function syntax` | `composer.json, composer.lock` | `yes` |
| `php_http_service_versions_v1` | `php_http_service_capabilities_v1` | `PHP >=8.2,<9 function syntax` | `composer.json` | `yes` |
| `rust_command_line_versions_v1` | `rust_command_line_capabilities_v1` | `Rust 2024 edition with rust-version 1.85` | `Cargo.toml` | `yes` |
| `typescript_browser_versions_v1` | `typescript_browser_capabilities_v3` | `TypeScript 5.9.3 with TSX react-jsx targeting ECMAScript 2022` | `package.json` | `yes` |
<!-- END PROJECT_VERSION_PROFILE_REGISTRY -->

- The TypeScript browser stack supplies React runtime/shell, HTML entrypoint,
  Tailwind CSS v4 through its pinned Vite plugin and code-owned CSS import,
  integrity-locked npm dependencies, strict source policy, isolated
  acceptance/runtime tests, typechecking, and a CSS-producing production build.
- The Go CLI stack supplies a dependency-free module/runtime/entrypoint and runs
  focused tests, `go test -count=1 ./...`, `go vet ./...`, and `go build ./...`.
- The Laravel HTTP stack supplies Laravel 13's application/router lifecycle,
  server-rendered responses, optional canonical PostgreSQL durable state,
  NGINX, digest-pinned Docker stages, exact Composer lock authority, and a
  collision-free configurable host binding. Its application and NGINX health
  checks traverse the exact reserved Laravel readiness route. Its registered
  release provenance is the official
  [`laravel/framework` `v13.29.0`](https://github.com/laravel/framework/releases/tag/v13.29.0)
  tag and official
  [`laravel/laravel` `v13.10.1`](https://github.com/laravel/laravel/releases/tag/v13.10.1)
  tag (both published 2026-08-25); the exact framework lock resolves commit
  `6e2c363716964d8238cee7097b258119a984f0cf`. Its pinned PostgreSQL 18
  volume targets `/var/lib/postgresql`, as required by the
  [Docker Official Image PostgreSQL 18 `PGDATA` contract](https://github.com/docker-library/docs/blob/master/postgres/README.md#pgdata).
- The JavaScript CLI stack supplies strict ESM runtime/entrypoint and runs Node
  with exact permissions, strict result normalization, tests, and syntax checks.
- The Rust CLI stack supplies a dependency-free locked Cargo project and runs
  focused tests plus locked, offline test/check/build verification.
- The Java CLI stack supplies runtime, entrypoint, reflection-free test runner,
  strict `javac`, focused assertions, and an executable application archive.
- The PHP service stack supplies typed PHP runtime/router, bounded server-rendered
  HTML leaves where required, conditional Tailwind assets, digest-pinned
  Node/Composer/NGINX images, an exact Docker context, NGINX configuration, PHP
  lint/tests, Docker Compose build/config checks, live NGINX/app startup, typed
  real HTTP verification for every endpoint, and deterministic cleanup. When
  the accepted lifetime leaf requires cross-request authority, code additionally
  supplies one PostgreSQL schema, migration runner, task-scoped state facade,
  persistent volume, and cross-process verification; request-local projects omit
  those artifacts and services entirely.

One complete target-tree resolution covers the frozen workload under the
selected stack. When naming remains semantically unresolved, the model returns
only one raw hierarchy of directory and file basenames; code alone constructs
normalized relative paths from that hierarchy. No current stack needs that
call. TypeScript/React allocates one neutral numbered TSX source/test pair for
the whole workload; the command-line and PHP stacks allocate their registered
per-task pairs. Each allocator treats the pair atomically, checks regular-file
ancestry separately from existing directories, and cannot claim a reserved or
static leaf. Code retains task provenance, builds the union and coverage graph,
generates all task-neutral static files, and rejects any path or artifact class
outside that stack. An unsupported surface, state lifetime, or technical format
fails loudly; there is no fallback stack.
