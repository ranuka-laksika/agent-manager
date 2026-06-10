#!/bin/bash
set -euo pipefail

# Removes an environment: uninstalls its API Platform Gateway helm release, then
# deletes the Environment via the Agent Manager API (which in turn deletes it in
# OpenChoreo and cleans up link tables).
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/ai-agent-management-platform/main/deployments/scripts/remove-environment.sh \
#     | ENV_NAME=staging \
#       AGENT_MANAGER_TOKEN=<token> \
#       bash
#
# Required:
#   - ENV_NAME: the environment name to remove (cannot be 'default')
#   - AGENT_MANAGER_TOKEN: bearer token authorized to delete environments
# Optional:
#   - ORG_NAME (default: default)
#   - AGENT_MANAGER_URL (default: http://localhost:9000)
#   - GATEWAY_NAMESPACE (default: openchoreo-data-plane)

# --- Required inputs ---
: "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
: "${AGENT_MANAGER_TOKEN:?AGENT_MANAGER_TOKEN is required (bearer token)}"

if [ "$ENV_NAME" = "default" ]; then
    echo "❌ Cannot remove the default environment"
    exit 1
fi

# --- Configuration ---
ORG_NAME="${ORG_NAME:-default}"
AGENT_MANAGER_URL="${AGENT_MANAGER_URL:-http://localhost:9000}"
AGENT_MANAGER_API_URL="${AGENT_MANAGER_API_URL:-${AGENT_MANAGER_URL}/api/v1}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-openchoreo-data-plane}"

# Release name MUST match what add-environment.sh installs. Single org segment.
RELEASE_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
RELEASE_NAME=$(echo "$RELEASE_NAME" | head -c 53 | sed 's/-*$//')

echo "=== Removing Environment: ${ENV_NAME} ==="
echo ""

# --- Step 1: Uninstall the gateway helm release ---
echo "🌐 Uninstalling API Platform Gateway..."
if helm status "${RELEASE_NAME}" --namespace "${GATEWAY_NAMESPACE}" > /dev/null 2>&1; then
    helm uninstall "${RELEASE_NAME}" --namespace "${GATEWAY_NAMESPACE}"
    echo "✅ Gateway helm release uninstalled"
else
    echo "ℹ️  Gateway helm release '${RELEASE_NAME}' not found, skipping..."
fi

# Wait for gateway operator to clean up the APIGateway CR
GATEWAY_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
echo ""
echo "⏳ Waiting for gateway resources to be cleaned up..."
if kubectl wait --for=delete "apigateway/${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=120s 2>/dev/null; then
    echo "✅ Gateway resources cleaned up"
else
    echo "⚠️  Timed out or failed waiting for apigateway/${GATEWAY_NAME} to delete; continuing..."
fi

# --- Step 2: Delete the environment via Agent Manager API ---
echo ""
echo "⏳ Checking Agent Manager is healthy..."
MAX_WAIT=30
ELAPSED=0
until curl -sf "${AGENT_MANAGER_URL}/healthz" > /dev/null 2>&1; do
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_URL}/healthz after ${MAX_WAIT}s"
        exit 1
    fi
    sleep 3
    ELAPSED=$((ELAPSED + 3))
done

echo ""
echo "🌍 Deleting environment '${ENV_NAME}'..."
RESP_FILE="$(mktemp)"
trap 'rm -f "$RESP_FILE"' EXIT
DEL_HTTP_CODE=$(curl -s -o "$RESP_FILE" -w "%{http_code}" -X DELETE \
    "${AGENT_MANAGER_API_URL}/orgs/${ORG_NAME}/environments/${ENV_NAME}" \
    -H "Authorization: Bearer ${AGENT_MANAGER_TOKEN}")

case "$DEL_HTTP_CODE" in
    204)
        echo "✅ Environment '${ENV_NAME}' deleted"
        ;;
    404)
        echo "ℹ️  Environment '${ENV_NAME}' not found — already deleted"
        ;;
    *)
        echo "⚠️  Failed to delete environment (HTTP ${DEL_HTTP_CODE})"
        cat "$RESP_FILE" 2>/dev/null; echo
        exit 1
        ;;
esac

echo ""
echo "=== Environment '${ENV_NAME}' removed ==="
echo ""
