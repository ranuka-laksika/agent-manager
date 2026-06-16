// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyController defines handlers for MCP proxy operations.
type MCPProxyController interface {
	CreateMCPProxy(w http.ResponseWriter, r *http.Request)
	ListMCPProxies(w http.ResponseWriter, r *http.Request)
	ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request)
	GetMCPProxy(w http.ResponseWriter, r *http.Request)
	UpdateMCPProxy(w http.ResponseWriter, r *http.Request)
	DeleteMCPProxy(w http.ResponseWriter, r *http.Request)
	FetchServerInfo(w http.ResponseWriter, r *http.Request)
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
	RotateAPIKey(w http.ResponseWriter, r *http.Request)
}

type mcpProxyController struct {
	mcpProxyService           *services.MCPProxyService
	agentConfigurationService services.AgentConfigurationService
}

// NewMCPProxyController creates a new MCP proxy controller. agentConfigurationService is
// used to cascade mapping-artifact redeploys after a successful proxy update.
func NewMCPProxyController(mcpProxyService *services.MCPProxyService, agentConfigurationService services.AgentConfigurationService) MCPProxyController {
	return &mcpProxyController{
		mcpProxyService:           mcpProxyService,
		agentConfigurationService: agentConfigurationService,
	}
}

// CreateMCPProxy handles POST /orgs/{orgName}/mcp-proxies.
func (c *mcpProxyController) CreateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("CreateMCPProxy: starting", "orgName", orgName)

	var req models.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateMCPProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.mcpProxyService.Create(ctx, orgName, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("CreateMCPProxy: invalid request", "orgName", orgName, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrMCPProxyExists):
			log.Error("CreateMCPProxy: MCP proxy already exists", "orgName", orgName, "id", req.ID)
			utils.WriteErrorResponse(w, http.StatusConflict, "MCP proxy already exists")
		default:
			log.Error("CreateMCPProxy: failed", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create MCP proxy")
		}
		return
	}

	log.Info("CreateMCPProxy: completed", "orgName", orgName, "id", req.ID)
	utils.WriteSuccessResponse(w, http.StatusCreated, resp)
}

// ListMCPProxies handles GET /orgs/{orgName}/mcp-proxies.
func (c *mcpProxyController) ListMCPProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	limit := getMCPProxyIntQueryParam(r, "limit", utils.DefaultLimit)
	offset := getMCPProxyIntQueryParam(r, "offset", utils.DefaultOffset)

	if limit < utils.MinLimit || limit > utils.MaxLimit {
		limit = utils.DefaultLimit
	}
	if offset < utils.MinOffset {
		offset = utils.DefaultOffset
	}

	log.Info("ListMCPProxies: starting", "orgName", orgName, "limit", limit, "offset", offset)

	resp, err := c.mcpProxyService.List(ctx, orgName, limit, offset)
	if err != nil {
		log.Error("ListMCPProxies: failed", "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxies")
		return
	}

	log.Info("ListMCPProxies: completed", "orgName", orgName, "count", resp.Count)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ListAvailableMCPPolicies handles GET /orgs/{orgName}/mcp-proxies/policies.
func (c *mcpProxyController) ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("ListAvailableMCPPolicies: starting", "orgName", orgName)

	resp, err := c.mcpProxyService.ListAvailableMCPPolicies(ctx, orgName)
	if err != nil {
		log.Error("ListAvailableMCPPolicies: failed", "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list available MCP policies")
		return
	}

	log.Info("ListAvailableMCPPolicies: completed", "orgName", orgName, "count", resp.Count)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// GetMCPProxy handles GET /orgs/{orgName}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) GetMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("GetMCPProxy: starting", "orgName", orgName, "proxyID", proxyID)

	resp, err := c.mcpProxyService.Get(ctx, orgName, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("GetMCPProxy: MCP proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("GetMCPProxy: invalid request", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("GetMCPProxy: failed", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get MCP proxy")
		}
		return
	}

	log.Info("GetMCPProxy: completed", "orgName", orgName, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// UpdateMCPProxy handles PUT /orgs/{orgName}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) UpdateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("UpdateMCPProxy: starting", "orgName", orgName, "proxyID", proxyID)

	var req models.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateMCPProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, source, err := c.mcpProxyService.Update(ctx, orgName, proxyID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("UpdateMCPProxy: MCP proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("UpdateMCPProxy: invalid request", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		default:
			log.Error("UpdateMCPProxy: failed", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update MCP proxy")
		}
		return
	}

	// Cascade: refresh every agent-scoped MCP mapping artifact derived from this proxy
	// so they pick up the new upstream / policies. Best-effort — log and continue on
	// failure, matching the prior hook semantics.
	if err := c.agentConfigurationService.RedeployMCPMappingsForSourceProxy(ctx, source, orgName); err != nil {
		log.Warn("UpdateMCPProxy: failed to redeploy MCP mapping artifacts", "orgName", orgName, "proxyID", proxyID, "error", err)
	}

	log.Info("UpdateMCPProxy: completed", "orgName", orgName, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// DeleteMCPProxy handles DELETE /orgs/{orgName}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) DeleteMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("DeleteMCPProxy: starting", "orgName", orgName, "proxyID", proxyID)

	if err := c.mcpProxyService.Delete(ctx, orgName, proxyID); err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("DeleteMCPProxy: MCP proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrMCPProxyHasMappings):
			log.Warn("DeleteMCPProxy: MCP proxy has mappings", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrMCPProxyHasMappings.Error())
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeleteMCPProxy: invalid request", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("DeleteMCPProxy: failed", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy")
		}
		return
	}

	log.Info("DeleteMCPProxy: completed", "orgName", orgName, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// CreateAPIKey handles POST /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys.
func (c *mcpProxyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	var specReq spec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateMCPProxyAPIKey: failed to decode request", "orgName", orgName, "proxyID", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	name := ""
	if specReq.Name != nil {
		name = *specReq.Name
	}
	displayName := ""
	if specReq.DisplayName != nil {
		displayName = *specReq.DisplayName
	}
	if name == "" && displayName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return
	}

	response, err := c.mcpProxyService.CreateAPIKey(ctx, orgName, proxyID, &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeMCPProxyAPIKeyError(w, log, "CreateMCPProxyAPIKey", orgName, proxyID, "", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RevokeAPIKey handles DELETE /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys/{keyName}.
func (c *mcpProxyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)
	keyName := r.PathValue("keyName")

	if err := c.mcpProxyService.RevokeAPIKey(ctx, orgName, proxyID, keyName); err != nil {
		c.writeMCPProxyAPIKeyError(w, log, "RevokeMCPProxyAPIKey", orgName, proxyID, keyName, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RotateAPIKey handles PUT /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys/{keyName}.
func (c *mcpProxyController) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)
	keyName := r.PathValue("keyName")

	var specReq spec.RotateLLMAPIKeyRequest
	_ = json.NewDecoder(r.Body).Decode(&specReq)

	response, err := c.mcpProxyService.RotateAPIKey(ctx, orgName, proxyID, keyName, &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeMCPProxyAPIKeyError(w, log, "RotateMCPProxyAPIKey", orgName, proxyID, keyName, err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *mcpProxyController) writeMCPProxyAPIKeyError(w http.ResponseWriter, log *slog.Logger, operation, orgName, proxyID, keyName string, err error) {
	switch {
	case errors.Is(err, utils.ErrMCPProxyNotFound):
		log.Warn(operation+": MCP proxy not found", "orgName", orgName, "proxyID", proxyID, "keyName", keyName)
		utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
	case errors.Is(err, utils.ErrInvalidInput):
		log.Error(operation+": invalid request", "orgName", orgName, "proxyID", proxyID, "keyName", keyName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request")
	case errors.Is(err, utils.ErrGatewayNotFound):
		log.Error(operation+": no gateways found", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
	default:
		log.Error(operation+": failed", "orgName", orgName, "proxyID", proxyID, "keyName", keyName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to manage API key")
	}
}

// FetchServerInfo handles POST /orgs/{orgName}/mcp-proxies/fetch-server-info.
func (c *mcpProxyController) FetchServerInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("FetchMCPProxyServerInfo: starting", "orgName", orgName)

	var req models.MCPServerInfoFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("FetchMCPProxyServerInfo: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.mcpProxyService.FetchServerInfo(ctx, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("FetchMCPProxyServerInfo: invalid request", "orgName", orgName, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrURLUnreachable):
			log.Error("FetchMCPProxyServerInfo: MCP server URL is unreachable", "orgName", orgName, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "MCP server URL is unreachable", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrMCPServerUnauthorized):
			log.Error("FetchMCPProxyServerInfo: MCP server returned unauthorized", "orgName", orgName, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusUnauthorized, "MCP server returned 401 Unauthorized. Check the provided credentials.", err.Error(), utils.ErrCodeUnauthorized)
		default:
			log.Error("FetchMCPProxyServerInfo: failed", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to fetch MCP server info")
		}
		return
	}

	log.Info("FetchMCPProxyServerInfo: completed", "orgName", orgName)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func getMCPProxyIntQueryParam(r *http.Request, key string, defaultValue int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
