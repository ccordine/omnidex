# Omnidex integration SDKs

The SDKs expose the same authenticated, versioned control-plane API in five languages:

| Runtime | Package directory | Minimum runtime | HTTP implementation |
| --- | --- | --- | --- |
| Go | [`go`](go) | Go version from the root module | `net/http` |
| JavaScript | [`javascript`](javascript) | Node.js 20 | Web Fetch |
| PHP | [`php`](php) | PHP 8.2 | native streams, injectable transport |
| Java | [`java`](java) | Java 21 | JDK `HttpClient` |
| Rust | [`rust`](rust) | Rust 2021 | blocking Reqwest with Rustls |

Each client can register direct or delegated PostgreSQL data sources, create and fetch assistant channels, submit a message, page the typed transcript, and fetch job state. Successful responses reject unknown fields where the runtime permits exact decoding. Requests have bounded timeouts and responses have a 16 MiB hard limit. Redirects are forbidden by every built-in transport.

## Server configuration

Set one server-side secret before starting Omnidex:

```dotenv
OMNIDEX_INTEGRATION_API_TOKEN=<32-to-4096-visible-ASCII-byte-secret>
```

An empty value disables `/v1/integrations/*`; it is not an anonymous mode. Send the token only from a trusted backend over TLS or a private authenticated network. In particular, do not bundle it into browser JavaScript. The JavaScript package targets trusted Node.js/server runtimes for this reason.

The canonical wire contract is [`contract/openapi.yaml`](contract/openapi.yaml). SDK method and wire-field mappings are in [`contract/methods.md`](contract/methods.md).

## Direct and delegated data

Direct mode gives Omnidex a read-only PostgreSQL connection. Omnidex stores the supplied secret using its existing server-side data-source authority and performs bounded read-only queries itself.

Delegated mode is intended for applications that must apply their own tenant, user, and record authorization before any database evidence leaves the application boundary. Omnidex stores only:

- the delegated authority base URL;
- the name of a dedicated `OMNIDEX_DELEGATED_AUTHORITY_…_TOKEN` environment variable containing the outbound bearer secret;
- public data-source metadata.

It does not store PostgreSQL credentials for a delegated source.

The prefix and `_TOKEN` suffix are mandatory. Every token has a paired `_URL` variable, and Omnidex requires that URL to exactly match the registered authority before reading or sending the token:

```dotenv
OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_URL=https://application.internal
OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN=<outbound-authority-secret>
```

Registration rejects general process secrets such as `DATABASE_URL` or provider API-key variables. The exact URL binding prevents a caller from forwarding a configured authority token to a different registered host.

```text
trusted application backend
  ├─ integration bearer + prompt + opaque per-turn authority ──> Omnidex
  └─ policy/tenant mapping: opaque authority ──> authenticated user context

Omnidex
  └─ bounded schema/relational plan + opaque authority ──> application authority endpoints

application authority
  └─ permission filters + deterministic plan compilation ──> PostgreSQL
       └─ bounded evidence ──> Omnidex ──> local Ollama ──> typed transcript/job result
```

Generate a new `dba_…` authority ID for each delegated turn. Keep its mapping to the authenticated application user in server-owned state. The value is deliberately opaque: it must not contain a user ID, tenant ID, role, database key, or PHI.

The authority host returns only an allowlisted schema and permission-filtered, bounded evidence. Omnidex sends a typed relational plan, never model-authored SQL, to that host. Delegated evidence execution currently fails unless Omnidex uses its local Ollama inference path.

This boundary prevents an external model provider from receiving delegated evidence, but it is not by itself a HIPAA compliance claim. Prompts, evidence, transcripts, logs, backups, TLS termination, retention, access controls, and the Omnidex PostgreSQL deployment must all remain inside the organization’s governed environment.

## Packages

- [Go usage](go/README.md)
- [JavaScript usage](javascript/README.md)
- [PHP and Laravel usage](php/README.md)
- [Java usage](java/README.md)
- [Rust usage](rust/README.md)

These packages are source-ready in this repository. Their manifests are prepared for registries, but no claim is made that version `0.1.0` has already been published to an external package registry.
