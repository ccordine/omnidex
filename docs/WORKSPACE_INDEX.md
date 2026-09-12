# Workspace Index

The workspace index is an ephemeral, bounded view of the current filesystem.
Code builds it from the actual workspace when a consumer needs a scan. It does
not load an old snapshot or maintain cross-run history.

The index records:

- workspace path
- file paths
- file sizes
- recognized package/build manifests
- deterministic package manager and command probes

Local env/key material such as `.env`, `.env.*`, `*.pem`, and `*.key` is skipped.
The scan also skips generated/cache directories and `.omni`. Reaching the file
limit marks the result truncated; filesystem inspection failures return errors.

No `.omni/index.json`, file digest, incremental rehash count, or command-result
cache key is produced. The retired `omni index build` and `omni index update`
commands are not supported interfaces.

Implementation: [workspace scan](../internal/omni/workspace_index.go). The
[project map](CODEBASE_MAP.md) is one current consumer. This display index does
not replace the parser/compiler facts used by the coding assembly line.
