#!/bin/bash
set -e

# Installs the API Platform Gateway extension chart.
# Must run AFTER Agent Manager is up and migrations have completed,
# because the bootstrap job registers the gateway via the Agent Manager API.
#
#   setup-gateway.sh           # default: agent-manager runs via docker-compose


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Gateway identity for the bootstrap default environment. Keep in sync with
# add-environment.sh so every env's gateway record uses the same vhost convention
# (otherwise the agent-manager `gateways.vhost` row is set from the chart's
# misleading static default).
ORG_NAME="${ORG_NAME:-default}"
ENV_NAME="${ENV_NAME:-default}"
GATEWAY_VHOST_PORT="${GATEWAY_VHOST_PORT:-19080}"
GATEWAY_VHOST="${GATEWAY_VHOST:-http://${ENV_NAME}-${ORG_NAME}.gateway.localhost:${GATEWAY_VHOST_PORT}}"

echo "=== Installing API Platform Gateway ==="

# Verify Agent Manager is reachable
echo "⏳ Checking Agent Manager is healthy..."
MAX_WAIT=60
ELAPSED=0
AGENT_MANAGER_HEALTH_URL="${AGENT_MANAGER_HEALTH_URL:-http://localhost:9000/healthz}"
until curl -sf "$AGENT_MANAGER_HEALTH_URL" > /dev/null 2>&1; do
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_HEALTH_URL} after ${MAX_WAIT}s"
        echo "   Make sure docker-compose services are up and migrations have run."
        exit 1
    fi
    sleep 3
    ELAPSED=$((ELAPSED + 3))
done
echo "✅ Agent Manager is healthy"

echo ""
echo "🌐 Installing gateway chart..."
helm upgrade --install "api-platform-${ORG_NAME}-${ENV_NAME}" \
    "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension" \
    --namespace openchoreo-data-plane \
    --set agentManager.orgName="${ORG_NAME}" \
    --set gateway.environment="${ENV_NAME}" \
    --set gateway.vhost="${GATEWAY_VHOST}" \
    --set agentManager.apiUrl="http://host.docker.internal:9000/api/v1" \
    --set apiGateway.controlPlane.host="host.docker.internal:9243" \
    -f "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension/values-dev.yaml"

echo "⏳ Waiting for Gateway to be ready..."
if kubectl wait --for=condition=Programmed "apigateway/api-platform-${ORG_NAME}-${ENV_NAME}" -n openchoreo-data-plane --timeout=180s; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time"
fi

# The OTEL ingest RestApi is provisioned by the gateway extension chart
# (templates/gateway-otel-restapi.yaml) with the restapi-target label that
# binds it to this gateway. Wait for that chart-managed route to program; it
# carries the jwt-auth claim mappings agents need to export traces. (The
# standalone values/otel-collector-rest-api.yaml is not applied here: it lacks
# the restapi-target label, so it can never bind and stays GatewayNotReady.)
OTEL_RESTAPI="api-platform-${ORG_NAME}-${ENV_NAME}-otel-restapi"

echo "⏳ Waiting for OTEL ingest RestApi to be programmed..."
if kubectl wait --for=condition=Programmed "restapi/${OTEL_RESTAPI}" -n openchoreo-data-plane --timeout=300s; then
    echo "✅ OTEL ingest RestApi is programmed"
else
    echo "❌ RestApi ${OTEL_RESTAPI} did not become Programmed in time"
    kubectl describe "restapi/${OTEL_RESTAPI}" -n openchoreo-data-plane || true
    exit 1
fi

echo ""
echo "✅ API Platform Gateway installed"
