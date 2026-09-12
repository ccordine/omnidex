# Codebase Map

The project-map API builds a fresh, bounded display of the current workspace.
It is not a durable repository model, an incremental cache, or proof that a
requested change has been implemented.

The [workspace index](WORKSPACE_INDEX.md) supplies current paths, file sizes,
recognized manifests, and package probes. The map groups files by language and
directory and supplies entrypoint, test, command, and symbol display data. Some
labels are path-based heuristics; they are not verified semantic understanding
or execution evidence.

## Current interface

The [project-map service](../internal/api/project_map_service.go) handles map reads
and explicit scans for a registered project. It scans locally when the project
is accessible there, or uses the configured host bridge for a host-only project.
Scan errors are reported, and the response identifies truncation.

The API limits scans to 1,200 files and returns bounded display previews. Its
workspace association is the actual root path and the enclosing project, not a
hashed workspace ID.

There is no saved `.omni/codebase-map.json`, per-file summary hash, or stale-hash
invalidation path. The old `omni map build`, `update`, `query`, and `route` commands
are not current CLI interfaces.

Implementation: [map construction](../internal/omni/codebase_map.go) and
[host-bridge projection](../internal/api/project_map_bridge.go).
