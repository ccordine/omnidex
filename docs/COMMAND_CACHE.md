# Command execution evidence

Omnidex records commands that actually ran, including their output, exit status,
timing, and workspace association. See [EVIDENCE_LEDGER.md](EVIDENCE_LEDGER.md).

There is no hash-keyed command-result cache and no `.omni/command-cache`
directory maintained by the runtime. `OMNI_ENABLE_COMMAND_CACHE`,
`--enable-command-cache`, and `command_cache_hit` are retired controls/events,
not current interfaces.

A retained command observation describes its original execution. It must not be
presented as a newly executed verification because a file hash or command string
matches. Code may retain accepted job state and its existing evidence; changed
obligations are verified through the authoritative execution path.
