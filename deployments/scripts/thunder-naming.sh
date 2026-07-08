#!/bin/bash
# Thunder ID naming helpers — the SINGLE source of truth for the bash side of
# env-Thunder's release-name / namespace / host derivation (53-char cap,
# truncate-to-46 + sha256-6, hyphen trimming for release names; 63/56 for
# hosts). Meant to be sourced, not executed directly.
#
# Every script that provisions, removes, or wires the gateway to a per-env
# Thunder instance sources THIS file instead of re-implementing these
# functions — see each caller's "_load_thunder_naming_lib" for how it's
# fetched when run standalone via curl | bash. Do not add a copy of these
# functions anywhere else.
#
# Go can't source bash, so agent-manager-service/clients/thundersvc/naming.go
# necessarily keeps its OWN implementation of the same algorithm
# (ThunderReleaseName/ThunderNamespace/ThunderHost/ThunderIssuerURL). Its
# maxReleaseNameLen/truncatePrefixLen constants (53/46) must stay numerically
# identical to this file's — that pairing is the one naming duplication that
# can't be eliminated, only kept in sync by convention.

# _sha6 STRING -> first 6 hex chars of its sha256 (portable: shasum or sha256sum).
_sha6() {
  if command -v shasum >/dev/null 2>&1; then
    printf '%s' "$1" | shasum -a 256 | cut -c1-6
  else
    printf '%s' "$1" | sha256sum | cut -c1-6
  fi
}

# thunder_release_name ORG ENV -> helm release name, <=53 chars, collision-safe.
# Twin of the gateway release `amp-thunder-<org>-<env>`.
thunder_release_name() {
  local org env full hash prefix
  org="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  env="$(printf '%s' "$2" | tr '[:upper:]' '[:lower:]')"
  full="amp-thunder-${org}-${env}"
  if [ "${#full}" -le 53 ]; then
    printf '%s' "${full%-}"
    return 0
  fi
  # Truncate the readable part and append a deterministic hash of the FULL input
  # so distinct environments never collapse into one shared instance.
  hash="$(_sha6 "${org}/${env}")"
  prefix="${full:0:46}"
  prefix="${prefix%-}"
  printf '%s-%s' "$prefix" "$hash"
}

# thunder_namespace ORG ENV -> dedicated namespace (mirrors the release name).
thunder_namespace() {
  thunder_release_name "$1" "$2"
}

# thunder_host ORG ENV -> single DNS label under thunder.<THUNDER_HOST_BASE_DOMAIN>
# (wildcard-cert friendly: *.thunder.<base-domain>), capped at 63 characters.
#
# THUNDER_HOST_BASE_DOMAIN defaults to "amp.localhost" (local dev, k3d's
# *.amp.localhost wildcard cert). VM/production deployments override it
# deployment-wide (see deployments/vm/lib-vm.sh) — NEVER per call: the Go side
# (agent-manager-service/clients/thundersvc/naming.go's ThunderHost) always
# computes this same value purely from (org, env) plus its OWN copy of this same
# config (THUNDER_HOST_BASE_DOMAIN env var), with no way to learn about a one-off
# override here. Both sides must be set to the identical value on any given
# deployment, or the URLs AMS reports and the host env-Thunder actually answers to
# will diverge.
thunder_host() {
  local org env label hash prefix base_domain
  org="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  env="$(printf '%s' "$2" | tr '[:upper:]' '[:lower:]')"
  base_domain="${THUNDER_HOST_BASE_DOMAIN:-amp.localhost}"
  label="${org}-${env}"
  if [ "${#label}" -le 63 ]; then
    printf '%s.thunder.%s' "${label%-}" "$base_domain"
    return 0
  fi
  hash="$(_sha6 "${org}/${env}")"
  prefix="${label:0:56}"
  prefix="${prefix%-}"
  printf '%s-%s.thunder.%s' "$prefix" "$hash" "$base_domain"
}

# thunder_issuer ORG ENV -> the OIDC issuer / publicUrl (immutable once minting).
#
# TLS_ENABLED (default false; the SAME flag deployments/vm/lib-vm.sh already sets
# for platform Thunder's own advertised URLs) switches to https with no explicit
# port, matching a VM's Caddy terminating TLS on the standard HTTPS port instead of
# the k3d gateway's plain-HTTP :8080 used in local dev.
thunder_issuer() {
  if [ "${TLS_ENABLED:-false}" = "true" ]; then
    printf 'https://%s' "$(thunder_host "$1" "$2")"
  else
    printf 'http://%s:8080' "$(thunder_host "$1" "$2")"
  fi
}
