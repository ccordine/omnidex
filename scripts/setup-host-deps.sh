#!/usr/bin/env bash
set -euo pipefail

PROFILE="all"
WITH_WHISPER=0
ASSUME_YES=0
DRY_RUN=0
NO_SUDO=0
PACKAGE_MANAGER=""

declare -a INSTALLED_PACKAGES=()
declare -a WARNINGS=()
declare -a DEPENDENCIES=()
declare -A DEP_SEEN=()

usage() {
  cat <<'EOF'
Usage:
  scripts/setup-host-deps.sh [options]

Options:
  --profile core|local|all  Dependency profile to install (default: all)
  --with-whisper            Install Python whisper CLI (`openai-whisper`) for audio transcription
  -y, --yes                 Pass non-interactive yes flags to package manager
  --dry-run                 Print commands without executing
  --no-sudo                 Do not use sudo when root is required
  -h, --help                Show this help

Profiles:
  core   Build/run essentials for core + CLI (go, docker, compose, npm, etc.)
  local  Local automation tools (media/OCR/screenshot + networking diagnostics)
  all    core + local

Notes:
  - The script detects your package manager automatically.
  - It installs packages one-by-one and keeps going if one package name is unavailable.
  - For Fedora, you may need RPM Fusion enabled to install `ffmpeg`/`vlc`.
EOF
}

log() {
  printf '[setup] %s\n' "$*"
}

warn() {
  printf '[setup][warn] %s\n' "$*" >&2
  WARNINGS+=("$*")
}

die() {
  printf '[setup][error] %s\n' "$*" >&2
  exit 1
}

run_cmd() {
  if ((DRY_RUN)); then
    printf '+'
    for token in "$@"; do
      printf ' %q' "$token"
    done
    printf '\n'
    return 0
  fi
  "$@"
}

run_pkg_cmd() {
  if [[ "$PACKAGE_MANAGER" == "brew" ]]; then
    run_cmd "$@"
    return
  fi
  if ((EUID == 0)); then
    run_cmd "$@"
    return
  fi
  if ((NO_SUDO)); then
    die "root privileges are required to install packages (rerun without --no-sudo or as root)"
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    die "sudo is required to install packages as non-root"
  fi
  run_cmd sudo "$@"
}

detect_package_manager() {
  if command -v dnf >/dev/null 2>&1; then
    PACKAGE_MANAGER="dnf"
    return
  fi
  if command -v apt-get >/dev/null 2>&1; then
    PACKAGE_MANAGER="apt-get"
    return
  fi
  if command -v pacman >/dev/null 2>&1; then
    PACKAGE_MANAGER="pacman"
    return
  fi
  if command -v apk >/dev/null 2>&1; then
    PACKAGE_MANAGER="apk"
    return
  fi
  if command -v brew >/dev/null 2>&1; then
    PACKAGE_MANAGER="brew"
    return
  fi
  die "unsupported host package manager (supported: dnf, apt-get, pacman, apk, brew)"
}

refresh_package_index() {
  case "$PACKAGE_MANAGER" in
    dnf)
      run_pkg_cmd dnf makecache
      ;;
    apt-get)
      run_pkg_cmd apt-get update
      ;;
    apk)
      run_pkg_cmd apk update
      ;;
    pacman | brew)
      ;;
    *)
      ;;
  esac
}

package_installed() {
  local pkg="$1"
  case "$PACKAGE_MANAGER" in
    dnf)
      rpm -q "$pkg" >/dev/null 2>&1
      ;;
    apt-get)
      dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "install ok installed"
      ;;
    pacman)
      pacman -Q "$pkg" >/dev/null 2>&1
      ;;
    apk)
      apk info -e "$pkg" >/dev/null 2>&1
      ;;
    brew)
      brew list --versions "$pkg" >/dev/null 2>&1
      ;;
    *)
      return 1
      ;;
  esac
}

install_package() {
  local pkg="$1"
  local -a cmd=()

  if package_installed "$pkg"; then
    log "package already installed: $pkg"
    return 0
  fi

  case "$PACKAGE_MANAGER" in
    dnf)
      cmd=(dnf install)
      if ((ASSUME_YES)); then
        cmd+=(-y)
      fi
      cmd+=("$pkg")
      ;;
    apt-get)
      cmd=(apt-get install)
      if ((ASSUME_YES)); then
        cmd+=(-y)
      fi
      cmd+=("$pkg")
      ;;
    pacman)
      cmd=(pacman -S --needed)
      if ((ASSUME_YES)); then
        cmd+=(--noconfirm)
      fi
      cmd+=("$pkg")
      ;;
    apk)
      cmd=(apk add)
      cmd+=("$pkg")
      ;;
    brew)
      cmd=(brew install "$pkg")
      ;;
    *)
      warn "unknown package manager: $PACKAGE_MANAGER"
      return 1
      ;;
  esac

  log "installing package: $pkg"
  if run_pkg_cmd "${cmd[@]}"; then
    INSTALLED_PACKAGES+=("$pkg")
    return 0
  fi
  warn "failed installing package: $pkg"
  return 1
}

dependency_candidates() {
  local key="$1"
  case "$PACKAGE_MANAGER" in
    dnf)
      case "$key" in
        git) echo "git" ;;
        go) echo "golang go" ;;
        make) echo "make" ;;
        curl) echo "curl" ;;
        jq) echo "jq" ;;
        rg) echo "ripgrep" ;;
        docker) echo "docker moby-engine docker-ce" ;;
        docker_compose) echo "docker-compose-plugin docker-compose" ;;
        npm) echo "npm nodejs" ;;
        ffmpeg) echo "ffmpeg" ;;
        pactl) echo "pulseaudio-utils pipewire-utils pipewire-pulseaudio pulseaudio" ;;
        vlc) echo "vlc" ;;
        playerctl) echo "playerctl" ;;
        iproute) echo "iproute" ;;
        dnsutils) echo "bind-utils" ;;
        netstat_tool) echo "iproute net-tools" ;;
        traceroute) echo "traceroute" ;;
        whois) echo "whois" ;;
        nmap) echo "nmap" ;;
        nmcli) echo "NetworkManager" ;;
        tesseract) echo "tesseract" ;;
        screenshot) echo "grim gnome-screenshot maim scrot ImageMagick" ;;
        python3) echo "python3" ;;
        pip3) echo "python3-pip" ;;
      esac
      ;;
    apt-get)
      case "$key" in
        git) echo "git" ;;
        go) echo "golang-go" ;;
        make) echo "make" ;;
        curl) echo "curl" ;;
        jq) echo "jq" ;;
        rg) echo "ripgrep" ;;
        docker) echo "docker.io docker-ce" ;;
        docker_compose) echo "docker-compose-plugin docker-compose" ;;
        npm) echo "npm nodejs" ;;
        ffmpeg) echo "ffmpeg" ;;
        pactl) echo "pulseaudio-utils pipewire-audio" ;;
        vlc) echo "vlc" ;;
        playerctl) echo "playerctl" ;;
        iproute) echo "iproute2" ;;
        dnsutils) echo "dnsutils bind9-dnsutils" ;;
        netstat_tool) echo "iproute2 net-tools" ;;
        traceroute) echo "traceroute" ;;
        whois) echo "whois" ;;
        nmap) echo "nmap" ;;
        nmcli) echo "network-manager" ;;
        tesseract) echo "tesseract-ocr" ;;
        screenshot) echo "grim gnome-screenshot maim scrot imagemagick" ;;
        python3) echo "python3" ;;
        pip3) echo "python3-pip" ;;
      esac
      ;;
    pacman)
      case "$key" in
        git) echo "git" ;;
        go) echo "go" ;;
        make) echo "make" ;;
        curl) echo "curl" ;;
        jq) echo "jq" ;;
        rg) echo "ripgrep" ;;
        docker) echo "docker" ;;
        docker_compose) echo "docker-compose" ;;
        npm) echo "npm nodejs" ;;
        ffmpeg) echo "ffmpeg" ;;
        pactl) echo "pulseaudio pipewire-pulse" ;;
        vlc) echo "vlc" ;;
        playerctl) echo "playerctl" ;;
        iproute) echo "iproute2" ;;
        dnsutils) echo "bind" ;;
        netstat_tool) echo "iproute2 net-tools" ;;
        traceroute) echo "traceroute" ;;
        whois) echo "whois" ;;
        nmap) echo "nmap" ;;
        nmcli) echo "networkmanager" ;;
        tesseract) echo "tesseract" ;;
        screenshot) echo "grim gnome-screenshot maim scrot imagemagick" ;;
        python3) echo "python" ;;
        pip3) echo "python-pip" ;;
      esac
      ;;
    apk)
      case "$key" in
        git) echo "git" ;;
        go) echo "go" ;;
        make) echo "make" ;;
        curl) echo "curl" ;;
        jq) echo "jq" ;;
        rg) echo "ripgrep" ;;
        docker) echo "docker docker-cli" ;;
        docker_compose) echo "docker-cli-compose docker-compose" ;;
        npm) echo "npm nodejs" ;;
        ffmpeg) echo "ffmpeg" ;;
        pactl) echo "pulseaudio-utils pipewire-pulse" ;;
        vlc) echo "vlc" ;;
        playerctl) echo "playerctl" ;;
        iproute) echo "iproute2" ;;
        dnsutils) echo "bind-tools" ;;
        netstat_tool) echo "iproute2 net-tools" ;;
        traceroute) echo "traceroute" ;;
        whois) echo "whois" ;;
        nmap) echo "nmap" ;;
        nmcli) echo "networkmanager" ;;
        tesseract) echo "tesseract-ocr tesseract" ;;
        screenshot) echo "grim maim scrot imagemagick" ;;
        python3) echo "python3" ;;
        pip3) echo "py3-pip" ;;
      esac
      ;;
    brew)
      case "$key" in
        git) echo "git" ;;
        go) echo "go" ;;
        make) echo "make" ;;
        curl) echo "curl" ;;
        jq) echo "jq" ;;
        rg) echo "ripgrep" ;;
        docker) echo "docker" ;;
        docker_compose) echo "docker-compose" ;;
        npm) echo "node" ;;
        ffmpeg) echo "ffmpeg" ;;
        pactl) echo "" ;;
        vlc) echo "vlc" ;;
        playerctl) echo "playerctl" ;;
        iproute) echo "" ;;
        dnsutils) echo "bind" ;;
        netstat_tool) echo "" ;;
        traceroute) echo "" ;;
        whois) echo "whois" ;;
        nmap) echo "nmap" ;;
        nmcli) echo "" ;;
        tesseract) echo "tesseract" ;;
        screenshot) echo "imagemagick" ;;
        python3) echo "python" ;;
        pip3) echo "python" ;;
      esac
      ;;
  esac
}

install_first_available() {
  local key="$1"
  local candidates
  local pkg
  candidates="$(dependency_candidates "$key")"
  if [[ -z "${candidates// }" ]]; then
    warn "no package mapping available for dependency: $key on $PACKAGE_MANAGER"
    return 1
  fi
  for pkg in $candidates; do
    if install_package "$pkg"; then
      return 0
    fi
  done
  return 1
}

have_docker_compose() {
  if ! command -v docker >/dev/null 2>&1; then
    return 1
  fi
  docker compose version >/dev/null 2>&1
}

have_screenshot_tool() {
  local cmd
  for cmd in grim gnome-screenshot maim scrot import; do
    if command -v "$cmd" >/dev/null 2>&1; then
      return 0
    fi
  done
  return 1
}

dependency_satisfied() {
  local key="$1"
  case "$key" in
    docker_compose)
      have_docker_compose
      ;;
    screenshot)
      have_screenshot_tool
      ;;
    iproute)
      command -v ip >/dev/null 2>&1 || command -v ifconfig >/dev/null 2>&1
      ;;
    dnsutils)
      command -v dig >/dev/null 2>&1 || command -v nslookup >/dev/null 2>&1 || command -v host >/dev/null 2>&1
      ;;
    netstat_tool)
      command -v ss >/dev/null 2>&1 || command -v netstat >/dev/null 2>&1 || command -v lsof >/dev/null 2>&1
      ;;
    traceroute)
      command -v traceroute >/dev/null 2>&1 || command -v mtr >/dev/null 2>&1
      ;;
    whois | nmap | nmcli)
      command -v "$key" >/dev/null 2>&1
      ;;
    git | go | make | curl | jq | rg | docker | npm | ffmpeg | pactl | vlc | playerctl | tesseract | python3 | pip3)
      command -v "$key" >/dev/null 2>&1
      ;;
    *)
      return 1
      ;;
  esac
}

ensure_dependency() {
  local key="$1"
  if dependency_satisfied "$key"; then
    log "dependency ready: $key"
    return 0
  fi

  log "dependency missing: $key (attempting install)"
  if ! install_first_available "$key"; then
    warn "unable to install dependency: $key"
    return 1
  fi

  if dependency_satisfied "$key"; then
    log "dependency installed: $key"
    return 0
  fi

  warn "dependency still missing after install attempt: $key"
  return 1
}

add_dependency() {
  local key="$1"
  if [[ -n "${DEP_SEEN[$key]:-}" ]]; then
    return
  fi
  DEP_SEEN["$key"]=1
  DEPENDENCIES+=("$key")
}

install_whisper_cli() {
  if command -v whisper >/dev/null 2>&1; then
    log "whisper CLI already available"
    return 0
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    warn "python3 is required for whisper CLI"
    return 1
  fi

  local -a pip_cmd=()
  if command -v pip3 >/dev/null 2>&1; then
    pip_cmd=(pip3)
  else
    pip_cmd=(python3 -m pip)
  fi

  log "installing whisper CLI via pip (openai-whisper)"
  if run_cmd "${pip_cmd[@]}" install --user --upgrade openai-whisper; then
    if ((DRY_RUN)); then
      log "dry-run: whisper CLI install command prepared"
      return 0
    fi
    if command -v whisper >/dev/null 2>&1; then
      log "whisper CLI installed successfully"
      return 0
    fi
    if [[ -x "$HOME/.local/bin/whisper" ]]; then
      warn "whisper installed at $HOME/.local/bin/whisper; add ~/.local/bin to PATH"
      return 0
    fi
    warn "pip install completed, but whisper command is not on PATH yet"
    return 1
  fi
  warn "failed to install whisper CLI"
  return 1
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --profile)
        if (($# < 2)); then
          die "--profile requires a value"
        fi
        PROFILE="$2"
        shift 2
        ;;
      --with-whisper)
        WITH_WHISPER=1
        shift
        ;;
      -y | --yes)
        ASSUME_YES=1
        shift
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      --no-sudo)
        NO_SUDO=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *)
        die "unknown option: $1 (use --help)"
        ;;
    esac
  done

  case "$PROFILE" in
    core | local | all)
      ;;
    *)
      die "invalid --profile value: $PROFILE (use core|local|all)"
      ;;
  esac
}

main() {
  parse_args "$@"
  detect_package_manager
  log "detected package manager: $PACKAGE_MANAGER"
  refresh_package_index

  if [[ "$PROFILE" == "core" || "$PROFILE" == "all" ]]; then
    add_dependency git
    add_dependency go
    add_dependency make
    add_dependency curl
    add_dependency jq
    add_dependency rg
    add_dependency docker
    add_dependency docker_compose
    add_dependency npm
  fi

  if [[ "$PROFILE" == "local" || "$PROFILE" == "all" ]]; then
    add_dependency ffmpeg
    add_dependency pactl
    add_dependency vlc
    add_dependency playerctl
    add_dependency iproute
    add_dependency dnsutils
    add_dependency netstat_tool
    add_dependency traceroute
    add_dependency whois
    add_dependency nmap
    add_dependency nmcli
    add_dependency tesseract
    add_dependency screenshot
  fi

  if ((WITH_WHISPER)); then
    add_dependency python3
    add_dependency pip3
  fi

  local dep
  for dep in "${DEPENDENCIES[@]}"; do
    ensure_dependency "$dep" || true
  done

  if ((WITH_WHISPER)); then
    install_whisper_cli || true
  fi

  if ((DRY_RUN)); then
    log "dry-run completed (no changes were made)"
    exit 0
  fi

  local missing=()
  for dep in "${DEPENDENCIES[@]}"; do
    if ! dependency_satisfied "$dep"; then
      missing+=("$dep")
    fi
  done
  if ((WITH_WHISPER)) && ! command -v whisper >/dev/null 2>&1 && [[ ! -x "$HOME/.local/bin/whisper" ]]; then
    missing+=("whisper")
  fi

  if ((${#INSTALLED_PACKAGES[@]} > 0)); then
    log "installed packages: ${INSTALLED_PACKAGES[*]}"
  else
    log "no new packages were installed"
  fi

  if ((${#missing[@]} > 0)); then
    warn "missing dependencies after setup: ${missing[*]}"
    exit 1
  fi

  if ((${#WARNINGS[@]} > 0)); then
    warn "setup completed with warnings (see messages above)"
  else
    log "setup completed successfully"
  fi
}

main "$@"
