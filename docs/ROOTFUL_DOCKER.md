# Rootful Docker Is an Omnidex Invariant

Omnidex supports exactly one Docker authority: the system daemon exposed at
`/var/run/docker.sock` through Docker's built-in `default` context.

Rootless Docker must never be used to install, update, start, stop, verify, or
recover Omnidex. Rootless user-namespace translation changes bind-mount
ownership: a host workspace owned by the invoking user can appear owned by
container UID `0`, while the non-root core process cannot write it. That makes
authoritative host mutation fail even though the mount is read-write.

The project enforces this boundary as follows:

- `.env` must contain `DOCKER_CONTEXT=default`; every managed Compose and
  update entrypoint rejects any other value.
- Managed Docker commands clear ambient Docker, BuildKit, and Buildx routing
  variables and bind `DOCKER_CONTEXT=default` before invocation.
- `docker-compose.yml` mounts only `/var/run/docker.sock`, read-write.
- The core qualifies the daemon through `/info` and rejects a daemon that
  reports rootless security authority before executing Docker work.
- The core process uses the exact configured `HOST_UID:HOST_GID`, preserving
  ownership of files written through the rootful host bind.

Do not run ambient `docker compose` commands for Omnidex. Use `./up.sh`,
`./down.sh`, `./update.sh`, or `omni service`; these entrypoints own the exact
daemon and Compose project identity.

Before first use, verify the system socket and configure its group identity:

```bash
test -S /var/run/docker.sock
docker --context default info
stat -c '%g' /var/run/docker.sock
```

Set that final numeric value as `DOCKER_GID`. A missing system socket or a
daemon that reports rootless operation is an explicit deployment failure, not
a signal to select another Docker environment.
