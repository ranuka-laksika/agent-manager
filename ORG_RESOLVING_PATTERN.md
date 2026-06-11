# Org Resolving Logic - Implementation Pattern

## Overview
This refactoring adds validation to ensure that org names in HTTP path parameters match the caller's organization identity from their JWT token. This prevents org-switching/path-traversal attacks.

## Pattern

### 1. Add middleware import
```go
import (
	"context"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
)
```

### 2. Add validation helper function (in each controller file)
```go
// validateOrgFromPath validates that the org in the path matches the caller's token org.
func validateOrgFromPath(w http.ResponseWriter, ctx context.Context, pathOrg string) bool {
	resolvedOrg, ok := middleware.GetResolvedOrg(ctx)
	if !ok {
		utils.WriteErrorResponse(w, http.StatusForbidden, "missing org context")
		return false
	}
	if pathOrg != resolvedOrg.OuHandle {
		utils.WriteErrorResponse(w, http.StatusForbidden, "org mismatch with token identity")
		return false
	}
	return true
}
```

### 3. In each handler that extracts orgName from path:
```go
// Before: Extract and use directly
func (c *controller) Handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue(utils.PathParamOrgName)
	// ... use orgName directly
}

// After: Extract, validate, then use
func (c *controller) Handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue(utils.PathParamOrgName)
	
	if !validateOrgFromPath(w, ctx, orgName) {
		return
	}
	// ... now safe to use orgName
}
```

## Completed
- ✅ agent_token_controller.go
- ✅ agent_controller.go (with helper function)

## Remaining Controllers

Apply the pattern to these 15 controllers:

1. agent_apikey_controller.go - 5 functions
2. agent_configuration_controller.go - 5 functions  
3. agent_kind_controller.go - 9 functions
4. evaluator_controller.go - 6 functions
5. catalog_controller.go - 1 function
6. environment_controller.go - 6 functions
7. git_secret_controller.go - 3 functions
8. gateway_controller.go - 12 functions
9. llm_controller.go - 17 functions
10. infra_resource_controller.go - 10 functions
11. llm_proxy_deployment_controller.go - 6 functions
12. llm_deployment_controller.go - 6 functions
13. llm_provider_apikey_controller.go - 3 functions
14. llm_proxy_apikey_controller.go - 3 functions
15. monitor_controller.go - 10 functions
16. monitor_scores_controller.go - 6 functions

## Example: Full Update for One Controller

For each controller:

1. Add imports at top:
```go
import (
	"context"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	// ... existing imports
)
```

2. Add the validation helper function after the imports/before the first handler method

3. In each handler that extracts `orgName := r.PathValue(utils.PathParamOrgName)`:
   - Add validation call right after extracting orgName
   - Return on validation failure

Example for `ListAPIKeys` in `agent_apikey_controller.go`:
```go
func (c *agentAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)

	if !validateOrgFromPath(w, ctx, orgName) {  // ← ADD THIS
		return                                      // ← ADD THIS
	}

	log.Info("ListAgentAPIKeys: starting", "orgName", orgName, "projName", projName, "agentName", agentName, "envID", envID)
	// ... rest of function unchanged
}
```

## Notes
- The validation ensures path parameter matches JWT token org
- No changes needed to service layers - controllers still pass orgName as before
- No changes needed to routes - middleware still applies org resolution
- This is purely a security validation layer at the controller level
- Prevents accidental or malicious org-switching attempts
