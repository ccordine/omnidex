# Failure Fingerprints

Failure fingerprints classify command output before asking a model to guess.

Classify stdin:

```bash
some-command 2>&1 | omni fingerprint
```

Classify explicit text:

```bash
omni fingerprint --text "webpack: command not found"
```

Current fingerprint kinds:

- `missing_command`
- `permission_denied`
- `port_in_use`
- `network_failure`
- `missing_file`
- `syntax_error`
- `test_failure`
- `dependency_unavailable`
- `unknown`

Fingerprints include a concise summary and, where possible, a deterministic remediation hint.
