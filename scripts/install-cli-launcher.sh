#!/usr/bin/env bash
# Bind installed CLI names to one authoritative native binary and configuration.
set -euo pipefail
umask 077
[[ $# -ge 3 ]] || { echo 'Usage: install-cli-launcher.sh <repo> <config-root> <launcher> [cli-alias ...]' >&2; exit 2; }
repo=$1
config_root=$2
launcher=$3
shift 3
[[ -x "$repo/bin/omni" && -f "$config_root/runtime.env" && ! -L "$config_root/runtime.env" ]] || { echo 'Built CLI and retained regular native configuration required' >&2; exit 1; }
for target in "$launcher" "$@"; do
  [[ "$target" == /* && "$target" != *[[:space:]%\"\\]* ]] || { echo 'CLI installation requires absolute plain paths' >&2; exit 1; }
  [[ ! -d "$target" ]] || { echo "Preserving directory: $target" >&2; exit 1; }
  if [[ -e "$target" || -L "$target" ]]; then
    if [[ -L "$target" && $(readlink "$target") == "$launcher" ]]; then continue; fi
    if rg -q '^# Managed by Omnidex native installer$' "$target"; then continue; fi
    if [[ "$target" != "$launcher" && ! -L "$target" ]] && go version -m "$target" 2>/dev/null | rg -q '^[[:space:]]+path[[:space:]]+github.com/gryph/omnidex/cmd/omni$'; then continue; fi
    echo "Preserving unrecognized CLI: $target" >&2
    exit 1
  fi
done
mkdir -p "$(dirname "$launcher")"
candidate=$(mktemp "$(dirname "$launcher")/.omnidex-launcher.XXXXXX")
trap 'rm -f -- "$candidate"' EXIT
cat > "$candidate" <<EOF
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
chmod 700 "$candidate"
mv -- "$candidate" "$launcher"
for target in "$@"; do
  [[ "$target" != "$launcher" ]] || continue
  mkdir -p "$(dirname "$target")"
  ln -sfn -- "$launcher" "$target"
done
