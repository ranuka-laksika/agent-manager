#!/usr/bin/env bash
# install-advanced.sh — config-driven Agent Manager install on a VM with Docker.
# Run ON the target VM with sudo. Supports custom domains and three TLS modes
# (letsencrypt | byoc | upstream). See --init for the annotated config template.
#
# Usage:
#   sudo ./install-advanced.sh --config amp-config.env
#   ./install-advanced.sh --init > amp-config.env      # emit annotated template
#   sudo ./install-advanced.sh --config amp-config.env --dry-run   # validate + render only
set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# This installer wraps the quick-start installer (install.sh + k3d-config.yaml).
QS_DIR="$(cd "${VM_DIR}/../quick-start" && pwd)"

log() { printf '\033[0;34m[install-advanced]\033[0m %s\n' "$*"; }
die() { printf '\033[0;31m[install-advanced] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# shellcheck source=lib-vm.sh
source "${VM_DIR}/lib-vm.sh"
# shellcheck source=lib-advanced.sh
source "${VM_DIR}/lib-advanced.sh"
# shellcheck source=lib-bootstrap.sh
source "${VM_DIR}/lib-bootstrap.sh"

print_template() {
  cat <<'TEMPLATE'
# amp-config.env — Agent Manager advanced VM install configuration.
# Sourced by install-advanced.sh. Lines are shell assignments.

# --- Required ---
AMP_VERSION=0.15.0                 # amp/v* release tag (see github.com/wso2/agent-manager/tags)
DOMAIN_BASE=amp.mycompany.com      # service hosts derived as <svc>.<DOMAIN_BASE>
TLS_MODE=letsencrypt               # letsencrypt | byoc | upstream

# --- letsencrypt mode ---
ACME_EMAIL=ops@mycompany.com       # ACME contact (recommended)

# --- byoc mode (operator-supplied cert/key) ---
# The cert MUST carry SANs covering *.<DOMAIN_BASE> AND *.<AGENTS_BASE>.
# TLS_CERT_FILE=/opt/amp/certs/fullchain.pem
# TLS_KEY_FILE=/opt/amp/certs/privkey.pem

# --- upstream mode (TLS terminated by a cloud LB / proxy in front of the VM) ---
# The LB must forward each hostname to the VM and set X-Forwarded-Proto: https.
# UPSTREAM_LISTEN_PORT=80          # plain-HTTP port the LB forwards to (default 80).
#   Must not be a loopback-bound cluster port (3000/8080/9000/9098/9243/19080/22893);
#   80 is safe.
# Restrict the listen port to the LB at the firewall, and set the LB's source CIDRs
# below so only it can set X-Forwarded-* (default 0.0.0.0/0 trusts any source). Use a
# space-separated list; the example is the GCP Application Load Balancer's ranges.
# UPSTREAM_TRUSTED_PROXIES="130.211.0.0/22 35.191.0.0/16"

# --- Optional ---
EXTERNAL_GATEWAYS=true             # expose the cp endpoint for external data-plane gateways

# --- Optional per-service host overrides (default: <svc>.<DOMAIN_BASE>) ---
# HOST_CONSOLE=console.amp.mycompany.com
# HOST_API=api.amp.mycompany.com
# HOST_THUNDER=thunder.amp.mycompany.com
# HOST_OBSERVER=observer.amp.mycompany.com
# HOST_GATEWAY=gateway.amp.mycompany.com
# HOST_CP=cp.amp.mycompany.com
# AGENTS_BASE=agents.amp.mycompany.com   # deployed-agent wildcard base
TEMPLATE
}

CONFIG_FILE="" DRY_RUN="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --init) print_template; exit 0 ;;
    --config) CONFIG_FILE="${2:?--config requires a path}"; shift 2 ;;
    --dry-run) DRY_RUN="true"; shift ;;
    -h|--help) grep '^#' "$0" | grep -v '^#!' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

[[ -n "$CONFIG_FILE" ]] || die "--config <file> is required (or --init to emit a template)"

# Load + validate config, derive hostnames.
load_config "$CONFIG_FILE" || die "could not load config: $CONFIG_FILE"
if ! validate_config; then
  printf '%s\n' "${CONFIG_ERRORS[@]}" >&2
  die "config validation failed (${#CONFIG_ERRORS[@]} error(s)) — fix amp-config.env and re-run"
fi
# Declare the host vars in this scope so the lib-vm.sh cores (dynamic scope) see them.
AMP_HOST_CONSOLE="" AMP_HOST_API="" AMP_HOST_THUNDER="" AMP_HOST_OBSERVER=""
AMP_HOST_GATEWAY="" AMP_HOST_CP="" AMP_AGENTS_BASE=""
derive_hosts

# BYOC cert validation happens before any cluster work.
if [[ "$TLS_MODE" == byoc ]]; then
  if ! validate_cert "$TLS_CERT_FILE" "$TLS_KEY_FILE"; then
    printf '%s\n' "${CERT_ERRORS[@]}" >&2
    die "certificate validation failed — see errors above"
  fi
fi

render_active_caddyfile() {   # echoes the Caddyfile for the active TLS_MODE
  case "$TLS_MODE" in
    letsencrypt) caddyfile letsencrypt "${ACME_EMAIL:-}" "" "" "" ;;
    byoc)        caddyfile byoc "" "$TLS_CERT_FILE" "$TLS_KEY_FILE" "" ;;
    upstream)    caddyfile upstream "" "" "" "${UPSTREAM_LISTEN_PORT:-80}" "${UPSTREAM_TRUSTED_PROXIES:-0.0.0.0/0}" ;;
  esac
}

# start_caddy_advanced — render the active Caddyfile and (re)start the Caddy container.
# byoc mode bind-mounts the operator cert/key read-only into the container.
start_caddy_advanced() {
  mkdir -p /opt/amp
  render_active_caddyfile >/opt/amp/Caddyfile
  log "Wrote /opt/amp/Caddyfile (${TLS_MODE})"

  local mounts=(-v amp-caddy-data:/data -v amp-caddy-config:/config
                -v /opt/amp/Caddyfile:/etc/caddy/Caddyfile:ro)
  if [[ "$TLS_MODE" == byoc ]]; then
    mounts+=(-v "${TLS_CERT_FILE}:${TLS_CERT_FILE}:ro" -v "${TLS_KEY_FILE}:${TLS_KEY_FILE}:ro")
  fi

  docker rm -f amp-caddy >/dev/null 2>&1 || true
  docker run -d --name amp-caddy --restart unless-stopped --network host \
    "${mounts[@]}" caddy:2
  verify_caddy_up
}

# preflight_dns <strict|advisory> — confirm the derived hostnames point at THIS VM.
# On a NAT'd cloud VM the host's own interfaces show only the private IP while DNS
# points at the public one, so candidates = local IPs + the public egress IP. In
# strict mode a letsencrypt mismatch aborts (only when the public IP is known, so a
# detection failure doesn't block a correct install); in advisory mode (dry-run) it
# reports without aborting.
preflight_dns() {
  local mode="$1"
  local -a cand=(); local ip pub
  while IFS= read -r ip; do [[ -n "$ip" ]] && cand+=("$ip"); done < <(_local_ips)
  pub="$(_public_ip)"; [[ -n "$pub" ]] && cand+=("$pub")
  if (( ${#cand[@]} == 0 )); then
    log "Could not determine the VM's IP for the DNS check; skipping (verify DNS manually)."
    return 0
  fi
  if validate_dns "${cand[@]}"; then
    [[ "$mode" == advisory ]] && log "DNS check: all hostnames resolve to this VM."
    return 0
  fi
  # validate_dns already printed the per-host details + (letsencrypt) remediation.
  if [[ "$mode" == strict && "${TLS_MODE:-}" == letsencrypt ]]; then
    [[ -n "$pub" ]] && die "DNS pre-flight failed (letsencrypt) — see remediation above"
    log "WARNING: DNS check inconclusive (could not determine the VM's public IP); proceeding — Caddy will fail loudly if the hostnames don't point at this VM."
  fi
  return 0
}

run_advanced_install() {
  [[ "$(id -u)" -eq 0 ]] || die "run with sudo — this installs Docker, opens the firewall and creates the cluster"

  preflight_dns strict

  log "Phase 1/2: bootstrap (Docker + tools + firewall)"
  ensure_prerequisites
  if [[ "$TLS_MODE" == upstream ]]; then ensure_firewall "${UPSTREAM_LISTEN_PORT:-80}"; else ensure_firewall 443; fi
  ensure_disk

  log "Phase 2/2: install Agent Manager + start Caddy (this takes 8-15 min)"
  # Build the override arrays install.sh honors, from the hostname-driven cores.
  # shellcheck disable=SC2034  # arrays are inherited by the subshell that sources install.sh
  mapfile -t AMP_HELM_ARGS < <(amp_helm_args)
  # shellcheck disable=SC2034
  mapfile -t THUNDER_HELM_ARGS < <(thunder_helm_args)
  # shellcheck disable=SC2034
  mapfile -t GATEWAY_HELM_ARGS < <(gateway_helm_args)
  # shellcheck disable=SC2034
  mapfile -t CP_HELM_ARGS < <(cp_helm_args)
  # shellcheck disable=SC2034
  mapfile -t PLATFORM_RESOURCES_HELM_ARGS < <(build_platform_resources_helm_args)
  # shellcheck disable=SC2034
  mapfile -t OBSERVABILITY_HELM_ARGS < <(observability_helm_args)

  DP_EXTERNAL_INGRESS="$(dataplane_external_ingress)"; export DP_EXTERNAL_INGRESS
  export VERSION="$AMP_VERSION"
  export SHOW_LOCALHOST_URLS=false

  render_k3d_vm_config <"${QS_DIR}/k3d-config.yaml" >/tmp/k3d-config-vm.yaml
  export K3D_CONFIG=/tmp/k3d-config-vm.yaml
  render_coredns_vm_config "k3d-amp-local-server-0" >/tmp/coredns-amp-vm.yaml
  export COREDNS_FILE=/tmp/coredns-amp-vm.yaml

  log "Running base installer with custom-domain overrides (${TLS_MODE})"
  local rc=0
  ( set +e; source "${QS_DIR}/install.sh" ) || rc=$?
  [[ "$rc" -eq 0 ]] || die "Base installer exited $rc"

  start_caddy_advanced

  log "Done. Access URLs:"
  cat <<EOF

  Console:   https://${AMP_HOST_CONSOLE}
  API:       https://${AMP_HOST_API}
  Thunder:   https://${AMP_HOST_THUNDER}
  Observer:  https://${AMP_HOST_OBSERVER}
  OTel ingest: https://${AMP_HOST_GATEWAY}/otel
  Deployed agents: https://<org>-<project>.${AMP_AGENTS_BASE}/...
EOF
  [[ -n "$AMP_HOST_CP" ]] && echo "  Gateway control plane: https://${AMP_HOST_CP}  (connect external gateways here; registration token is secret-bearing)"
}

if [[ "$DRY_RUN" == "true" ]]; then
  log "DRY RUN — derived hosts:"
  printf '  console=%s api=%s thunder=%s observer=%s gateway=%s cp=%s agents=%s\n' \
    "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" "$AMP_HOST_OBSERVER" \
    "$AMP_HOST_GATEWAY" "${AMP_HOST_CP:-<none>}" "$AMP_AGENTS_BASE"
  log "DRY RUN — amp helm args:"; amp_helm_args
  log "DRY RUN — Caddyfile (${TLS_MODE}):"; render_active_caddyfile
  log "DRY RUN — DNS pre-flight (advisory):"; preflight_dns advisory
  exit 0
fi

run_advanced_install
