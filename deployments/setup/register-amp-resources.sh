#!/bin/bash
# Registers the Agent Manager (amp) resource server and all 81 permissions in Thunder.
# Runs against the external Thunder endpoint using amp-system-client credentials.
#
# Usage:
#   ./register-amp-resources.sh
#   THUNDER_URL=http://thunder.amp.localhost:8080 ./register-amp-resources.sh

set -e

THUNDER_URL="${THUNDER_URL:-http://thunder.amp.localhost:8080}"
CLIENT_ID="${THUNDER_CLIENT_ID:-amp-system-client}"
CLIENT_SECRET="${THUNDER_CLIENT_SECRET:-amp-system-client-secret}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
log_info()    { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} ✓ $1" >&2; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} ⚠ $1" >&2; }
log_error()   { echo -e "${RED}[ERROR]${NC} ✗ $1" >&2; }

command -v jq >/dev/null 2>&1 || { log_error "jq is required but not installed"; exit 1; }

# ---------------------------------------------------------------------------
# Get a system-scoped token
# ---------------------------------------------------------------------------
log_info "Obtaining system token from $THUNDER_URL ..."
TOKEN_RESPONSE=$(curl -s -X POST "$THUNDER_URL/oauth2/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -u "$CLIENT_ID:$CLIENT_SECRET" \
  -d "grant_type=client_credentials&scope=system")

TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')
if [[ -z "$TOKEN" ]]; then
  log_error "Failed to obtain system token. Response: $TOKEN_RESPONSE"
  exit 1
fi
log_success "System token obtained."

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
api_call() {
  local method="$1" endpoint="$2" data="${3:-}"
  if [[ -z "$data" ]]; then
    curl -s -w "\n%{http_code}" -X "$method" "$THUNDER_URL$endpoint" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo -e "\n000"
  else
    curl -s -w "\n%{http_code}" -X "$method" "$THUNDER_URL$endpoint" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "$data" 2>/dev/null || echo -e "\n000"
  fi
}

create_or_get_rs() {
  local name="$1" handle="$2" identifier="$3" description="$4" ou_id="$5"
  local payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"identifier\":\"${identifier}\",\"description\":\"${description}\",\"ouId\":\"${ou_id}\"}"
  local response http_code body id

  response=$(api_call POST "/resource-servers" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]]; then
    id=$(echo "$body" | jq -r '.id')
    log_success "Resource server '${identifier}' created (id: $id)"
    echo "$id"; return 0
  fi

  if [[ "$http_code" == "409" ]]; then
    log_warning "Resource server '${identifier}' already exists, retrieving ID..."
    response=$(api_call GET "/resource-servers")
    http_code="${response: -3}"; body="${response%???}"
    [[ "$http_code" != "200" ]] && { log_error "Failed to list resource servers (HTTP $http_code)"; exit 1; }
    id=$(echo "$body" | jq -r --arg ident "$identifier" '.resourceServers[] | select(.identifier == $ident) | .id')
    [[ -z "$id" ]] && { log_error "Resource server '${identifier}' not found after 409"; exit 1; }
    log_success "Found existing resource server '${identifier}' (id: $id)"
    echo "$id"; return 0
  fi

  log_error "Failed to create resource server '${identifier}' (HTTP $http_code): $body"
  exit 1
}

create_or_get_resource() {
  local rs_id="$1" name="$2" handle="$3" description="$4" parent_id="${5:-}"
  local payload list_url response http_code body id

  if [[ -n "$parent_id" ]]; then
    payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\",\"parent\":\"${parent_id}\"}"
    list_url="/resource-servers/${rs_id}/resources?parentId=${parent_id}"
  else
    payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\"}"
    list_url="/resource-servers/${rs_id}/resources"
  fi

  response=$(api_call POST "/resource-servers/${rs_id}/resources" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]]; then
    id=$(echo "$body" | jq -r '.id')
    echo "$id"; return 0
  fi

  if [[ "$http_code" == "409" ]]; then
    log_warning "Resource '${handle}' already exists, retrieving ID..."
    response=$(api_call GET "${list_url}")
    http_code="${response: -3}"; body="${response%???}"
    [[ "$http_code" != "200" ]] && { log_error "Failed to list resources (HTTP $http_code)"; exit 1; }
    id=$(echo "$body" | jq -r --arg h "$handle" '.resources[] | select(.handle == $h) | .id')
    [[ -z "$id" ]] && { log_error "Resource '${handle}' not found after 409"; exit 1; }
    log_success "Found existing resource '${handle}' (id: $id)"
    echo "$id"; return 0
  fi

  log_error "Failed to create resource '${handle}' (HTTP $http_code): $body"
  exit 1
}

create_action() {
  local rs_id="$1" res_id="$2" name="$3" handle="$4" description="$5"
  local payload="{\"name\":\"${name}\",\"handle\":\"${handle}\",\"description\":\"${description}\"}"
  local response http_code body

  response=$(api_call POST "/resource-servers/${rs_id}/resources/${res_id}/actions" "$payload")
  http_code="${response: -3}"; body="${response%???}"

  if [[ "$http_code" == "201" ]] || [[ "$http_code" == "200" ]] || [[ "$http_code" == "409" ]]; then
    return 0
  fi

  log_error "Failed to create action '${handle}' on resource ${res_id} (HTTP $http_code): $body"
  exit 1
}

# ===========================================================================
# 0. Fetch default OU ID (required by Thunder for resource server creation)
# ===========================================================================
log_info "Fetching default organization unit ID..."
OU_RESPONSE=$(api_call GET "/organization-units/tree/default")
OU_HTTP="${OU_RESPONSE: -3}"; OU_BODY="${OU_RESPONSE%???}"
if [[ "$OU_HTTP" != "200" ]]; then
  log_error "Failed to fetch default OU (HTTP $OU_HTTP): $OU_BODY"
  exit 1
fi
DEFAULT_OU_ID=$(echo "$OU_BODY" | jq -r '.id')
if [[ -z "$DEFAULT_OU_ID" ]]; then
  log_error "Could not extract default OU ID from response"
  exit 1
fi
log_success "Default OU ID: $DEFAULT_OU_ID"

# ===========================================================================
# 1. Resource server
# ===========================================================================
log_info "Creating 'amp' resource server..."
RS_ID=$(create_or_get_rs "Agent Manager API" "amp" "amp" "Agent Manager platform permissions" "$DEFAULT_OU_ID")
log_info "Resource server ready (id: $RS_ID)"

# ===========================================================================
# 2. Level-1 resources
# ===========================================================================
log_info "Creating level-1 resources..."
R_ORG=$(create_or_get_resource    "$RS_ID" "Organization"           "org"                   "Organizational unit management")
R_PROJECT=$(create_or_get_resource "$RS_ID" "Project"               "project"               "Project management")
R_ENV=$(create_or_get_resource     "$RS_ID" "Environment"           "environment"           "Environment management")
R_GW=$(create_or_get_resource      "$RS_ID" "Gateway"               "gateway"               "Gateway management")
R_DP=$(create_or_get_resource      "$RS_ID" "Data Plane"            "data-plane"            "Data plane visibility")
R_PIPE=$(create_or_get_resource    "$RS_ID" "Deployment Pipeline"   "deployment-pipeline"   "Deployment pipeline visibility")
R_GIT=$(create_or_get_resource     "$RS_ID" "Git Secret"            "git-secret"            "Git credential management")
R_LLMT=$(create_or_get_resource    "$RS_ID" "LLM Provider Template" "llm-provider-template" "LLM provider template management")
R_LLM=$(create_or_get_resource     "$RS_ID" "LLM Provider"          "llm-provider"          "LLM provider management")
R_MCP=$(create_or_get_resource     "$RS_ID" "MCP Server"            "mcp-server"            "MCP server management")
R_PROXY=$(create_or_get_resource   "$RS_ID" "LLM Proxy"             "llm-proxy"             "LLM proxy management")
R_EVAL=$(create_or_get_resource    "$RS_ID" "Evaluator"             "evaluator"             "Evaluator management")
R_AGENT=$(create_or_get_resource   "$RS_ID" "Agent"                 "agent"                 "Agent management")
R_MON=$(create_or_get_resource     "$RS_ID" "Monitor"               "monitor"               "Monitor management")
R_OBS=$(create_or_get_resource     "$RS_ID" "Observability"         "observability"         "Observability dashboards and metrics")
R_ROLE=$(create_or_get_resource    "$RS_ID" "Role"                  "role"                  "Role management")
R_GROUP=$(create_or_get_resource   "$RS_ID" "Group"                 "group"                 "Group management")
R_CAT=$(create_or_get_resource     "$RS_ID" "Catalog"               "catalog"               "Resource catalog")
R_REPO=$(create_or_get_resource    "$RS_ID" "Repository"            "repository"            "Source repository browsing")
log_info "Level-1 resources created."

# ===========================================================================
# 3. Level-2 sub-resources
# ===========================================================================
log_info "Creating level-2 sub-resources..."
R_GW_TOKEN=$(create_or_get_resource    "$RS_ID" "Gateway Token"        "token"   "Gateway token management"           "$R_GW")
R_LLM_KEY=$(create_or_get_resource     "$RS_ID" "LLM Provider API Key" "api-key" "LLM provider API key management"    "$R_LLM")
R_PROXY_KEY=$(create_or_get_resource   "$RS_ID" "LLM Proxy API Key"    "api-key" "LLM proxy API key management"       "$R_PROXY")
R_AGENT_DEPLOY=$(create_or_get_resource "$RS_ID" "Agent Deployment"    "deploy"  "Agent deployment operations"        "$R_AGENT")
R_AGENT_TOKEN=$(create_or_get_resource  "$RS_ID" "Agent Token"         "token"   "Agent token management"             "$R_AGENT")
R_MON_SCORE=$(create_or_get_resource   "$RS_ID" "Monitor Score"        "score"   "Monitor score management"           "$R_MON")
log_info "Level-2 sub-resources created."

# ===========================================================================
# 4. Actions
# ===========================================================================
log_info "Creating actions..."

create_action "$RS_ID" "$R_ORG"          "View"                   "view"                   "View organizational details"
create_action "$RS_ID" "$R_ORG"          "Modify Settings"        "modify-settings"        "Modify organizational settings"
create_action "$RS_ID" "$R_ORG"          "Invite Member"          "invite-member"          "Invite members to the organization"
create_action "$RS_ID" "$R_ORG"          "Remove Member"          "remove-member"          "Remove members from the organization"
create_action "$RS_ID" "$R_ORG"          "Assign Role"            "assign-role"            "Assign or revoke roles"
create_action "$RS_ID" "$R_ORG"          "Manage IDP"             "manage-idp"             "Configure Identity Provider"
create_action "$RS_ID" "$R_ORG"          "Manage Service Account" "manage-service-account" "Manage service accounts"

create_action "$RS_ID" "$R_PROJECT"      "Create"  "create"  "Create a project"
create_action "$RS_ID" "$R_PROJECT"      "Read"    "read"    "View project details"
create_action "$RS_ID" "$R_PROJECT"      "Update"  "update"  "Update project settings"
create_action "$RS_ID" "$R_PROJECT"      "Delete"  "delete"  "Delete a project"

create_action "$RS_ID" "$R_ENV"          "Create"  "create"  "Create an environment"
create_action "$RS_ID" "$R_ENV"          "Read"    "read"    "View environment details"
create_action "$RS_ID" "$R_ENV"          "Update"  "update"  "Update an environment"
create_action "$RS_ID" "$R_ENV"          "Delete"  "delete"  "Delete an environment"

create_action "$RS_ID" "$R_GW"           "Create"  "create"  "Register a gateway"
create_action "$RS_ID" "$R_GW"           "Read"    "read"    "View gateway details and status"
create_action "$RS_ID" "$R_GW"           "Update"  "update"  "Update gateway configuration"
create_action "$RS_ID" "$R_GW"           "Delete"  "delete"  "Delete a gateway"
create_action "$RS_ID" "$R_GW_TOKEN"     "Manage"  "manage"  "Create, list, and delete gateway tokens"

create_action "$RS_ID" "$R_DP"           "Read"    "read"    "View data planes"
create_action "$RS_ID" "$R_PIPE"         "Create"  "create"  "Create a deployment pipeline"
create_action "$RS_ID" "$R_PIPE"         "Read"    "read"    "View deployment pipelines"
create_action "$RS_ID" "$R_PIPE"         "Update"  "update"  "Update a deployment pipeline"
create_action "$RS_ID" "$R_PIPE"         "Delete"  "delete"  "Delete a deployment pipeline"

create_action "$RS_ID" "$R_GIT"          "Create"  "create"  "Create a git secret"
create_action "$RS_ID" "$R_GIT"          "Read"    "read"    "List git secrets"
create_action "$RS_ID" "$R_GIT"          "Delete"  "delete"  "Delete a git secret"

create_action "$RS_ID" "$R_LLMT"         "Create"  "create"  "Create a provider template"
create_action "$RS_ID" "$R_LLMT"         "Read"    "read"    "View provider templates"
create_action "$RS_ID" "$R_LLMT"         "Update"  "update"  "Update a provider template"
create_action "$RS_ID" "$R_LLMT"         "Delete"  "delete"  "Delete a provider template"

create_action "$RS_ID" "$R_LLM"          "Create"              "create"              "Create an LLM provider"
create_action "$RS_ID" "$R_LLM"          "Read"                "read"                "View LLM providers and deployments"
create_action "$RS_ID" "$R_LLM"          "Update"              "update"              "Update an LLM provider"
create_action "$RS_ID" "$R_LLM"          "Delete"              "delete"              "Delete an LLM provider"
create_action "$RS_ID" "$R_LLM"          "Configure Guardrail" "configure-guardrail" "Configure guardrails, rate limits, and budgets"
create_action "$RS_ID" "$R_LLM"          "Connect"             "connect"             "Connect an agent to an LLM provider"
create_action "$RS_ID" "$R_LLM"          "Deploy"              "deploy"              "Deploy, undeploy, and restore an LLM provider"
create_action "$RS_ID" "$R_LLM_KEY"      "Manage"              "manage"              "Create, update, and delete LLM provider API keys"

create_action "$RS_ID" "$R_MCP"          "Create"              "create"              "Create an MCP server"
create_action "$RS_ID" "$R_MCP"          "Read"                "read"                "View MCP servers"
create_action "$RS_ID" "$R_MCP"          "Update"              "update"              "Update an MCP server"
create_action "$RS_ID" "$R_MCP"          "Delete"              "delete"              "Delete an MCP server"
create_action "$RS_ID" "$R_MCP"          "Configure Guardrail" "configure-guardrail" "Configure guardrails on an MCP server"
create_action "$RS_ID" "$R_MCP"          "Connect"             "connect"             "Connect an agent to an MCP server"

create_action "$RS_ID" "$R_PROXY"        "Create"  "create"  "Create an LLM proxy"
create_action "$RS_ID" "$R_PROXY"        "Read"    "read"    "View LLM proxies and deployments"
create_action "$RS_ID" "$R_PROXY"        "Update"  "update"  "Update an LLM proxy"
create_action "$RS_ID" "$R_PROXY"        "Delete"  "delete"  "Delete an LLM proxy"
create_action "$RS_ID" "$R_PROXY"        "Deploy"  "deploy"  "Deploy, undeploy, and restore an LLM proxy"
create_action "$RS_ID" "$R_PROXY_KEY"    "Manage"  "manage"  "Create, update, and delete LLM proxy API keys"

create_action "$RS_ID" "$R_EVAL"         "Create"  "create"  "Create a custom evaluator"
create_action "$RS_ID" "$R_EVAL"         "Read"    "read"    "View evaluators"
create_action "$RS_ID" "$R_EVAL"         "Update"  "update"  "Update a custom evaluator"
create_action "$RS_ID" "$R_EVAL"         "Delete"  "delete"  "Delete a custom evaluator"

create_action "$RS_ID" "$R_AGENT"        "Create"   "create"   "Create an agent"
create_action "$RS_ID" "$R_AGENT"        "Read"     "read"     "View agent details, builds, deployments, and configs"
create_action "$RS_ID" "$R_AGENT"        "Update"   "update"   "Update agent configuration and resource configs"
create_action "$RS_ID" "$R_AGENT"        "Delete"   "delete"   "Delete an agent"
create_action "$RS_ID" "$R_AGENT"        "Build"    "build"    "Trigger an agent build"
create_action "$RS_ID" "$R_AGENT"        "Promote"  "promote"  "Promote an agent deployment across environments"
create_action "$RS_ID" "$R_AGENT"        "Rollback" "rollback" "Rollback an agent deployment"
create_action "$RS_ID" "$R_AGENT"        "Suspend"  "suspend"  "Suspend or stop an agent deployment"
create_action "$RS_ID" "$R_AGENT_DEPLOY" "Non-Production" "non-production" "Deploy an agent to a non-production environment"
create_action "$RS_ID" "$R_AGENT_DEPLOY" "Production"     "production"     "Deploy an agent to a production environment"
create_action "$RS_ID" "$R_AGENT_TOKEN"  "Manage"         "manage"         "Generate agent tokens"

create_action "$RS_ID" "$R_MON"          "Create"   "create"   "Create a monitor"
create_action "$RS_ID" "$R_MON"          "Read"     "read"     "View monitors, runs, and run logs"
create_action "$RS_ID" "$R_MON"          "Update"   "update"   "Update a monitor"
create_action "$RS_ID" "$R_MON"          "Delete"   "delete"   "Delete a monitor"
create_action "$RS_ID" "$R_MON"          "Execute"  "execute"  "Start, stop, and rerun monitors"
create_action "$RS_ID" "$R_MON_SCORE"    "Read"     "read"     "View scores, breakdowns, and timeseries"
create_action "$RS_ID" "$R_MON_SCORE"    "Publish"  "publish"  "Publish monitor scores (internal/system use)"

create_action "$RS_ID" "$R_OBS"          "Org Dashboard"     "org-dashboard"     "Access the organization-level observability dashboard"
create_action "$RS_ID" "$R_OBS"          "Project Dashboard" "project-dashboard" "Access the project-level observability dashboard"
create_action "$RS_ID" "$R_OBS"          "Guardrail Metric"  "guardrail-metric"  "View guardrail metrics"
create_action "$RS_ID" "$R_OBS"          "Infra Metric"      "infra-metric"      "View CPU, memory, and infrastructure metrics"

create_action "$RS_ID" "$R_ROLE"         "Create"  "create"  "Create a custom role"
create_action "$RS_ID" "$R_ROLE"         "Read"    "read"    "View roles and their permissions"
create_action "$RS_ID" "$R_ROLE"         "Update"  "update"  "Update a custom role"
create_action "$RS_ID" "$R_ROLE"         "Delete"  "delete"  "Delete a custom role"
create_action "$RS_ID" "$R_GROUP"        "Create"  "create"  "Create a group"
create_action "$RS_ID" "$R_GROUP"        "Read"    "read"    "View groups and their members"
create_action "$RS_ID" "$R_GROUP"        "Update"  "update"  "Update a group"
create_action "$RS_ID" "$R_GROUP"        "Delete"  "delete"  "Delete a group"

create_action "$RS_ID" "$R_CAT"          "Read"    "read"    "View the resource catalog"
create_action "$RS_ID" "$R_REPO"         "Read"    "read"    "Browse repository branches and commits"

log_success "Agent Manager resource server registration complete (84 permissions registered)."
