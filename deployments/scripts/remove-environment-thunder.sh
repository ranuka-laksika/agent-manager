#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

# Removes the dedicated Thunder ID instance for ONE environment.
# Mirrors add-environment-thunder.sh — call this before removing the environment itself.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/remove-environment-thunder.sh \
#     | ENV_NAME=staging \
#       bash
#
# Re-running is idempotent: if the release or namespace is already gone the step
# is skipped gracefully.
#
# Required:
#   - ENV_NAME: the environment name (lowercase alphanumeric with hyphens)
# Optional:
#   - ORG_NAME (default: default)

# ---------------------------------------------------------------------------
# Pure helpers
# ---------------------------------------------------------------------------

validate_name() {
  printf '%s' "${1:-}" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
}

# Load the shared Thunder naming helpers (thunder_release_name/thunder_namespace)
# — the single source of truth for this derivation, see
# deployments/scripts/thunder-naming.sh. Prefers a local sibling file
# (checked-out repo); falls back to fetching it from the same ref this script
# itself would be fetched from when piped via curl | bash.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh" ]; then
  # shellcheck source=thunder-naming.sh
  source "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh"
else
  _naming_lib_url="${THUNDER_NAMING_LIB_URL:-${SCRIPT_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts}/thunder-naming.sh}"
  _naming_lib_tmp="$(mktemp)"
  if ! curl -fsSL "${_naming_lib_url}" -o "${_naming_lib_tmp}"; then
    echo "❌ Failed to fetch Thunder naming helpers from ${_naming_lib_url}" >&2
    rm -f "${_naming_lib_tmp}"
    exit 1
  fi
  # shellcheck source=/dev/null
  source "${_naming_lib_tmp}"
  rm -f "${_naming_lib_tmp}"
  unset _naming_lib_url _naming_lib_tmp
fi

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------
main() {
  : "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"

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

  # Namespace is ALWAYS computed from (org, env) — never overridable — so removal
  # targets exactly the namespace add-environment-thunder.sh actually provisioned.
  local release ns
  release="$(thunder_release_name "$org" "$ENV_NAME")"
  ns="$(thunder_namespace "$org" "$ENV_NAME")"

  echo "=== Removing Thunder ID for environment '${ENV_NAME}' (org '${org}') ==="
  echo ""
  echo "  Release:   ${release}"
  echo "  Namespace: ${ns}"
  echo ""

  # --- Step 1: Uninstall the Thunder helm release ---
  echo "🗑️  Uninstalling Thunder helm release..."
  if helm status "$release" --namespace "$ns" > /dev/null 2>&1; then
    helm uninstall "$release" --namespace "$ns"
    echo "✅ Thunder helm release uninstalled"
  else
    echo "ℹ️  Thunder release '${release}' not found — already removed or never installed"
  fi

  # --- Step 1b: Delete the cross-namespace HTTPRoute ---
  # add-environment-thunder.sh applies this directly via kubectl (not part of the
  # Helm release, since it lives in openchoreo-control-plane rather than the
  # release's own namespace), so it is NOT removed by `helm uninstall` above.
  echo ""
  echo "🗑️  Deleting HTTPRoute in openchoreo-control-plane..."
  if kubectl get httproute "$release" -n openchoreo-control-plane > /dev/null 2>&1; then
    kubectl delete httproute "$release" -n openchoreo-control-plane
    echo "✅ HTTPRoute deleted"
  else
    echo "ℹ️  HTTPRoute '${release}' not found — already removed or never created"
  fi

  # --- Step 2: Delete the Thunder namespace ---
  # The namespace holds the system-client Secret (created outside Helm); deleting
  # the namespace ensures full cleanup even if Helm tracking was incomplete.
  echo ""
  echo "🗑️  Deleting Thunder namespace '${ns}'..."
  if kubectl get namespace "$ns" > /dev/null 2>&1; then
    kubectl delete namespace "$ns"
    echo "✅ Thunder namespace deleted"
  else
    echo "ℹ️  Namespace '${ns}' not found — already deleted"
  fi

  echo ""
  echo "=== Thunder ID for '${ENV_NAME}' removed ==="
  echo ""
  echo "  Environment: ${ENV_NAME}"
  echo "  Release:     ${release}"
  echo "  Namespace:   ${ns}"
  echo ""
}

# Run main only when executed directly — not when sourced (e.g. by tests).
# BASH_SOURCE[0] is unset when the script is piped to bash (curl ... | bash);
# ${BASH_SOURCE[0]:-$0} falls back to $0 (which equals "bash") so the condition
# stays true and main runs, while sourced execution still sees the script filename.
if [ "${BASH_SOURCE[0]:-$0}" = "${0}" ]; then
  main "$@"
fi
