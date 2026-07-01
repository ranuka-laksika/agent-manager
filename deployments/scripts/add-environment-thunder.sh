#!/usr/bin/env bash
set -euo pipefail

# Provisions a dedicated Thunder ID instance for ONE environment (the home of that
# environment's agent identities). The platform Thunder (amp-thunder, console login)
# is separate and untouched — env-Thunders are added alongside it.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/add-environment-thunder.sh \
#     | ENV_NAME=staging \
#       DISPLAY_NAME="Staging" \
#       bash
#
# Re-running with the same ENV_NAME is idempotent (helm upgrade --install; the
# system-client secret is reused, never rotated).
#
# Prerequisites:
#   - kubectl and helm must be configured
#   - ENV_NAME: resource name (lowercase alphanumeric with hyphens)
#   - DISPLAY_NAME: human-readable name
# Optional:
#   - ORG_NAME (default: default)
#   - THUNDER_CHART: override the chart ref (default: oci://ghcr.io/wso2/wso2-amp-thunder-extension)
#   - CHART_VERSION: pin a published chart version (OCI charts only; latest when unset)
#   - THUNDER_NAMESPACE (default: derived amp-thunder-<org>-<env>)
#   - THUNDER_HOST (default: derived <org>-<env>.thunder.amp.localhost)
#   - SYSTEM_CLIENT_SECRET (default: generated; reused if one already exists)
#   - PERSISTENCE_SIZE (default: 1Gi), STORAGE_CLASS (default: cluster default)
#   - WAIT_TIMEOUT (default: 180s)
#   - OPENBAO_ADDR (default: http://localhost:8200) — OpenBao for storing the system-client secret
#   - OPENBAO_TOKEN (default: root)
#   - OPENBAO_PATH (default: secret) — KV mount path
#   Platform Thunder trusted-issuer (env-Thunder accepts platform Thunder tokens):
#   - PLATFORM_THUNDER_ISSUER   (default: http://thunder.amp.localhost:8080)
#   - PLATFORM_THUNDER_JWKS_URL (default: HTTPS JWKS endpoint of platform Thunder)
#   - PLATFORM_THUNDER_TOKEN_AUDIENCE (default: application)

# ---------------------------------------------------------------------------
# Pure helpers (sourced by the test suite; keep free of side effects).
# ---------------------------------------------------------------------------

# validate_name NAME -> 0 if a valid DNS-1123-ish label, non-zero otherwise.
validate_name() {
  printf '%s' "${1:-}" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
}

# _sha256 FILE -> full SHA-256 hex of a file (portable: shasum or sha256sum).
_sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

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
  local org="$1" env="$2" full hash prefix
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

# thunder_host ORG ENV -> single DNS label under thunder.amp.localhost
# (wildcard-cert friendly: *.thunder.amp.localhost), capped at 63 characters.
thunder_host() {
  local org="$1" env="$2" label hash prefix
  label="${org}-${env}"
  if [ "${#label}" -le 63 ]; then
    printf '%s.thunder.amp.localhost' "${label%-}"
    return 0
  fi
  hash="$(_sha6 "${org}/${env}")"
  prefix="${label:0:56}"
  prefix="${prefix%-}"
  printf '%s-%s.thunder.amp.localhost' "$prefix" "$hash"
}

# thunder_issuer ORG ENV -> the OIDC issuer / publicUrl (immutable once minting).
thunder_issuer() {
  printf 'http://%s:8080' "$(thunder_host "$1" "$2")"
}

# platform_thunder_issuer -> OIDC issuer of the shared platform Thunder instance.
# Env-Thunder trusts tokens bearing this issuer so callers can authenticate with
# a platform Thunder token instead of env-Thunder system-client credentials.
platform_thunder_issuer() {
  printf 'http://thunder.amp.localhost:8080'
}

# platform_thunder_jwks_url -> HTTPS JWKS URL that env-Thunder pods use to verify
# incoming platform Thunder tokens. Routed via the dedicated HTTPS Gateway on port
# 8443 (cert-manager-issued TLS, CA trusted via SSL_CERT_FILE inside the pod).
platform_thunder_jwks_url() {
  printf 'https://thunder.amp.localhost:8443/oauth2/jwks'
}

# platform_thunder_ca_cert -> prints the PEM CA cert that signed the
# thunder.amp.localhost TLS certificate, or returns 1 if not yet provisioned.
# Set PLATFORM_THUNDER_CA_PEM to inject a cert directly (useful in tests/CI).
platform_thunder_ca_cert() {
  if [ -n "${PLATFORM_THUNDER_CA_PEM:-}" ]; then
    printf '%s' "$PLATFORM_THUNDER_CA_PEM"
    return 0
  fi
  local b64
  b64="$(kubectl get secret amp-local-root-ca-secret -n cert-manager \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || true)"
  [ -z "$b64" ] && return 1
  printf '%s' "$b64" | _b64decode
}

# _b64decode (stdin) -> decoded bytes (openssl is portable across macOS/Linux).
_b64decode() {
  openssl base64 -d -A
}

# read_existing_secret NS NAME -> prints the stored client-secret, or returns 1.
read_existing_secret() {
  local ns="$1" name="$2" b64
  b64="$(kubectl get secret "$name" -n "$ns" -o jsonpath='{.data.client-secret}' 2>/dev/null || true)"
  [ -z "$b64" ] && return 1
  printf '%s' "$b64" | _b64decode
}

# write_to_openbao ORG ENV SECRET — writes the Thunder system-client secret to OpenBao
# so agent-manager-service can read it from both Docker (local dev) and Kubernetes (prod).
# Path: {OPENBAO_PATH}/data/thunder-system-clients/{org}/{env}
#
# Strategy: try direct HTTP first (works when port-forward is active), then fall back to
# kubectl exec into the OpenBao pod (works during 'make setup' before the port-forward starts).
write_to_openbao() {
  local org="$1" env_name="$2" secret_val="$3"
  local addr="${OPENBAO_ADDR:-http://localhost:8200}"
  local token="${OPENBAO_TOKEN:-root}"
  local mount="${OPENBAO_PATH:-secret}"
  local kv_path="thunder-system-clients/${org}/${env_name}"

  # --- attempt 1: direct HTTP (port-forward or explicit OPENBAO_ADDR) ---
  local http_code
  http_code="$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${addr}/v1/${mount}/data/${kv_path}" \
    -H "X-Vault-Token: ${token}" \
    -H "Content-Type: application/json" \
    -d '{"data":{"client-secret":"'"${secret_val}"'"}}' 2>/dev/null || echo "000")"

  case "$http_code" in
    200|204)
      echo "🔐 Stored system-client secret in OpenBao (${kv_path})"
      return 0
      ;;
  esac

  # --- attempt 2: kubectl exec into the OpenBao pod (no port-forward needed) ---
  if command -v kubectl &>/dev/null; then
    local openbao_pod
    openbao_pod="$(kubectl get pod -n openbao -l app.kubernetes.io/name=openbao \
      -o name 2>/dev/null | head -1)"
    if [ -n "$openbao_pod" ]; then
      if kubectl exec -n openbao "$openbao_pod" -- \
          env VAULT_ADDR="http://127.0.0.1:8200" VAULT_TOKEN="${token}" \
          bao kv put "${mount}/${kv_path}" "client-secret=${secret_val}" \
          >/dev/null 2>&1; then
        echo "🔐 Stored system-client secret in OpenBao via kubectl exec (${kv_path})"
        return 0
      fi
    fi
  fi

  echo "⚠️  Could not write to OpenBao (HTTP ${http_code:-unreachable}, kubectl exec also failed)"
  echo "   agent-manager-service uses OpenBao to resolve env-Thunder credentials."
  echo "   Re-run add-environment-thunder.sh once OpenBao is reachable."
  return 1
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------
main() {
  : "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
  : "${DISPLAY_NAME:?DISPLAY_NAME is required (e.g. DISPLAY_NAME=\"Staging\")}"

  local org="${ORG_NAME:-default}"

  if ! validate_name "$ENV_NAME"; then
    echo "❌ Invalid ENV_NAME '${ENV_NAME}'"
    echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen)."
    exit 1
  fi
  if ! validate_name "$org"; then
    echo "❌ Invalid ORG_NAME '${org}'"
    echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen)."
    exit 1
  fi

  local release ns host issuer chart secret_name
  release="$(thunder_release_name "$org" "$ENV_NAME")"
  ns="${THUNDER_NAMESPACE:-$(thunder_namespace "$org" "$ENV_NAME")}"
  host="${THUNDER_HOST:-$(thunder_host "$org" "$ENV_NAME")}"
  issuer="http://${host}:8080"
  chart="${THUNDER_CHART:-oci://ghcr.io/wso2/wso2-amp-thunder-extension}"
  secret_name="${release}-system-client"

  local persistence_size="${PERSISTENCE_SIZE:-1Gi}"
  local storage_class="${STORAGE_CLASS:-}"
  local wait_timeout="${WAIT_TIMEOUT:-180s}"

  # Platform Thunder coordinates — CORS origin + trusted-issuer JWKS (HTTPS via port 8443).
  local pt_issuer pt_jwks pt_audience
  pt_issuer="${PLATFORM_THUNDER_ISSUER:-$(platform_thunder_issuer)}"
  pt_jwks="${PLATFORM_THUNDER_JWKS_URL:-$(platform_thunder_jwks_url)}"
  pt_audience="${PLATFORM_THUNDER_TOKEN_AUDIENCE:-application}"

  echo "=== Provisioning Thunder ID for environment '${ENV_NAME}' (org '${org}') ==="
  echo ""
  echo "  Release:   ${release}"
  echo "  Namespace: ${ns}"
  echo "  Issuer:    ${issuer}"
  echo ""

  # Resolve chart version for OCI charts (local chart paths skip this).
  local version_args=()
  if printf '%s' "$chart" | grep -q '^oci://'; then
    local chart_version="${CHART_VERSION:-}"
    if [ -z "$chart_version" ]; then
      echo "🔎 Resolving latest Thunder chart version from ${chart}..."
      chart_version="$(helm show chart "$chart" 2>/dev/null | awk '/^version:/ {print $2; exit}')"
      if [ -z "$chart_version" ]; then
        echo "❌ Could not resolve the latest chart version from ${chart}"
        echo "   Pin a version explicitly and retry (e.g. CHART_VERSION=0.1.0)."
        exit 1
      fi
      echo "✅ Using chart version: ${chart_version}"
    else
      echo "📌 Using pinned chart version: ${chart_version}"
    fi
    version_args=(--version "$chart_version")
  fi

  # Ensure the namespace exists (idempotent) so the secrets can live in it.
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

  # Resolve the system-client secret: reuse an existing one (NO rotation), else
  # mint a unique one and store it as a K8s Secret in the env-Thunder namespace.
  local system_secret
  if system_secret="$(read_existing_secret "$ns" "$secret_name")" && [ -n "$system_secret" ]; then
    echo "🔐 Reusing existing system-client secret (${secret_name})"
  else
    system_secret="${SYSTEM_CLIENT_SECRET:-$(openssl rand -hex 24)}"
    kubectl create secret generic "$secret_name" -n "$ns" \
      --from-literal=client-secret="$system_secret" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    echo "🔐 Stored new system-client secret (${secret_name})"
  fi
  # Mirror the secret to OpenBao so agent-manager-service can read it from Docker and K8s.
  if ! write_to_openbao "$org" "$ENV_NAME" "$system_secret"; then
    exit 1
  fi

  # Per-env overrides. The chart's defaults serve the platform Thunder; these turn
  # one chart into an isolated per-environment instance.
  local set_args=(
    --set-string "thunder.ocIngress.hostname=${host}"
    --set-string "thunder.configuration.server.publicUrl=${issuer}"
    --set-string "thunder.configuration.jwt.issuer=${issuer}"
    --set-string "thunder.configuration.gateClient.hostname=${host}"
    --set "thunder.persistence.enabled=true"
    --set "thunder.persistence.size=${persistence_size}"
    --set-string "thunder.bootstrap.ampSystemClient.clientSecret=${system_secret}"
    # CORS: allow the platform Thunder origin so its console can reach env-Thunder APIs.
    --set "thunder.configuration.cors.allowedOrigins={http://localhost:3000,${pt_issuer}}"
    # Disable the HTTPS gateway on env-Thunder instances (it's only needed on platform Thunder).
    --set "thunder.ocIngress.https.enabled=false"
  )
  if [ -n "$storage_class" ]; then
    set_args+=(--set-string "thunder.persistence.storageClass=${storage_class}")
  fi

  # ---------------------------------------------------------------------------
  # Trusted issuer: env-Thunder accepts tokens issued by platform Thunder.
  #
  # Flow:
  #   1. Fetch the self-signed CA cert that signed the platform Thunder TLS cert
  #      from cert-manager (or from PLATFORM_THUNDER_CA_PEM if injected by the
  #      caller — useful in tests/CI where cert-manager is not running).
  #   2. Fetch the Mozilla CA bundle (the same set shipped by Alpine / Debian
  #      ca-certificates packages) so the combined file is a complete trust store.
  #   3. Store the combined PEM bundle as a ConfigMap in the env-Thunder namespace.
  #   4. Mount it into the env-Thunder pod and set SSL_CERT_FILE to the combined
  #      file so Go's TLS stack trusts both commercial CAs and the self-signed CA.
  # ---------------------------------------------------------------------------
  local ca_pem ca_bundle mozilla_bundle tmp_bundle expected_sha actual_sha
  local ca_cm_name="amp-thunder-platform-ca"

  # Fetch platform Thunder CA. Missing cert is fatal to prevent silent auth failures.
  if ! ca_pem="$(platform_thunder_ca_cert)" || [ -z "$ca_pem" ]; then
    echo "❌ Platform Thunder CA cert is not available."
    echo "   Ensure cert-manager has issued amp-local-root-ca in the cert-manager namespace."
    echo "   Check status: kubectl get certificate -n cert-manager amp-local-root-ca"
    echo "   Alternatively, set PLATFORM_THUNDER_CA_PEM to inject the CA cert directly."
    exit 1
  fi

  # Fetch Mozilla root CA bundle. Appending our Root CA ensures the trust store
  # remains additive and compatible with any base image (Debian/Alpine).
  # The bundle is built on the operator's machine (not inside the pod), so we
  # cannot rely on the pod's /etc/ssl — we need a portable external source.
  echo "🔐 Fetching Mozilla CA bundle from curl.se..."
  tmp_bundle="$(mktemp)"
  local attempt verified
  verified=false
  for attempt in 1 2 3; do
    if ! curl -fsSL --connect-timeout 30 https://curl.se/ca/cacert.pem -o "$tmp_bundle" 2>/dev/null \
        || ! grep -q "BEGIN CERTIFICATE" "$tmp_bundle"; then
      rm -f "$tmp_bundle"
      echo "❌ Could not fetch Mozilla CA bundle from https://curl.se/ca/cacert.pem"
      echo "   Download it on a machine with internet access, then re-run:"
      echo "     curl -fsSL https://curl.se/ca/cacert.pem -o /tmp/cacert.pem"
      echo "     ENV_NAME=... bash $(basename "$0")"
      exit 1
    fi
    # Verify against the published checksum to detect download corruption.
    if expected_sha="$(curl -fsSL --connect-timeout 15 https://curl.se/ca/cacert.pem.sha256 2>/dev/null | awk '{print $1}')" \
        && [ -n "$expected_sha" ]; then
      actual_sha="$(_sha256 "$tmp_bundle")"
      if [ "$expected_sha" != "$actual_sha" ]; then
        if [ "$attempt" -lt 3 ]; then
          echo "⚠️  Checksum mismatch on attempt ${attempt}/3 — retrying..."
          sleep 2
          continue
        fi
        rm -f "$tmp_bundle"
        echo "❌ Mozilla CA bundle checksum mismatch after 3 attempts — download may be corrupt."
        echo "   Expected: $expected_sha"
        echo "   Got:      $actual_sha"
        exit 1
      fi
      echo "   ✓ Checksum verified (SHA-256: ${actual_sha:0:16}...)"
    fi
    verified=true
    break
  done
  mozilla_bundle="$(cat "$tmp_bundle")"
  rm -f "$tmp_bundle"
  ca_bundle="${mozilla_bundle}
${ca_pem}"

  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1 || true

  # Store the combined bundle as a ConfigMap.
  kubectl create configmap "$ca_cm_name" -n "$ns" \
    --from-literal=ca-bundle.crt="${ca_bundle}" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  echo "🔐 Combined CA bundle (Mozilla + platform Thunder CA) stored in ${ns}/${ca_cm_name}"

  set_args+=(
    --set-json "thunder.extraVolumes=[{\"name\":\"platform-ca\",\"configMap\":{\"name\":\"${ca_cm_name}\"}}]"
    --set-json "thunder.extraVolumeMounts=[{\"name\":\"platform-ca\",\"mountPath\":\"/etc/ssl/amp\",\"readOnly\":true}]"
    # Inject SSL_CERT_FILE pointing to the combined bundle. Use index-based append
    # to preserve default env vars in values.yaml.
    --set "thunder.deployment.env[0].name=SSL_CERT_FILE"
    --set "thunder.deployment.env[0].value=/etc/ssl/amp/ca-bundle.crt"
    # Configure the trusted issuer endpoints.
    --set-string "thunder.configuration.server.security.trustedIssuer.issuer=${pt_issuer}"
    --set-string "thunder.configuration.server.security.trustedIssuer.jwksUrl=${pt_jwks}"
    --set-string "thunder.configuration.server.security.trustedIssuer.audience=${pt_audience}"
  )

  echo ""
  echo "📦 Installing Thunder (${release})..."
  helm upgrade --install "$release" "$chart" \
    ${version_args[@]+"${version_args[@]}"} \
    --namespace "$ns" --create-namespace \
    "${set_args[@]}"

  echo ""
  echo "⏳ Waiting for Thunder '${release}' to be ready..."
  if kubectl wait --for=condition=available --timeout="$wait_timeout" \
      deployment -l "app.kubernetes.io/instance=${release}" -n "$ns" 2>/dev/null; then
    echo "✅ Thunder is ready"
  else
    echo "⚠️  Thunder did not become ready in time — check: kubectl get pods -n ${ns}"
  fi

  echo ""
  echo "=== Thunder ID for '${ENV_NAME}' provisioned ==="
  echo ""
  echo "  Environment:     ${ENV_NAME}"
  echo "  Namespace:       ${ns}"
  echo "  Release:         ${release}"
  echo "  Issuer:          ${issuer}"
  echo "  JWKS:            ${issuer}/oauth2/jwks"
  echo "  Trusted issuer:  ${pt_issuer}"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Thunder ID Console — ${ENV_NAME}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  URL:      ${issuer}/console"
  echo "  Username: admin"
  echo "  Password: admin"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
}

# Run main only when executed directly — not when sourced (e.g. by tests).
# BASH_SOURCE[0] is unset when the script is piped to bash (curl ... | bash);
# ${BASH_SOURCE[0]:-$0} falls back to $0 (which equals "bash") so the condition
# stays true and main runs, while sourced execution still sees the script filename.
if [ "${BASH_SOURCE[0]:-$0}" = "${0}" ]; then
  main "$@"
fi
