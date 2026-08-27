#!/bin/sh
set -eu

fail() {
  printf '[volume-init][error] %s\n' "$*" >&2
  exit 1
}

uid="${1:-}"
gid="${2:-}"
if [ "$#" -ge 2 ]; then
  shift 2
else
  shift "$#"
fi

case "${uid}" in
  ''|0|0*|*[!0-9]*) fail "target UID must be one exact positive numeric host identity" ;;
esac
case "${gid}" in
  ''|0|0*|*[!0-9]*) fail "target GID must be one exact positive numeric host identity" ;;
esac
[ "${#uid}" -le 10 ] && [ "${uid}" -le 4294967294 ] ||
  fail "target UID must be one exact positive numeric host identity"
[ "${#gid}" -le 10 ] && [ "${gid}" -le 4294967294 ] ||
  fail "target GID must be one exact positive numeric host identity"
[ "$#" -gt 0 ] || fail "at least one runtime volume directory is required"

expected="${uid}:${gid}"
for directory in "$@"; do
  [ -d "${directory}" ] && [ ! -L "${directory}" ] ||
    fail "runtime volume must be a real directory: ${directory}"
  marker="${directory}/.omnidex-owner"
  if [ -e "${marker}" ] || [ -L "${marker}" ]; then
    [ -f "${marker}" ] && [ ! -L "${marker}" ] ||
      fail "runtime volume owner marker must be a regular non-symlink file: ${marker}"
  fi
  chmod u+rwx "${directory}" ||
    fail "cannot make runtime volume owner-writable: ${directory}"
  if [ -f "${marker}" ] &&
    [ "$(cat "${marker}")" = "${expected}" ] &&
    [ "$(stat -c '%u:%g' "${directory}")" = "${expected}" ]; then
    continue
  fi
  [ -z "$(find "${directory}" -type l -print -quit)" ] ||
    fail "runtime volume contains a symbolic link: ${directory}"
  chown -R "${expected}" "${directory}" ||
    fail "cannot assign runtime volume ownership ${expected}: ${directory}"
  printf '%s\n' "${expected}" > "${marker}"
  chown "${expected}" "${marker}"
  chmod 0600 "${marker}"
  [ "$(stat -c '%u:%g' "${directory}")" = "${expected}" ] ||
    fail "runtime volume ownership verification failed: ${directory}"
done
