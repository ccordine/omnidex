# Command Cache

The command cache stores command observations by:

- workspace path
- command text
- indexed file hashes

This lets Omnidex reuse evidence when the command and relevant workspace state have not changed.

Runtime reuse is opt-in:

```bash
OMNI_ENABLE_COMMAND_CACHE=1 omni
omni --enable-command-cache
```

Eligible verification/read commands are cached only when the command succeeds and the workspace index proves the inputs are unchanged. The first successful run stores the observation under `.omni/command-cache`; later matching runs emit `command_cache_hit` and reuse the prior exit code/stdout/stderr.

Failed commands are not cached. This prevents a transient missing file, missing dependency, or broken environment from poisoning later runs after the workspace changes.

Initial eligible command families:

- `go test ...`
- `npm test`
- `npm run test`
- `npm run build`
- `git status ...`
- `git diff ...`
- `git branch ...`
- `test -f PATH`
