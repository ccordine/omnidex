# Workspace Index

The workspace index is a deterministic snapshot of local project state.

Build an index:

```bash
omni index build
```

Update an existing index:

```bash
omni index update
```

Print JSON:

```bash
omni index build --json
```

The index records:

- workspace path
- file paths
- file sizes
- SHA-256 hashes
- recognized package/build manifests
- deterministic package manager and command probes

By default, the index is written to `.omni/index.json` in the workspace.

Local env/key material such as `.env`, `.env.*`, `*.pem`, and `*.key` is skipped.

This is the foundation for incremental workspace scans, command/result cache keys, and faster context assembly.

`omni index update` reuses prior hashes when file size and modification time are unchanged, and reports reused, rehashed, added, and removed file counts.
