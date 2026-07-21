#!/usr/bin/env bash
# lib-advanced.sh — config loading, host derivation, and pre-flight validation for
# install-advanced.sh. Sourcing only defines functions (no side effects).

# derive_hosts — from DOMAIN_BASE (+ optional HOST_* overrides, AGENTS_BASE,
# EXTERNAL_GATEWAYS), set the AMP_HOST_*/AMP_AGENTS_BASE variables the lib-vm.sh
# cores read. Caller should declare these in its scope (or accept globals).
# shellcheck disable=SC2034  # AMP_HOST_*/AMP_AGENTS_BASE are consumed by the lib-vm.sh cores.
derive_hosts() {
  : "${DOMAIN_BASE:?derive_hosts requires DOMAIN_BASE}"
  AMP_HOST_CONSOLE="${HOST_CONSOLE:-console.${DOMAIN_BASE}}"
  AMP_HOST_API="${HOST_API:-api.${DOMAIN_BASE}}"
  AMP_HOST_THUNDER="${HOST_THUNDER:-thunder.${DOMAIN_BASE}}"
  AMP_HOST_OBSERVER="${HOST_OBSERVER:-observer.${DOMAIN_BASE}}"
  AMP_HOST_GATEWAY="${HOST_GATEWAY:-gateway.${DOMAIN_BASE}}"
  AMP_AGENTS_BASE="${AGENTS_BASE:-agents.${DOMAIN_BASE}}"
  if [[ "${EXTERNAL_GATEWAYS:-true}" == "true" ]]; then
    AMP_HOST_CP="${HOST_CP:-cp.${DOMAIN_BASE}}"
  else
    AMP_HOST_CP=""
  fi
}

# load_config <file> — source an env-style config file in the caller's scope.
load_config() {
  local file="${1:?load_config requires a config file path}"
  [[ -f "$file" ]] || { printf 'config file not found: %s\n' "$file" >&2; return 1; }
  # Export every assignment so DNS-provider credentials (CF_*, AWS_*, GCE_*, ...) reach
  # the dockerized lego: _lego_cred_env_args lists them via `compgen -e` (exported only)
  # and forwards them with `docker run -e`. A plain `source` leaves them unexported, so
  # token-based providers (Cloudflare/route53/azuredns) would silently get no credentials.
  # shellcheck disable=SC1090
  set -a; source "$file"; set +a
}

# validate_config — check required keys and the DNS-01 provider/credentials. Populates
# the CONFIG_ERRORS array (reset on each call) and returns 1 if any error was recorded.
# TLS is always cert-manager DNS-01 (there is a single mode), so this validates the
# common keys plus the provider block (validate_dns01_config, from lib-certmanager.sh).
validate_config() {
  CONFIG_ERRORS=()
  [[ -n "${AMP_VERSION:-}" ]] || CONFIG_ERRORS+=("AMP_VERSION is required (an amp/v* release tag, e.g. 0.15.0)")
  [[ -n "${DOMAIN_BASE:-}" ]] || CONFIG_ERRORS+=("DOMAIN_BASE is required (e.g. amp.mycompany.com)")
  [[ -n "${ACME_EMAIL:-}" ]]  || CONFIG_ERRORS+=("ACME_EMAIL is required (cert-manager registers an ACME account with it)")
  validate_dns01_config   # DNS_PROVIDER + provider-specific credentials (lib-certmanager.sh)
  (( ${#CONFIG_ERRORS[@]} == 0 ))
}

# _resolve_host <hostname> — print ALL A records (one per line). Overridable in tests.
# Uses dig if present, else getent. Prints nothing if unresolved.
_resolve_host() {
  if command -v dig >/dev/null 2>&1; then
    dig +short A "$1" | grep -E '^[0-9.]+$'
  else
    getent ahostsv4 "$1" 2>/dev/null | awk '{print $1}' | sort -u
  fi
}

# _local_ips — print this host's IPv4 addresses (one per line), excluding loopback.
_local_ips() {
  if command -v hostname >/dev/null 2>&1 && hostname -I >/dev/null 2>&1; then
    hostname -I | tr ' ' '\n' | grep -E '^[0-9.]+$' | grep -vE '^127\.'
  else
    ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1
  fi
}

# _public_ip — best-effort public egress IP (cloud VMs are NAT'd, so a host's own
# interfaces show only the private IP while DNS must point at the public one). Empty
# if it can't be determined. Overridable in tests.
_public_ip() {
  local ip
  for url in https://api.ipify.org https://ifconfig.me https://icanhazip.com; do
    if command -v curl >/dev/null 2>&1; then
      ip="$(curl -fsS --max-time 4 "$url" 2>/dev/null)"
    elif command -v wget >/dev/null 2>&1; then
      ip="$(wget -qO- --timeout=4 "$url" 2>/dev/null)"
    fi
    ip="$(echo "$ip" | tr -d '[:space:]')"
    [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && { echo "$ip"; return; }
  done
  # Must succeed even when every endpoint is unreachable (egress-restricted VMs):
  # the caller assigns the output under `set -e`, so a non-zero status here would
  # abort the whole install instead of just skipping the public-IP DNS candidate.
  return 0
}

# validate_dns <ip> [more_ips...] — confirm derived hosts resolve to one of the given
# candidate IPs (this VM's local addresses + its public egress IP). Hard-fail in
# letsencrypt mode (ACME needs correct DNS); advisory otherwise. Populates DNS_ERRORS.
# shellcheck disable=SC2154  # AMP_HOST_*/AMP_AGENTS_BASE come from the caller's scope.
validate_dns() {
  local -a candidates=("$@")
  DNS_ERRORS=()
  local host got ip ok e
  for host in "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" \
              "$AMP_HOST_OBSERVER" "$AMP_HOST_GATEWAY" "${AMP_HOST_CP:-}" \
              "probe.${AMP_AGENTS_BASE}"; do
    [[ -z "$host" ]] && continue
    got="$(_resolve_host "$host")"
    if [[ -z "$got" ]]; then
      DNS_ERRORS+=("$host does not resolve to any A record")
      continue
    fi
    # Every A record must point at this VM (a resolver may return several / rotate
    # them), so a host that partly points elsewhere is caught regardless of order.
    while IFS= read -r ip; do
      [[ -z "$ip" ]] && continue
      ok=no
      for e in "${candidates[@]}"; do [[ -n "$e" && "$ip" == "$e" ]] && { ok=yes; break; }; done
      [[ "$ok" == yes ]] || DNS_ERRORS+=("$host resolves to '${ip}', not this VM (${candidates[*]})")
    done <<<"$got"
  done
  if (( ${#DNS_ERRORS[@]} )); then
    printf '[preflight] DNS issue: %s\n' "${DNS_ERRORS[@]}" >&2
    printf '[preflight] (advisory: certificate issuance uses DNS-01 and needs no inbound; point your DNS — or client /etc/hosts entries — at this VM so clients can reach the services)\n' >&2
  fi
  return 0
}
