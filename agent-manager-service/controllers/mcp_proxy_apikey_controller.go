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
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyAPIKeyController defines handlers for source MCP proxy API key operations.
type MCPProxyAPIKeyController interface {
	ListAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
	RotateAPIKey(w http.ResponseWriter, r *http.Request)
}

type mcpProxyAPIKeyController struct {
	apiKeyService *services.MCPProxyAPIKeyService
}

// NewMCPProxyAPIKeyController creates a new MCP proxy API key controller.
func NewMCPProxyAPIKeyController(apiKeyService *services.MCPProxyAPIKeyService) MCPProxyAPIKeyController {
	return &mcpProxyAPIKeyController{
		apiKeyService: apiKeyService,
	}
}

// ListAPIKeys handles GET /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys.
func (c *mcpProxyAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	response, err := c.apiKeyService.ListAPIKeys(ctx, orgName, proxyID)
	if err != nil {
		c.writeAPIKeyError(w, log, "ListMCPProxyAPIKeys", orgName, proxyID, "", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateAPIKey handles POST /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys.
func (c *mcpProxyAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
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

	response, err := c.apiKeyService.CreateAPIKey(ctx, orgName, proxyID, &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeAPIKeyError(w, log, "CreateMCPProxyAPIKey", orgName, proxyID, "", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RevokeAPIKey handles DELETE /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys/{keyName}.
func (c *mcpProxyAPIKeyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)
	keyName := r.PathValue("keyName")

	if err := c.apiKeyService.RevokeAPIKey(ctx, orgName, proxyID, keyName); err != nil {
		c.writeAPIKeyError(w, log, "RevokeMCPProxyAPIKey", orgName, proxyID, keyName, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RotateAPIKey handles PUT /orgs/{orgName}/mcp-proxies/{proxyId}/api-keys/{keyName}.
func (c *mcpProxyAPIKeyController) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue(utils.PathParamProxyId)
	keyName := r.PathValue("keyName")

	var specReq spec.RotateLLMAPIKeyRequest
	// Body is optional for rotation: an empty body (io.EOF) is allowed, but any
	// other decode error indicates a malformed request and must be rejected.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	response, err := c.apiKeyService.RotateAPIKey(ctx, orgName, proxyID, keyName, &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeAPIKeyError(w, log, "RotateMCPProxyAPIKey", orgName, proxyID, keyName, err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *mcpProxyAPIKeyController) writeAPIKeyError(w http.ResponseWriter, log *slog.Logger, operation, orgName, proxyID, keyName string, err error) {
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
