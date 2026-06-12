#!/usr/bin/env bash
# lib-bootstrap.sh — host preparation shared by install-vm.sh and install-advanced.sh.
# Sourcing only defines functions (no side effects). The caller is expected to define
# log()/die(); fallbacks are provided so this file is usable standalone.

command -v log >/dev/null 2>&1 || log() { printf '\033[0;34m[bootstrap]\033[0m %s\n' "$*"; }
command -v die >/dev/null 2>&1 || die() { printf '\033[0;31m[bootstrap] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

ensure_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    log "Installing Docker via get.docker.com"
    curl -fsSL https://get.docker.com | sh
  else
    log "Docker CLI present"
  fi
  systemctl enable --now docker
  local _
  for _ in $(seq 1 15); do docker info >/dev/null 2>&1 && return; sleep 2; done
  die "Docker daemon did not become ready"
}

ensure_prerequisites() {
  local arch; arch="$(dpkg --print-architecture)"   # amd64 | arm64
  local pkgs=()
  command -v curl >/dev/null 2>&1 || pkgs+=(curl)
  command -v lsof >/dev/null 2>&1 || pkgs+=(lsof)
  command -v openssl >/dev/null 2>&1 || pkgs+=(openssl)
  if (( ${#pkgs[@]} )); then
    log "Installing base packages: ${pkgs[*]}"
    apt-get update -qq && apt-get install -y -qq "${pkgs[@]}"
  fi

  ensure_docker

  if ! command -v k3d >/dev/null 2>&1; then
    log "Installing k3d"
    curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
  fi
  if ! command -v kubectl >/dev/null 2>&1; then
    log "Installing kubectl (${arch})"
    local kver; kver="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"
    curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${kver}/bin/linux/${arch}/kubectl"
    chmod +x /usr/local/bin/kubectl
  fi
  if ! command -v helm >/dev/null 2>&1; then
    log "Installing helm"
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  fi
}

# ensure_firewall <port> — open the given inbound TCP port on the OS firewall.
ensure_firewall() {
  local port="${1:?ensure_firewall requires a port}"
  if command -v ufw >/dev/null 2>&1; then
    ufw allow "${port}/tcp" || true
    log "ufw: opened ${port}"
  elif command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${port}/tcp" || true
    firewall-cmd --reload || true
    log "firewalld: opened ${port}"
  else
    log "No ufw/firewalld found; assuming the host firewall is open for ${port}"
  fi
  log "Ensure inbound ${port}/tcp is open in your cloud security group (raw TCP, no TLS-terminating proxy in front) — Caddy needs it."
}

ensure_disk() {
  local avail_kb min_kb=$((40 * 1024 * 1024))
  avail_kb="$(df -Pk / | awk 'NR==2 {print $4}')"
  if [[ -n "$avail_kb" && "$avail_kb" -lt "$min_kb" ]]; then
    log "WARNING: only $((avail_kb / 1024 / 1024)) GB free on / — agent builds may"
    log "         hit DiskPressure. A 50 GB+ disk is recommended (see the VM docs)."
  fi
}

# verify_caddy_up — fail loudly if the amp-caddy container isn't healthy shortly after
# start. Caddy runs on the host network, so a port collision (e.g. an UPSTREAM_LISTEN_PORT
# already bound by k3d) makes it crash-loop; without this check the installer would
# report success while every request 404s through a dead proxy.
verify_caddy_up() {
  sleep 5
  local status restarts
  status="$(docker inspect -f '{{.State.Status}}' amp-caddy 2>/dev/null || true)"
  restarts="$(docker inspect -f '{{.RestartCount}}' amp-caddy 2>/dev/null || echo 0)"
  if [[ "$status" != "running" || "${restarts:-0}" -gt 0 ]]; then
    log "amp-caddy is not healthy (status=${status:-missing}, restarts=${restarts:-?}). Recent logs:"
    docker logs --tail 25 amp-caddy 2>&1 | sed 's/^/    /' >&2 || true
    die "Caddy failed to start — see logs above (in upstream mode, a common cause is UPSTREAM_LISTEN_PORT colliding with a loopback-bound cluster port)"
  fi
  log "Caddy healthy (status=${status})"
}
