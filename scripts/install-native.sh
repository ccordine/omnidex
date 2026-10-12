#!/usr/bin/env bash
# Install built binaries from the authoritative checkout. Never copy source.
set -euo pipefail
umask 077
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
for command in psql pg_isready createdb; do
  command -v "$command" >/dev/null || { echo "PostgreSQL client $command is required" >&2; exit 1; }
done
[[ $# == 4 ]] || { echo 'Usage: install-native.sh <codex> <model> <postgres-bin> <valkey-server>' >&2; exit 2; }
codex_command=$1
model=$2
postgres_bin=$3
valkey_command=$4
config_root=${XDG_CONFIG_HOME:-$HOME/.config}/omnidex
state_root=${XDG_STATE_HOME:-$HOME/.local/state}/omnidex
unit_root=${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user
launcher=$HOME/.local/bin/omnidex
for path in "$repo" "$codex_command" "$postgres_bin" "$valkey_command" "$config_root" "$state_root" "$unit_root"; do
  [[ "$path" == /* && "$path" != *[[:space:]%\"\\]* ]] || { echo 'Installation paths must be absolute plain paths' >&2; exit 2; }
done
[[ ! -e "$launcher" ]] || rg -q '^# Managed by Omnidex native installer$' "$launcher" || { echo "Preserving unmanaged launcher: $launcher" >&2; exit 1; }
for command in "$repo/bin/omnidex" "$repo/bin/omni" "$codex_command" "$postgres_bin/postgres" "$postgres_bin/initdb" "$valkey_command"; do
  [[ -x "$command" ]] || { echo "Missing executable: $command" >&2; exit 1; }
done
for name in omnidex omnidex-postgres omnidex-valkey; do
  unit="$unit_root/$name.service"
  [[ ! -e "$unit" ]] || rg -q '^# Managed by Omnidex native installer$' "$unit" || { echo "Preserving unmanaged unit: $unit" >&2; exit 1; }
done
if [[ $(loginctl show-user "$USER" -p Linger --value) != yes ]]; then
  loginctl enable-linger "$USER"
fi
mkdir -p "$config_root" "$state_root/postgres" "$state_root/socket" "$state_root/valkey" "$unit_root"
chmod 700 "$state_root" "$state_root/postgres" "$state_root/socket" "$state_root/valkey" "$config_root"
if [[ ! -e "$state_root/postgres/PG_VERSION" ]]; then
  [[ -z $(ls -A "$state_root/postgres") ]] || { echo 'Existing nonempty database directory preserved' >&2; exit 1; }
  "$postgres_bin/initdb" -D "$state_root/postgres" --auth-local=peer --auth-host=reject --encoding=UTF8 --no-locale
fi
if [[ ! -e "$config_root/runtime.env" ]]; then
  secret=$(openssl rand -hex 32)
  {
    printf 'LISTEN_ADDR=127.0.0.1:18090\nCORE_URL=http://127.0.0.1:18090\n'
    printf 'DATABASE_URL=postgresql:///omnidex?host=%s/socket&port=15432\nDATABASE_SCHEMA=omnidex_runtime\n' "$state_root"
    printf 'LLM_PROVIDER=codex\nCODEX_COMMAND=%s\nCODEX_DIRECTORY=%s\n' "$codex_command" "$repo"
    printf 'HOST_DIRECTORY_ACCESS_ROOT=%s\n' "$repo"
    printf 'INFERENCE_CONTEXT_TOKENS=131072\nREQUEST_TIMEOUT=30m\n'
    printf 'REDIS_URL=redis://:%s@127.0.0.1:16379/0\n' "$secret"
    printf 'OMNIDEX_INTEGRATION_API_TOKEN=%s\n' "$(openssl rand -hex 32)"
    "$repo/bin/omni" model-env "$model"
  } > "$config_root/runtime.env"
  {
    printf 'bind 127.0.0.1\nport 16379\nprotected-mode yes\nrequirepass %s\n' "$secret"
    printf 'dir %s/valkey\nappendonly yes\nappendfsync everysec\nmaxmemory 256mb\nmaxmemory-policy noeviction\n' "$state_root"
  } > "$config_root/valkey.conf"
fi
[[ -f "$config_root/valkey.conf" && ! -L "$config_root/runtime.env" && ! -L "$config_root/valkey.conf" ]] || { echo 'Regular native configuration required; existing state preserved' >&2; exit 1; }
chmod 600 "$config_root/runtime.env" "$config_root/valkey.conf"
cat > "$unit_root/omnidex-postgres.service" <<EOF
# Managed by Omnidex native installer
[Unit]
Description=Omnidex durable task database
[Service]
Type=notify
ExecStart=$postgres_bin/postgres -D $state_root/postgres -k $state_root/socket -h '' -p 15432
ExecStartPost=$(command -v pg_isready) -h $state_root/socket -p 15432 -t 30
Restart=on-failure
RestartSec=3
TimeoutStopSec=60
KillSignal=SIGINT
MemoryHigh=1G
[Install]
WantedBy=default.target
EOF
cat > "$unit_root/omnidex-valkey.service" <<EOF
# Managed by Omnidex native installer
[Unit]
Description=Omnidex session state
[Service]
ExecStart=$valkey_command $config_root/valkey.conf
Restart=on-failure
RestartSec=3
MemoryHigh=384M
[Install]
WantedBy=default.target
EOF
cat > "$unit_root/omnidex.service" <<EOF
# Managed by Omnidex native installer
[Unit]
Description=Omnidex persistent task execution
Requires=omnidex-postgres.service omnidex-valkey.service
After=network-online.target omnidex-postgres.service omnidex-valkey.service
[Service]
WorkingDirectory=$repo
EnvironmentFile=$config_root/runtime.env
ExecStart=$repo/bin/omnidex
Restart=on-failure
RestartSec=3
TimeoutStopSec=60
KillMode=control-group
Environment=GOMAXPROCS=2
MemoryHigh=2G
[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now omnidex-postgres.service omnidex-valkey.service
if ! psql -h "$state_root/socket" -p 15432 -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='omnidex'" | rg -q '^1$'; then
  createdb -h "$state_root/socket" -p 15432 omnidex
fi
systemctl --user enable --now omnidex.service
core_url=$(sed -n 's/^CORE_URL=//p' "$config_root/runtime.env")
ready=0
for ((attempt=0; attempt<30; attempt++)); do
  if curl --fail --silent --max-time 2 "$core_url/readyz" >/dev/null; then ready=1; break; fi
  sleep 1
done
((ready)) || { echo 'Omnidex did not become ready; inspect its user service journal' >&2; exit 1; }
mkdir -p "$(dirname "$launcher")"
cat > "$launcher" <<EOF
#!/usr/bin/env bash
# Managed by Omnidex native installer
set -euo pipefail
if [[ -z \${CORE_URL:-} ]]; then
  CORE_URL=\$(sed -n 's/^CORE_URL=//p' "$config_root/runtime.env")
  export CORE_URL
fi
if ((\$# == 0)); then set -- chat; fi
exec "$repo/bin/omni" "\$@"
EOF
chmod 700 "$launcher"
systemctl --user is-active omnidex.service omnidex-postgres.service omnidex-valkey.service
[[ $(loginctl show-user "$USER" -p Linger --value) == yes ]] || { echo 'User lingering is required for startup before login' >&2; exit 1; }
printf 'Omnidex installed from %s; durable state: %s\n' "$repo" "$state_root"
