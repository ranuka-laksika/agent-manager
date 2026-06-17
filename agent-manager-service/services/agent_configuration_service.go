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

package services

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentConfigurationService interface defines agent configuration business logic
type AgentConfigurationService interface {
	Create(ctx context.Context, orgName, projectName, agentID string,
		req models.CreateAgentModelConfigRequest, createdBy string) (*models.AgentModelConfigResponse, error)
	ValidateProvidersInCatalog(ctx context.Context, orgName string, providerHandles []string) error
	ValidateMCPProxiesInCatalog(ctx context.Context, orgName string, proxyHandles []string) error
	Get(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error)
	GetMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error)
	GetByAgent(ctx context.Context, agentID, orgName string) (*models.AgentModelConfigResponse, error)
	List(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	ListByType(ctx context.Context, orgName, projectName, agentName string, typeID uint, limit, offset int) (*models.AgentModelConfigListResponse, error)
	ListMCP(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	Update(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	UpdateMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	Delete(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error
	DeleteMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error
	// DeleteForAgentDeletion removes all external proxy resources for a single LLM config during
	// agent deletion. It skips OC Component/Workload/ReleaseBinding env-var patching and
	// SecretReference CR deletion because the component itself is being torn down. isExternalAgent
	// must be resolved once by the caller to avoid a GetComponent call per config.
	DeleteForAgentDeletion(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string, isExternalAgent bool) error
	// ListAgentLLMConfigSecretReferences returns the set of SecretReference names persisted in the
	// DB for all LLM configurations of this agent in the given environment. Used during deploy to
	// identify which component env var secretRefs are system-managed (LLM config) vs user-provided.
	ListAgentLLMConfigSecretReferences(ctx context.Context, agentID, orgName, environmentName string) (map[string]struct{}, error)
	// ListSystemManagedEnvVarKeys returns the set of env var keys that are system-managed
	// (i.e. injected by LLM configurations) for the given agent and environment.
	// Used during promote to strip these keys from inherited workload overrides.
	ListSystemManagedEnvVarKeys(ctx context.Context, agentID, orgName, environmentName string) (map[string]bool, error)
	// BuildSystemManagedEnvVarsFromConfig constructs the LLM env vars for a given agent
	// and environment from the DB config. Used during promotion when the target environment's
	// ReleaseBinding doesn't have these vars yet.
	BuildSystemManagedEnvVarsFromConfig(ctx context.Context, agentID, orgName, environmentName string) ([]client.EnvVar, error)
	// RedeployMCPMappingsForSourceProxy refreshes every agent-scoped MCP mapping artifact derived
	// from the given source proxy. Called by the MCP proxy controller after a successful proxy
	// update so each derived artifact picks up new upstream URL / policies on its gateways.
	RedeployMCPMappingsForSourceProxy(ctx context.Context, source *models.MCPProxy, orgName string) error
}

type EnvConfigTemplate struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	IsSecret        bool   `json:"isSecret"`
	Value           string `json:"value"`
	SecretReference string `json:"secretReference"`
}

type agentConfigurationService struct {
	db                        *gorm.DB
	agentConfigRepo           repositories.AgentConfigurationRepository
	envMappingRepo            repositories.EnvAgentModelMappingRepository
	envMCPMappingRepo         repositories.EnvAgentMCPMappingRepository
	envVariableRepo           repositories.AgentEnvConfigVariableRepository
	llmProviderRepo           repositories.LLMProviderRepository
	mcpProxyRepo              repositories.MCPProxyRepository
	gatewayRepo               repositories.GatewayRepository
	llmProxyService           *LLMProxyService
	mcpProxyService           *MCPProxyService
	llmProxyDeploymentService *LLMProxyDeploymentService
	llmProxyAPIKeyService     *LLMProxyAPIKeyService
	llmProviderAPIKeyService  *LLMProviderAPIKeyService
	aiApplicationService      *AIApplicationService
	infraResourceManager      InfraResourceManager
	ocClient                  client.OpenChoreoClient
	logger                    *slog.Logger
	secretClient              secretmanagersvc.SecretManagementClient
	encryptionKey             []byte
}

// rollbackResource tracks a proxy, its deployment, and API keys for cleanup
type rollbackResource struct {
	proxyHandle       string
	deploymentID      uuid.UUID
	proxyAPIKeyID     string                           // API key created for the proxy
	providerAPIKeyID  string                           // API key name created for the provider
	providerUUID      string                           // UUID of the provider (needed to revoke the provider API key)
	mappingID         uint                             // ID of the env mapping to revert (HIGH-4, Scenario A only)
	oldProxyUUID      uuid.UUID                        // old proxy UUID to restore in the mapping on rollback (HIGH-4, Scenario A only)
	providerSecretLoc *secretmanagersvc.SecretLocation // Location for provider API key secret
	proxySecretLoc    *secretmanagersvc.SecretLocation // Location for proxy API key secret
	secretRefName     string                           // Name of the SecretReference CR to delete on rollback (internal agents only)
	// AI application rollback fields — only set when EnsureAndBind created a new app.
	createdNewApp  bool
	appAgentID     string
	appProjectName string
	appEnvName     string
}

// nonK8sNameChar matches any character not valid in a Kubernetes resource name segment.
var nonK8sNameChar = regexp.MustCompile(`[^a-z0-9-]`)

// multiHyphenRe matches two or more consecutive hyphens.
var multiHyphenRe = regexp.MustCompile(`-{2,}`)

// sanitizeForK8sName converts a string to a valid Kubernetes resource name segment.
// It lowercases the input, replaces spaces and underscores with hyphens, strips
// remaining invalid characters, collapses consecutive hyphens, trims leading/trailing
// hyphens, and caps the result at 63 characters.
func sanitizeForK8sName(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	s = nonK8sNameChar.ReplaceAllString(s, "")
	s = multiHyphenRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

const proxyNamePrefixMaxLen = 10

// agentAppIdentifier builds a stable, collision-resistant handle for the per-agent-per-env
// AIApplication. Format: "<agentPrefix>-<16-hex-chars>".
func agentAppIdentifier(projectName, agentID, envName string) string {
	raw := fmt.Sprintf("%s/%s/%s", projectName, agentID, envName)
	hash := sha256.Sum256([]byte(raw))
	hashSuffix := hex.EncodeToString(hash[:8])
	prefix := sanitizeForK8sName(agentID)
	if len(prefix) > proxyNamePrefixMaxLen {
		prefix = prefix[:proxyNamePrefixMaxLen]
	}
	return fmt.Sprintf("%s-%s", prefix, hashSuffix)
}

// scopedProxyIdentifier builds a deterministic, collision-resistant identifier
// from the config name and a hash of all scoping segments (project, agent, config, env).
// Format: "<configPrefix>-<16-hex-chars>" where configPrefix is the first 10 chars
// of the sanitized config name.
func scopedProxyIdentifier(projectName, agentName, configName, envName string) string {
	raw := fmt.Sprintf("%s/%s/%s/%s", projectName, agentName, configName, envName)
	hash := sha256.Sum256([]byte(raw))
	hashSuffix := hex.EncodeToString(hash[:8])

	prefix := sanitizeForK8sName(configName)
	if len(prefix) > proxyNamePrefixMaxLen {
		prefix = strings.TrimRight(prefix[:proxyNamePrefixMaxLen], "-")
	}
	return fmt.Sprintf("%s-%s", prefix, hashSuffix)
}

// mcpMappingProxyName builds the artifact handle/name and deployed proxy name for an
// agent-scoped MCP proxy mapping. It mirrors the LLM proxy naming scheme exactly:
// "<scopedID>-proxy", where scopedID is scopedProxyIdentifier(project, agent, config, env).
// Handle and name are identical, matching how LLM proxies derive both from the same value.
func mcpMappingProxyName(projectName, agentID, configName, envName string) string {
	return fmt.Sprintf("%s-proxy", scopedProxyIdentifier(projectName, agentID, configName, envName))
}

// buildProxyURL constructs the proxy base URL from a gateway and an optional context path.
// For internal agents, it uses the in-cluster gateway runtime service name (ClusterIP)
// so pods can reach the gateway without depending on external DNS.
// For external agents, it uses the gateway's vhost (reachable from outside the cluster).
func buildProxyURL(gateway *models.Gateway, contextPath *string, isInternal bool) string {
	base := gateway.Vhost
	if isInternal {
		gwRuntime := config.GetConfig().GatewayRuntime
		base = fmt.Sprintf("http://%s%s:%d", gateway.Name, gwRuntime.HostSuffix, gwRuntime.Port)
	}
	if contextPath != nil {
		return fmt.Sprintf("%s%s", base, *contextPath)
	}
	return base
}

// buildLLMEnvVars constructs the two env vars (URL and API key) from the env config templates.
func buildLLMEnvVars(templates []EnvConfigTemplate, proxyURL, secretRefName string) []client.EnvVar {
	var urlTemplate, apiKeyTemplate EnvConfigTemplate
	for _, t := range templates {
		switch t.Key {
		case "url":
			urlTemplate = t
		case "apikey":
			apiKeyTemplate = t
		}
	}
	return []client.EnvVar{
		{Key: urlTemplate.Name, Value: proxyURL},
		{
			Key: apiKeyTemplate.Name,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: secretRefName,
					Key:  secretmanagersvc.SecretKeyAPIKey,
				},
			},
		},
	}
}

func buildMCPEnvVars(templates []EnvConfigTemplate, proxyURL, secretRefName string) []client.EnvVar {
	var envVars []client.EnvVar
	for _, t := range templates {
		switch t.Key {
		case "url":
			envVars = append(envVars, client.EnvVar{Key: t.Name, Value: proxyURL})
		case "apikey":
			if secretRefName != "" {
				envVars = append(envVars, client.EnvVar{
					Key: t.Name,
					ValueFrom: &client.EnvVarValueFrom{
						SecretKeyRef: &client.SecretKeyRef{
							Name: secretRefName,
							Key:  secretmanagersvc.SecretKeyAPIKey,
						},
					},
				})
			}
		}
	}
	return envVars
}

func buildMCPProxyURL(vhost string, contextPath *string) string {
	base := strings.TrimRight(strings.TrimSpace(vhost), "/")
	path := "/mcp"
	if contextPath != nil && strings.TrimSpace(*contextPath) != "" {
		path = strings.TrimRight(*contextPath, "/") + "/mcp"
	}
	return base + path
}

// mcpProxyAPIKeySecurityEnabled reports whether the source MCP proxy requires API
// key security. When it returns false, agent mappings derived from the proxy are
// deployed without minting a gateway key, binding an app key, or injecting the
// apikey env var — mirroring how an LLM provider with security disabled yields
// proxies with no API key wired in the gateway.
func mcpProxyAPIKeySecurityEnabled(proxy *models.MCPProxy) bool {
	if proxy == nil {
		return false
	}
	security := proxy.Configuration.Security
	if security == nil || !isBoolTrue(security.Enabled) {
		return false
	}
	return security.APIKey != nil && isBoolTrue(security.APIKey.Enabled)
}

func mcpProxyAPIKeyHeaderName(proxy *models.MCPProxy) string {
	if proxy == nil || proxy.Configuration.Security == nil || proxy.Configuration.Security.APIKey == nil {
		return "X-API-Key"
	}
	header := strings.TrimSpace(proxy.Configuration.Security.APIKey.Key)
	if header == "" {
		return "X-API-Key"
	}
	return header
}

func (s *agentConfigurationService) createMCPMappingAPIKey(ctx context.Context, orgName string, mappingUUID uuid.UUID, keyName string) (*models.CreateAPIKeyResponse, error) {
	if s.llmProxyAPIKeyService == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	mappingID := mappingUUID.String()
	return s.llmProxyAPIKeyService.broadcaster.broadcastCreate(orgName, mappingID, mappingID, &models.CreateAPIKeyRequest{
		Name: keyName,
	})
}

func (s *agentConfigurationService) revokeMCPMappingAPIKey(ctx context.Context, orgName string, mappingUUID uuid.UUID, keyName string) error {
	if s.llmProxyAPIKeyService == nil {
		return fmt.Errorf("API key service is not configured")
	}
	mappingID := mappingUUID.String()
	return s.llmProxyAPIKeyService.broadcaster.broadcastRevoke(orgName, mappingID, mappingID, keyName)
}

func mcpMappingScopedID(config *models.AgentConfiguration, envName string) string {
	return scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName)
}

func mcpMappingAPIKeyName(config *models.AgentConfiguration, envName string) string {
	return fmt.Sprintf("%s-key", mcpMappingScopedID(config, envName))
}

func mcpMappingSecretLocation(config *models.AgentConfiguration, orgName, envName string) secretmanagersvc.SecretLocation {
	scopedID := mcpMappingScopedID(config, envName)
	return secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: envName,
		ConfigName:      config.Name,
		EntityName:      fmt.Sprintf("%s-proxy", scopedID),
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
}

func (s *agentConfigurationService) mcpMappingAPIKeyExists(mappingUUID uuid.UUID, keyName string) (bool, error) {
	if s.llmProxyAPIKeyService == nil || s.llmProxyAPIKeyService.broadcaster.apiKeyRepo == nil {
		return false, nil
	}
	_, err := s.llmProxyAPIKeyService.broadcaster.apiKeyRepo.GetByArtifactAndName(mappingUUID.String(), keyName)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func (s *agentConfigurationService) revokeStaleMCPMappingAPIKeys(ctx context.Context, orgName string, mappingUUID uuid.UUID, keepName string) error {
	if s.llmProxyAPIKeyService == nil || s.llmProxyAPIKeyService.broadcaster.apiKeyRepo == nil {
		return nil
	}
	keys, err := s.llmProxyAPIKeyService.broadcaster.apiKeyRepo.ListByArtifact(mappingUUID.String())
	if err != nil {
		return err
	}
	var errs []error
	for _, key := range keys {
		if key.Name == keepName {
			continue
		}
		if err := s.revokeMCPMappingAPIKey(ctx, orgName, mappingUUID, key.Name); err != nil {
			errs = append(errs, fmt.Errorf("key %s: %w", key.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *agentConfigurationService) revokeAllMCPMappingAPIKeys(ctx context.Context, orgName string, mappingUUID uuid.UUID) error {
	return s.revokeStaleMCPMappingAPIKeys(ctx, orgName, mappingUUID, "")
}

// envCredentialData tracks proxy credentials for external agents
type envCredentialData struct {
	apiKey   string
	proxyURL string
}

// NewAgentConfigurationService creates a new agent configuration service
func NewAgentConfigurationService(
	db *gorm.DB,
	agentConfigRepo repositories.AgentConfigurationRepository,
	envMappingRepo repositories.EnvAgentModelMappingRepository,
	envMCPMappingRepo repositories.EnvAgentMCPMappingRepository,
	envVariableRepo repositories.AgentEnvConfigVariableRepository,
	llmProviderRepo repositories.LLMProviderRepository,
	mcpProxyRepo repositories.MCPProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	llmProxyService *LLMProxyService,
	mcpProxyService *MCPProxyService,
	llmProxyDeploymentService *LLMProxyDeploymentService,
	llmProxyAPIKeyService *LLMProxyAPIKeyService,
	infraResourceManager InfraResourceManager,
	ocClient client.OpenChoreoClient,
	llmProviderAPIKeyService *LLMProviderAPIKeyService,
	aiApplicationService *AIApplicationService,
	logger *slog.Logger,
	secretClient secretmanagersvc.SecretManagementClient,
	encryptionKey []byte,
) AgentConfigurationService {
	svc := &agentConfigurationService{
		db:                        db,
		agentConfigRepo:           agentConfigRepo,
		envMappingRepo:            envMappingRepo,
		envMCPMappingRepo:         envMCPMappingRepo,
		envVariableRepo:           envVariableRepo,
		llmProviderRepo:           llmProviderRepo,
		mcpProxyRepo:              mcpProxyRepo,
		gatewayRepo:               gatewayRepo,
		llmProxyService:           llmProxyService,
		mcpProxyService:           mcpProxyService,
		llmProxyDeploymentService: llmProxyDeploymentService,
		llmProxyAPIKeyService:     llmProxyAPIKeyService,
		aiApplicationService:      aiApplicationService,
		infraResourceManager:      infraResourceManager,
		ocClient:                  ocClient,
		llmProviderAPIKeyService:  llmProviderAPIKeyService,
		logger:                    logger,
		secretClient:              secretClient,
		encryptionKey:             encryptionKey,
	}
	// Register the deletion reconciler now that this service exists; MCPProxyService is
	// constructed first and calls back into here when a proxy is deleted to strip the
	// proxy's injected env vars from dependent agents.
	return svc
}

// compensatingDeleteConfig performs a best-effort DELETE of the config row committed in Phase 1,
// when a later phase fails. CASCADE on EnvMappings/EnvVariables removes any partially-written rows.
func (s *agentConfigurationService) compensatingDeleteConfig(ctx context.Context, configUUID uuid.UUID, orgName string) {
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName)
	}); err != nil {
		s.logger.Error("CRITICAL: Failed to compensate config creation - orphaned config record",
			"configUUID", configUUID, "orgName", orgName, "error", err, "action", "manual cleanup required")
	} else {
		s.logger.Info("Compensating delete of config record succeeded", "configUUID", configUUID)
	}
}

// ValidateProvidersInCatalog verifies each handle resolves to an existing provider
// that is in catalog. Returns ErrLLMProviderNotFound (missing) or ErrInvalidInput
// (empty handle / not in catalog). Handles are deduped.
func (s *agentConfigurationService) ValidateProvidersInCatalog(
	_ context.Context, orgName string, providerHandles []string,
) error {
	seen := make(map[string]struct{}, len(providerHandles))
	for _, handle := range providerHandles {
		if handle == "" {
			return fmt.Errorf("%w: provider name is required", utils.ErrInvalidInput)
		}
		if _, dup := seen[handle]; dup {
			continue
		}
		seen[handle] = struct{}{}

		provider, err := s.llmProviderRepo.GetByHandle(handle, orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("provider %s not found: %w", handle, utils.ErrLLMProviderNotFound)
			}
			return fmt.Errorf("failed to validate provider %s: %w", handle, err)
		}
		if !provider.InCatalog {
			return fmt.Errorf("%w: provider %s must be in catalog", utils.ErrInvalidInput, handle)
		}
	}
	return nil
}

// ValidateMCPProxiesInCatalog verifies each handle resolves to an existing MCP proxy
// that is published in the catalog. Mirrors ValidateProvidersInCatalog and is used by the
// MCP auto-wiring preflight so a bad proxy fails fast before the component is created.
func (s *agentConfigurationService) ValidateMCPProxiesInCatalog(
	ctx context.Context, orgName string, proxyHandles []string,
) error {
	if s.mcpProxyRepo == nil {
		return fmt.Errorf("MCP configuration service is not fully configured")
	}
	seen := make(map[string]struct{}, len(proxyHandles))
	for _, handle := range proxyHandles {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			return fmt.Errorf("%w: MCP proxy name is required", utils.ErrInvalidInput)
		}
		if _, dup := seen[handle]; dup {
			continue
		}
		seen[handle] = struct{}{}

		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, handle, orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("MCP proxy %s not found: %w", handle, utils.ErrMCPProxyNotFound)
			}
			return fmt.Errorf("failed to validate MCP proxy %s: %w", handle, err)
		}
		if proxy.Artifact == nil || !proxy.Artifact.InCatalog {
			return fmt.Errorf("%w: MCP proxy %s must be in catalog", utils.ErrInvalidInput, handle)
		}
	}
	return nil
}

// Create creates a new agent model configuration
func (s *agentConfigurationService) Create(ctx context.Context, orgName, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string,
) (*models.AgentModelConfigResponse, error) {
	// Validate agent exists and determine type
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentID)
	if err != nil {
		// Check if it's a 404 error (agent not found) vs other errors
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		// For other errors (unauthorized, internal, etc), return as-is
		return nil, fmt.Errorf("failed to validate agent: %w", err)
	}

	// Determine if this is an external agent
	isExternalAgent := agent.Provisioning.Type == string(utils.ExternalAgent)

	switch req.Type {
	case models.AgentConfigTypeMCP:
		return s.createMCPConfig(ctx, orgName, projectName, agentID, req, createdBy, isExternalAgent)
	default:
		return s.createLLMConfig(ctx, orgName, projectName, agentID, req, createdBy, isExternalAgent)
	}
}

func (s *agentConfigurationService) createLLMConfig(ctx context.Context, orgName, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string, isExternalAgent bool,
) (*models.AgentModelConfigResponse, error) {
	// Validate that at least one environment mapping is provided (CRIT-5).
	// The binding:"required,min=1" tag on the DTO is ignored by net/http + json.NewDecoder,
	// so we enforce it explicitly here.
	if len(req.EnvMappings) == 0 {
		return nil, fmt.Errorf("%w: at least one environment mapping is required", utils.ErrInvalidInput)
	}

	// Fail fast: validate env var names before any I/O.
	// If the config name would generate a reserved env var prefix the error is returned here,
	// before any gateway/proxy/deployment resources have been created.
	// The returned slice is intentionally discarded; it is rebuilt at deployment time.
	if _, err := s.buildEnvironmentVariables(req.Name, req.EnvironmentVariables); err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}

	// Validate all providers exist and are in catalog (shared with the create-time preflight).
	handles := make([]string, 0, len(req.EnvMappings))
	for _, envMapping := range req.EnvMappings {
		handles = append(handles, envMapping.ProviderName)
	}
	if err := s.ValidateProvidersInCatalog(ctx, orgName, handles); err != nil {
		return nil, err
	}

	// Validate environment UUIDs exist
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.Name] = env
	}

	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	// Build config struct (UUID assigned on Create)
	config := &models.AgentConfiguration{
		Name:             req.Name,
		Description:      req.Description,
		AgentID:          agentID,
		TypeID:           models.AgentConfigTypeToID(req.Type),
		OrganizationName: orgName,
		ProjectName:      projectName,
	}

	// Phase 1 — Short TX: persist config row only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Create(ctx, tx, config); err != nil {
			if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
				return err
			}
			return fmt.Errorf("failed to create configuration: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		return nil, err
	}

	// Track created resources for rollback across all environments.
	var rollbackResources []rollbackResource

	// Track credentials for external agents.
	var envCredentials map[string]envCredentialData
	if isExternalAgent {
		envCredentials = make(map[string]envCredentialData)
	}

	// Resolve first/dev environment name for ReleaseBinding patch (internal agents only).
	firstEnvName := ""
	if !isExternalAgent {
		pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
		if pipelineErr != nil {
			s.logger.Warn("failed to get deployment pipeline; ReleaseBinding patch will be skipped", "err", pipelineErr)
		} else if pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	// Phase 2 — Loop over environments: external ops first, then short per-env TX.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		// Context cancellation check before each env.
		select {
		case <-ctx.Done():
			// Use a fresh context for cleanup so cancelled ctx doesn't prevent rollback (CRIT-2).
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			s.processRollBack(cleanupCtx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		// External ops — no transaction held.
		proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
		}

		// Resolve gateway where the provider is deployed (ensures proxy uses the same gateway)
		gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, orgName, envUUID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
		}
		// Track provider credentials immediately so they are cleaned up even if proxy creation fails.
		rollbackResources = append(rollbackResources, rollbackResource{
			providerAPIKeyID:  providerAPIKeyID,
			providerUUID:      providerUUID,
			providerSecretLoc: providerSecretLoc,
		})
		// Capture index immediately after append to avoid fragile len(slice)-1 indexing below.
		rbIdx := len(rollbackResources) - 1

		proxy, err := s.llmProxyService.Create(orgName, createdBy, proxyConfig)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
		}
		// Update the rollback entry with the proxy handle now that it was created.
		rollbackResources[rbIdx].proxyHandle = proxy.Handle

		scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
		deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
			Name:      fmt.Sprintf("%s-deployment", scopedID),
			Base:      "current",
			GatewayID: gateway.UUID.String(),
		}, orgName)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].deploymentID = deployment.DeploymentID

		proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
			Name: fmt.Sprintf("%s-key", scopedID),
		})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
		}
		s.logger.Info("Created proxy API key", "proxyHandle", proxy.Handle, "proxyKeyName", proxyAPIKey.KeyID, "name", fmt.Sprintf("%s-key", scopedID))
		rollbackResources[rbIdx].proxyAPIKeyID = proxyAPIKey.KeyID

		// Ensure one AI application exists per agent+env and bind the proxy API key.
		agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, env.Name)
		_, created, err := s.aiApplicationService.EnsureAndBind(
			ctx, orgName, config.ProjectName, config.AgentID, env.Name,
			agentAppHandle,
			fmt.Sprintf("%s Application", config.AgentID),
			proxyAPIKey.KeyID,
		)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to ensure AI application for environment %s: %w", envName, err)
		}
		if created {
			rollbackResources[rbIdx].createdNewApp = true
			rollbackResources[rbIdx].appAgentID = config.AgentID
			rollbackResources[rbIdx].appProjectName = config.ProjectName
			rollbackResources[rbIdx].appEnvName = env.Name
		}

		// Store proxy API key in OpenBao KV and create SecretReference
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         orgName,
			ProjectName:     projectName,
			AgentName:       agentID,
			EnvironmentName: env.Name,
			ConfigName:      config.Name,
			EntityName:      proxy.Handle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to store proxy API key in KV for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].proxySecretLoc = &proxySecretLoc
		rollbackResources[rbIdx].secretRefName = secretRefName

		// Build proxy URL with nil-safe context access.
		var proxyContext *string
		if proxy != nil {
			proxyContext = proxy.Configuration.Context
		}
		proxyURL := buildProxyURL(gateway, proxyContext, !isExternalAgent)

		// Capture credentials for external agents.
		if isExternalAgent {
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   proxyAPIKey.APIKey,
				proxyURL: proxyURL,
			}
		}

		// Build environment variables (pure computation, no I/O).
		envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, req.EnvironmentVariables)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
		}
		variables := []models.AgentEnvConfigVariable{}
		for _, envConfigTemplate := range envConfigTemplates {
			secretReference := ""
			if envConfigTemplate.IsSecret {
				secretReference = secretRefName
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envConfigTemplate.Name,
				VariableKey:     envConfigTemplate.Key,
				SecretReference: secretReference,
			})
		}

		// Short per-env TX: DB writes only.
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			mapping := &models.EnvAgentModelMapping{
				ConfigUUID:          config.UUID,
				EnvironmentUUID:     envUUID,
				LLMProxyUUID:        proxy.UUID,
				PolicyConfiguration: models.LLMPolicies(envMapping.Configuration.Policies),
			}
			if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
				return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
			}
			if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
				return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
			}
			return nil
		}); err != nil {
			// CASCADE on config row will clean up any mappings/variables written for earlier envs.
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, err
		}

		// Internal-agent only: inject per-env vars into ReleaseBinding.
		// SecretReference is already created by secretClient.CreateSecret above.
		// The Component CR (global, shared across envs) is updated once after the loop using the
		// first-environment's vars to avoid last-write-wins clobbering (HIGH-3).
		if !isExternalAgent {
			// Build the two env vars (URL plain, API key via secretKeyRef).
			envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)

			// Step 3: Inject per-environment URL and API key ref into the ReleaseBinding.
			// Each environment gets its own ReleaseBinding with the correct per-env proxy URL,
			// avoiding last-write-wins clobbering in the global Component CR.
			if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, projectName, agentID, envName, envVarsToInject); err != nil {
				s.logger.Warn("failed to patch ReleaseBinding for env var injection (will apply on next deploy)",
					"environment", envName, "err", err)
			}

			// Step 4: For the first/dev environment, also update the Component CR once as a bootstrap
			// default so agents with no ReleaseBinding yet have a working config.
			if firstEnvName != "" && envName == firstEnvName {
				if err := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentID, envVarsToInject); err != nil {
					s.logger.Error("failed to update Component CR env vars for internal agent — Component CR in inconsistent state",
						"environment", envName, "err", err)
				}
			}
		}

		s.logger.Info(
			"Created proxy and deployment for environment",
			"environment", envName,
			"proxyURL", proxyURL,
			"proxyUUID", proxy.UUID,
		)
	}

	// Phase 3 — Success.
	s.logger.Info(
		"Agent configuration created successfully",
		"configUUID", config.UUID,
		"configName", config.Name,
		"agentID", agentID,
		"orgName", orgName,
		"projectName", projectName,
		"createdBy", createdBy,
		"environmentCount", len(req.EnvMappings),
	)

	// Return created configuration with credentials for external agents
	if isExternalAgent {
		return s.buildExternalAgentConfigResponse(ctx, config, envCredentials)
	}
	return s.Get(ctx, config.UUID, orgName, projectName, agentID)
}

func (s *agentConfigurationService) createMCPConfig(ctx context.Context, orgName, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string, isExternalAgent bool,
) (*models.AgentModelConfigResponse, error) {
	if s.mcpProxyRepo == nil || s.envMCPMappingRepo == nil || s.mcpProxyService == nil {
		return nil, fmt.Errorf("MCP configuration service is not fully configured")
	}
	if len(req.EnvMappings) == 0 {
		return nil, fmt.Errorf("%w: at least one environment mapping is required", utils.ErrInvalidInput)
	}
	if _, err := s.buildMCPMappingEnvironmentVariables(req.Name, req.EnvironmentVariables); err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}

	proxiesByEnv := make(map[string]*models.MCPProxy, len(req.EnvMappings))
	for envName, envMapping := range req.EnvMappings {
		proxyHandle := strings.TrimSpace(envMapping.ProviderName)
		if proxyHandle == "" {
			return nil, fmt.Errorf("%w: MCP proxy is required for environment %s", utils.ErrInvalidInput, envName)
		}
		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("MCP proxy for environment %s not found: %w", envName, utils.ErrMCPProxyNotFound)
			}
			return nil, fmt.Errorf("failed to validate MCP proxy for environment %s: %w", envName, err)
		}
		proxiesByEnv[envName] = proxy
	}

	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.Name] = env
	}
	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	config := &models.AgentConfiguration{
		Name:             req.Name,
		Description:      req.Description,
		AgentID:          agentID,
		TypeID:           models.AgentConfigTypeIDMCP,
		OrganizationName: orgName,
		ProjectName:      projectName,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Create(ctx, tx, config)
	}); err != nil {
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		return nil, fmt.Errorf("failed to create MCP configuration: %w", err)
	}

	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	var envCredentials map[string]envCredentialData
	if isExternalAgent {
		envCredentials = make(map[string]envCredentialData)
	}

	for envName := range req.EnvMappings {
		env := envMap[envName]
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}
		sourceProxy := proxiesByEnv[envName]
		handle := mcpMappingProxyName(projectName, agentID, config.Name, envName)
		artifactName := handle
		sourceProxyVersion := sourceProxy.Version
		if sourceProxy.Artifact != nil && sourceProxy.Artifact.Version != "" {
			sourceProxyVersion = sourceProxy.Artifact.Version
		}
		if sourceProxyVersion == "" {
			sourceProxyVersion = sourceProxy.Configuration.Version
		}
		mapping := &models.EnvAgentMCPMapping{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			MCPProxyUUID:    sourceProxy.UUID,
			ArtifactUUID:    uuid.New(),
		}
		deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, orgName, handle)
		proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
		envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, req.EnvironmentVariables)
		if err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to build MCP environment variables for %s: %w", envName, err)
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := s.envMCPMappingRepo.Create(ctx, tx, mapping, proxyMapping, handle, artifactName, sourceProxyVersion, orgName); err != nil {
				return err
			}
			return nil
		}); err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, err
		}

		gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
		if err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to resolve gateway for MCP environment %s: %w", envName, err)
		}
		if err := s.mcpProxyService.deployMCPProxyToGateway(ctx, deployedProxy, orgName, gateway); err != nil {
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to deploy MCP proxy for environment %s: %w", envName, err)
		}

		scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
		// Only provision an inbound API key when the source MCP proxy has api-key
		// security enabled. When disabled, the mapping is deployed without a gateway
		// key / app binding and no apikey env var is injected (only the URL).
		secured := mcpProxyAPIKeySecurityEnabled(sourceProxy)
		var proxyAPIKey *models.CreateAPIKeyResponse
		var proxySecretLoc secretmanagersvc.SecretLocation
		secretRefName := ""
		if secured {
			var err error
			proxyAPIKey, err = s.createMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, fmt.Sprintf("%s-key", scopedID))
			if err != nil {
				s.cleanupMCPConfig(ctx, config.UUID, orgName)
				return nil, fmt.Errorf("failed to generate MCP API key for environment %s: %w", envName, err)
			}
			agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, env.Name)
			_, _, err = s.aiApplicationService.EnsureAndBind(
				ctx, orgName, config.ProjectName, config.AgentID, env.Name,
				agentAppHandle,
				fmt.Sprintf("%s Application", config.AgentID),
				proxyAPIKey.KeyID,
			)
			if err != nil {
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after AI application failure", "environment", envName, "err", revokeErr)
				}
				s.cleanupMCPConfig(ctx, config.UUID, orgName)
				return nil, fmt.Errorf("failed to ensure AI application for MCP environment %s: %w", envName, err)
			}
			proxySecretLoc = secretmanagersvc.SecretLocation{
				OrgName:         orgName,
				ProjectName:     projectName,
				AgentName:       agentID,
				EnvironmentName: env.Name,
				ConfigName:      config.Name,
				EntityName:      fmt.Sprintf("%s-proxy", scopedID),
				SecretKey:       secretmanagersvc.SecretKeyAPIKey,
			}
			secretRefName, err = s.secretClient.CreateSecret(ctx, proxySecretLoc,
				map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
			if err != nil {
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after secret persistence failure", "environment", envName, "err", revokeErr)
				}
				s.cleanupMCPConfig(ctx, config.UUID, orgName)
				return nil, fmt.Errorf("failed to store MCP API key in KV for environment %s: %w", envName, err)
			}
		}
		variables := make([]models.AgentEnvConfigVariable, 0, len(envTemplates))
		for _, envTemplate := range envTemplates {
			secretReference := ""
			if envTemplate.IsSecret {
				secretReference = secretRefName
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envTemplate.Name,
				VariableKey:     envTemplate.Key,
				SecretReference: secretReference,
			})
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.envVariableRepo.CreateBatch(ctx, tx, variables)
		}); err != nil {
			if secured {
				if delErr := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefName); delErr != nil {
					s.logger.Warn("failed to delete MCP API key secret after env var persistence failure", "environment", envName, "err", delErr)
				}
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after env var persistence failure", "environment", envName, "err", revokeErr)
				}
			}
			s.cleanupMCPConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to create MCP environment variables for %s: %w", envName, err)
		}

		proxyURL := buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context)
		if isExternalAgent {
			apiKey := ""
			if proxyAPIKey != nil {
				apiKey = proxyAPIKey.APIKey
			}
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   apiKey,
				proxyURL: proxyURL,
			}
		}
		if !isExternalAgent {
			envVarsToInject := buildMCPEnvVars(envTemplates, proxyURL, secretRefName)
			if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, projectName, agentID, envName, envVarsToInject); err != nil {
				s.logger.Warn("failed to patch ReleaseBinding for MCP env var injection", "environment", envName, "err", err)
			}
			if firstEnvName != "" && envName == firstEnvName {
				if err := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentID, envVarsToInject); err != nil {
					s.logger.Warn("failed to patch Component for MCP env var bootstrap", "environment", envName, "err", err)
				}
			}
		}
	}

	if isExternalAgent {
		return s.buildExternalAgentConfigResponse(ctx, config, envCredentials)
	}
	return s.GetMCP(ctx, config.UUID, orgName, projectName, agentID)
}

// Get retrieves a configuration by UUID with project and agent scoping validation
func (s *agentConfigurationService) Get(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if config.ProjectName != projectName || config.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// GetMCP retrieves an MCP proxy mapping by UUID with project and agent scoping validation.
func (s *agentConfigurationService) GetMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return nil, utils.ErrAgentConfigNotFound
	}

	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)
	return s.buildConfigResponse(ctx, config, isExternal)
}

// GetByAgent retrieves configuration by agent ID
func (s *agentConfigurationService) GetByAgent(ctx context.Context, agentID, orgName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, orgName, config.ProjectName, agentID)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// List lists all configurations for an organization, project, and agent
func (s *agentConfigurationService) List(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.ListByAgent(ctx, orgName, projectName, agentName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	count, err := s.agentConfigRepo.CountByAgent(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to count configurations: %w", err)
	}

	items := make([]models.AgentModelConfigListItem, len(configs))
	for i, cfg := range configs {
		items[i] = models.AgentModelConfigListItem{
			UUID:             cfg.UUID.String(),
			Name:             cfg.Name,
			Description:      cfg.Description,
			AgentID:          cfg.AgentID,
			Type:             models.AgentConfigTypeFromID(cfg.TypeID),
			OrganizationName: cfg.OrganizationName,
			ProjectName:      cfg.ProjectName,
			CreatedAt:        cfg.CreatedAt,
		}
	}

	return &models.AgentModelConfigListResponse{
		Configs: items,
		Pagination: models.PaginationInfo{
			Count:  int(count),
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// ListByType lists configurations for an organization, project, agent, and config type.
func (s *agentConfigurationService) ListByType(
	ctx context.Context, orgName, projectName, agentName string, typeID uint, limit, offset int,
) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.ListByAgentAndType(ctx, orgName, projectName, agentName, typeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations by type: %w", err)
	}

	count, err := s.agentConfigRepo.CountByAgentAndType(ctx, orgName, projectName, agentName, typeID)
	if err != nil {
		return nil, fmt.Errorf("failed to count configurations by type: %w", err)
	}

	items := make([]models.AgentModelConfigListItem, len(configs))
	for i, cfg := range configs {
		items[i] = models.AgentModelConfigListItem{
			UUID:             cfg.UUID.String(),
			Name:             cfg.Name,
			Description:      cfg.Description,
			AgentID:          cfg.AgentID,
			Type:             models.AgentConfigTypeFromID(cfg.TypeID),
			OrganizationName: cfg.OrganizationName,
			ProjectName:      cfg.ProjectName,
			CreatedAt:        cfg.CreatedAt,
		}
	}

	return &models.AgentModelConfigListResponse{
		Configs: items,
		Pagination: models.PaginationInfo{
			Count:  int(count),
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// ListMCP lists all MCP proxy mappings for an organization, project, and agent.
func (s *agentConfigurationService) ListMCP(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	return s.ListByType(ctx, orgName, projectName, agentName, models.AgentConfigTypeIDMCP, limit, offset)
}

// processEnvProviderChange handles Scenario A: provider changed for an existing environment.
// External ops run outside any transaction; a short per-env TX follows.
// Returns the old proxy handle (for later cleanup) and the rollback resource for the new proxy.
func (s *agentConfigurationService) processEnvProviderChange(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	orgName string,
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (oldProxyHandle string, rbRes rollbackResource, err error) {
	s.logger.Info("Provider changed for environment, recreating proxy",
		"environment", envName,
		"oldProviderUUID", existingMapping.LLMProxy.Configuration.Provider,
		"newProviderName", envMapping.ProviderName)

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// Resolve gateway where the new provider is deployed
	gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, orgName, envUUID)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes = rollbackResource{
		providerAPIKeyID:  providerAPIKeyID,
		providerUUID:      providerUUID,
		providerSecretLoc: providerSecretLoc,
		mappingID:         existingMapping.ID,
		oldProxyUUID:      existingMapping.LLMProxyUUID,
	}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-key", scopedID),
	})
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Ensure one AI application exists per agent+env and bind the proxy API key.
	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	_, created, err := s.aiApplicationService.EnsureAndBind(
		ctx, orgName, config.ProjectName, config.AgentID, envName,
		agentAppHandle,
		fmt.Sprintf("%s Application", config.AgentID),
		proxyAPIKey.KeyID,
	)
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return "", rollbackResource{}, fmt.Errorf("processEnvProviderChange: failed to ensure AI application for environment %s: %w", envName, err)
	}
	if created {
		rbRes.createdNewApp = true
		rbRes.appAgentID = config.AgentID
		rbRes.appProjectName = config.ProjectName
		rbRes.appEnvName = envName
	}

	// Store proxy API key in OpenBao KV and create/update SecretReference
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return "", rollbackResource{}, fmt.Errorf("processEnvProviderChange: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretLoc = &proxySecretLoc
	rbRes.secretRefName = secretRefName

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return "", rollbackResource{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = secretRefName
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		existingMapping.LLMProxyUUID = proxy.UUID
		if err := s.envMappingRepo.Update(ctx, tx, existingMapping); err != nil {
			return fmt.Errorf("failed to update environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUID); err != nil {
			return fmt.Errorf("failed to delete old environment variables for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return "", rbRes, err
	}

	if existingMapping.LLMProxy != nil {
		oldProxyHandle = existingMapping.LLMProxy.Handle
	}

	// Internal-agent only: inject env vars into Component/ReleaseBinding.
	// SecretReference is already created/updated by secretClient.CreateSecret above.
	if !isExternalAgent {
		proxyURL := buildProxyURL(gateway, proxy.Configuration.Context, true)
		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
			s.logger.Error("failed to update Component CR env vars in Scenario A — Component CR in inconsistent state", "env", envName, "err", uvErr)
		}
		if firstEnvName != "" && envName == firstEnvName {
			if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, firstEnvName, envVarsToInject); rbErr != nil {
				s.logger.Warn("failed to patch ReleaseBinding in Scenario A", "env", envName, "err", rbErr)
			}
		}
	}

	return oldProxyHandle, rbRes, nil
}

// processEnvProxyUpdate handles Scenario B: same provider, update proxy config and redeploy.
// No DB TX needed — mapping already points to the same proxy UUID.
// Returns a non-nil rollback resource only if a new providerAPIKeyID was created.
func (s *agentConfigurationService) processEnvProxyUpdate(
	ctx context.Context,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	orgName string,
) (rollbackResource, error) {
	s.logger.Info("Updating proxy configuration for environment",
		"environment", envName,
		"providerName", envMapping.ProviderName)

	if existingMapping.LLMProxy == nil {
		return rollbackResource{}, fmt.Errorf("existing proxy not found for environment %s", envName)
	}

	gateway, err := s.resolveGatewayForProxy(ctx, existingMapping.LLMProxy.Handle, orgName, envUUID)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerUUID, err := s.buildLLMProxyUpdateConfig(config, envMapping, existingMapping.LLMProxy)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// LLMProxy.Handle is gorm:"-" and not populated by GORM Preload.
	// Use the existing proxy's handle (Configuration.Name) rather than recomputing it,
	// so the proxy identity is preserved exactly as created.
	proxyHandle := existingMapping.LLMProxy.Configuration.Name
	proxyConfig.UUID = existingMapping.LLMProxy.UUID
	proxyConfig.Handle = proxyHandle
	proxyConfig.CreatedBy = existingMapping.LLMProxy.CreatedBy
	proxyConfig.Status = existingMapping.LLMProxy.Status

	updatedProxy, err := s.llmProxyService.Update(proxyHandle, orgName, proxyConfig)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to update proxy for environment %s: %w", envName, err)
	}

	gatewayID := gateway.UUID.String()
	deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(updatedProxy.Handle, orgName, &gatewayID, nil)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to get deployments for environment %s: %w", envName, err)
	}

	var existingDeployment *models.Deployment
	for _, dep := range deployments {
		if dep.Status != nil && *dep.Status == models.DeploymentStatusDeployed {
			existingDeployment = dep
			break
		}
	}

	deployBase := "current"
	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
	newDeployment, err := s.llmProxyDeploymentService.DeployLLMProxy(updatedProxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      deployBase,
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to redeploy proxy for environment %s: %w", envName, err)
	}

	s.logger.Info("Proxy configuration updated and redeployed",
		"environment", envName,
		"proxyHandle", updatedProxy.Handle,
		"newDeploymentID", newDeployment.DeploymentID)

	// Persist updated PolicyConfiguration to DB.
	existingMapping.PolicyConfiguration = models.LLMPolicies(envMapping.Configuration.Policies)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.envMappingRepo.Update(ctx, tx, existingMapping)
	}); err != nil {
		// Return zero-value struct; providerAPIKeyID cleanup handled separately below if needed (LOW-2).
		return rollbackResource{}, fmt.Errorf("failed to update policy configuration for environment %s: %w", envName, err)
	}

	if existingDeployment != nil && existingDeployment.DeploymentID != newDeployment.DeploymentID {
		if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(updatedProxy.Handle, existingDeployment.DeploymentID.String(), orgName); err != nil {
			s.logger.Warn("Failed to clean up old deployment after redeployment",
				"environment", envName,
				"oldDeploymentID", existingDeployment.DeploymentID,
				"error", err)
		}
	}

	// Scenario B preserves the proxy handle and context path, so the proxy URL and secret reference
	// are identical to what is already injected. Skip Component CR and ReleaseBinding updates to
	// avoid triggering an unnecessary agent pod restart.

	return rollbackResource{providerUUID: providerUUID}, nil
}

// processNewEnv handles Scenario C: new environment added during update.
// Mirrors Create() per-env logic: external ops then a short per-env TX.
func (s *agentConfigurationService) processNewEnv(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	orgName string,
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (rollbackResource, error) {
	s.logger.Info("Adding new environment to configuration",
		"environment", envName,
		"providerName", envMapping.ProviderName)

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretLoc, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// Resolve gateway where the provider is deployed
	gateway, err := s.resolveGatewayForProvider(ctx, providerUUID, orgName, envUUID)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes := rollbackResource{providerAPIKeyID: providerAPIKeyID, providerUUID: providerUUID, providerSecretLoc: providerSecretLoc}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return rbRes, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, env.Name)
	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-deployment", scopedID),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-key", scopedID),
	})
	if err != nil {
		return rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Ensure one AI application exists per agent+env and bind the proxy API key.
	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	_, created, err := s.aiApplicationService.EnsureAndBind(
		ctx, orgName, config.ProjectName, config.AgentID, envName,
		agentAppHandle,
		fmt.Sprintf("%s Application", config.AgentID),
		proxyAPIKey.KeyID,
	)
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return rollbackResource{}, fmt.Errorf("processNewEnv: failed to ensure AI application for environment %s: %w", envName, err)
	}
	if created {
		rbRes.createdNewApp = true
		rbRes.appAgentID = config.AgentID
		rbRes.appProjectName = config.ProjectName
		rbRes.appEnvName = envName
	}

	// Store proxy API key in OpenBao KV and create/update SecretReference
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefName, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return rollbackResource{}, fmt.Errorf("processNewEnv: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretLoc = &proxySecretLoc
	rbRes.secretRefName = secretRefName

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return rollbackResource{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = secretRefName
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		mapping := &models.EnvAgentModelMapping{
			ConfigUUID:      configUUID,
			EnvironmentUUID: envUUID,
			LLMProxyUUID:    proxy.UUID,
		}
		if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
			return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return rbRes, err
	}

	// Internal-agent only: inject per-env vars into ReleaseBinding.
	// SecretReference is already created by secretClient.CreateSecret above.
	// The Component CR (global) is updated only for the first/dev environment to avoid
	// last-write-wins clobbering across multiple environments (HIGH-3).
	if !isExternalAgent {
		// Reuse the gateway already resolved for deployment (resolveGatewayForProvider)
		proxyURL := buildProxyURL(gateway, proxy.Configuration.Context, true)

		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		// Inject per-env URL into the ReleaseBinding for this specific environment.
		if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envName, envVarsToInject); rbErr != nil {
			s.logger.Warn("failed to patch ReleaseBinding in Scenario C", "env", envName, "err", rbErr)
		}
		// Update Component CR only for the first/dev environment as a bootstrap default.
		if firstEnvName != "" && envName == firstEnvName {
			if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
				s.logger.Error("failed to update Component CR env vars in Scenario C — Component CR in inconsistent state", "env", envName, "err", uvErr)
			}
		}
	}

	return rbRes, nil
}

// processEnvRemoval handles Scenario D: environment removed from the request.
// Removes env vars from the ReleaseBinding and, only when this is the last
// remaining environment (isLastEnv == true), also clears the Component CR.
func (s *agentConfigurationService) processEnvRemoval(
	ctx context.Context,
	configUUID uuid.UUID,
	envUUIDStr string,
	mapping *models.EnvAgentModelMapping,
	configName string,
	envName string,
	orgName string,
	projectName string,
	agentName string,
	isExternalAgent bool,
	existingVarNames map[string]string,
	isLastEnv bool,
) error {
	proxyHandle := "<nil>"
	if mapping.LLMProxy != nil {
		proxyHandle = mapping.LLMProxy.Handle
	}
	s.logger.Info("Removing environment from configuration",
		"environment", envUUIDStr,
		"proxyHandle", proxyHandle)

	envUUIDParsed, err := uuid.Parse(envUUIDStr)
	if err != nil {
		return fmt.Errorf("invalid environment UUID %q: %w", envUUIDStr, err)
	}

	// Internal-agent only: remove env vars from Component CR and the removed environment's ReleaseBinding.
	if !isExternalAgent && envName != "" {
		// Build the list of env var keys from DB-persisted names so user-overridden names are respected.
		envConfigTemplates, buildErr := s.buildEnvironmentVariables(configName, varNamesToOverrides(existingVarNames))
		if buildErr != nil {
			s.logger.Warn("failed to build env var keys for Scenario D cleanup, skipping env var removal", "err", buildErr)
		} else {
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from the removed environment's ReleaseBinding.
			if rbErr := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, keysToRemove); rbErr != nil {
				s.logger.Warn("failed to remove env vars from ReleaseBinding in Scenario D", "environment", envName, "err", rbErr)
			}
			// Remove from the Component CR only when this is the last environment.
			// If other environments survive, their ReleaseBindings still hold the
			// correct per-env values and the Component CR should be left intact.
			if isLastEnv {
				if compErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, keysToRemove); compErr != nil {
					s.logger.Warn("failed to remove env vars from Component CR in Scenario D", "environment", envName, "err", compErr)
				}
			}
		}

		// Delete SecretReference CR after consumer refs have been cleaned up (best-effort).
		// Use the persisted SecretReference from AgentEnvConfigVariable (set at creation time)
		// rather than deriving it from mutable fields like configName which may have been renamed.
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, envUUIDParsed)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for SecretReference lookup in Scenario D", "err", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					s.logger.Info("Scenario D: using persisted SecretReference for deletion",
						"secretRef", v.SecretReference, "variableName", v.VariableName,
						"configUUID", configUUID, "environment", envName)
					if delErr := s.ocClient.DeleteSecretReference(ctx, orgName, v.SecretReference); delErr != nil {
						s.logger.Warn("failed to delete SecretReference in Scenario D", "name", v.SecretReference, "err", delErr)
					}
					break // Only one secret ref per config+env
				}
			}
		}
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUIDParsed); err != nil {
			return fmt.Errorf("failed to delete environment variables for %s: %w", envUUIDStr, err)
		}
		if err := s.envMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return fmt.Errorf("failed to delete environment mapping for %s: %w", envUUIDStr, err)
		}
		return nil
	})
}

func (s *agentConfigurationService) updateMCPConfig(ctx context.Context, existingConfig *models.AgentConfiguration, orgName, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	if s.mcpProxyRepo == nil || s.envMCPMappingRepo == nil || s.mcpProxyService == nil {
		return nil, fmt.Errorf("MCP configuration service is not fully configured")
	}

	allEnvs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse, len(allEnvs))
	uuidToEnvName := make(map[string]string, len(allEnvs))
	for _, e := range allEnvs {
		envMap[e.Name] = e
		uuidToEnvName[e.UUID] = e.Name
	}

	nameChanged := req.Name != "" && req.Name != existingConfig.Name
	if req.Name != "" {
		existingConfig.Name = req.Name
	}
	if req.Description != "" {
		existingConfig.Description = req.Description
	}
	if req.Name != "" || req.Description != "" {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.agentConfigRepo.Update(ctx, tx, existingConfig)
		}); err != nil {
			return nil, fmt.Errorf("failed to update MCP configuration: %w", err)
		}
	}

	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return nil, fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	if len(req.EnvironmentVariables) > 0 {
		if err := s.updateMCPConfigEnvironmentVariableNames(ctx, existingConfig, orgName, projectName, agentName, uuidToEnvName, isExternalAgent, firstEnvName, req.EnvironmentVariables); err != nil {
			return nil, err
		}
	}

	existingVarNames, err := s.loadExistingVarNames(ctx, existingConfig.UUID)
	if err != nil {
		return nil, err
	}
	envTemplates, err := s.buildMCPMappingEnvironmentVariables(existingConfig.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}

	if req.EnvMappings == nil {
		if nameChanged {
			if err := s.refreshAllMCPMappings(ctx, existingConfig, orgName, uuidToEnvName, envTemplates, isExternalAgent, firstEnvName); err != nil {
				return nil, err
			}
		}
		return s.GetMCP(ctx, existingConfig.UUID, orgName, projectName, agentName)
	}

	proxiesByEnv := make(map[string]*models.MCPProxy, len(req.EnvMappings))
	for envName, envMapping := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
		proxyHandle := strings.TrimSpace(envMapping.ProviderName)
		if proxyHandle == "" {
			return nil, fmt.Errorf("%w: MCP proxy is required for environment %s", utils.ErrInvalidInput, envName)
		}
		proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("MCP proxy for environment %s not found: %w", envName, utils.ErrMCPProxyNotFound)
			}
			return nil, fmt.Errorf("failed to validate MCP proxy for environment %s: %w", envName, err)
		}
		proxiesByEnv[envName] = proxy
	}

	existingEnvMap := make(map[string]*models.EnvAgentMCPMapping, len(existingConfig.EnvMCPMappings))
	for i := range existingConfig.EnvMCPMappings {
		envUUID := existingConfig.EnvMCPMappings[i].EnvironmentUUID.String()
		name := uuidToEnvName[envUUID]
		if name == "" {
			name = envUUID
		}
		existingEnvMap[name] = &existingConfig.EnvMCPMappings[i]
	}

	for envName := range req.EnvMappings {
		env := envMap[envName]
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}
		sourceProxy := proxiesByEnv[envName]
		handle := mcpMappingProxyName(projectName, agentName, existingConfig.Name, envName)
		artifactName := handle
		sourceVersion := mcpProxyArtifactVersion(sourceProxy)

		if mapping, ok := existingEnvMap[envName]; ok {
			sourceChanged := mapping.MCPProxyUUID != sourceProxy.UUID
			shouldRedeploy := sourceChanged || nameChanged
			if err := s.updateExistingMCPMapping(ctx, existingConfig, mapping, sourceProxy, envName, orgName, handle, artifactName, sourceVersion, false); err != nil {
				return nil, err
			}
			if err := s.reconcileMCPMappingCredentials(ctx, existingConfig, mapping, sourceProxy, envName, orgName, envTemplates, isExternalAgent, firstEnvName); err != nil {
				return nil, err
			}
			if shouldRedeploy {
				if err := s.deployMCPMapping(ctx, existingConfig, mapping, sourceProxy, envName, orgName, handle); err != nil {
					return nil, err
				}
			}
			if shouldRedeploy && !isExternalAgent {
				if err := s.injectMCPMappingEnvVars(ctx, existingConfig, mapping, sourceProxy, envName, orgName, envTemplates, firstEnvName); err != nil {
					s.logger.Warn("failed to inject updated MCP mapping env vars", "environment", envName, "err", err)
				}
			}
			delete(existingEnvMap, envName)
			continue
		}

		mapping := &models.EnvAgentMCPMapping{
			ConfigUUID:      existingConfig.UUID,
			EnvironmentUUID: envUUID,
			MCPProxyUUID:    sourceProxy.UUID,
			ArtifactUUID:    uuid.New(),
		}
		deployedProxy := buildAgentMCPConfigProxy(existingConfig, mapping, sourceProxy, envName, orgName, handle)
		proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := s.envMCPMappingRepo.Create(ctx, tx, mapping, proxyMapping, handle, artifactName, sourceVersion, orgName); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to create MCP mapping for environment %s: %w", envName, err)
		}
		if err := s.deployMCPMapping(ctx, existingConfig, mapping, sourceProxy, envName, orgName, handle); err != nil {
			s.cleanupNewMCPMapping(ctx, existingConfig, mapping, envName, orgName)
			return nil, err
		}
		scopedID := scopedProxyIdentifier(existingConfig.ProjectName, existingConfig.AgentID, existingConfig.Name, envName)
		// Mirror the create flow: only provision an inbound API key when the source
		// MCP proxy has api-key security enabled.
		secured := mcpProxyAPIKeySecurityEnabled(sourceProxy)
		var proxyAPIKey *models.CreateAPIKeyResponse
		var proxySecretLoc secretmanagersvc.SecretLocation
		secretRefName := ""
		if secured {
			var err error
			proxyAPIKey, err = s.createMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, fmt.Sprintf("%s-key", scopedID))
			if err != nil {
				s.cleanupNewMCPMapping(ctx, existingConfig, mapping, envName, orgName)
				return nil, fmt.Errorf("failed to generate MCP API key for environment %s: %w", envName, err)
			}
			agentAppHandle := agentAppIdentifier(existingConfig.ProjectName, existingConfig.AgentID, envName)
			_, _, err = s.aiApplicationService.EnsureAndBind(
				ctx, orgName, existingConfig.ProjectName, existingConfig.AgentID, envName,
				agentAppHandle,
				fmt.Sprintf("%s Application", existingConfig.AgentID),
				proxyAPIKey.KeyID,
			)
			if err != nil {
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after AI application failure", "environment", envName, "err", revokeErr)
				}
				s.cleanupNewMCPMapping(ctx, existingConfig, mapping, envName, orgName)
				return nil, fmt.Errorf("failed to ensure AI application for MCP environment %s: %w", envName, err)
			}
			proxySecretLoc = secretmanagersvc.SecretLocation{
				OrgName:         orgName,
				ProjectName:     existingConfig.ProjectName,
				AgentName:       existingConfig.AgentID,
				EnvironmentName: envName,
				ConfigName:      existingConfig.Name,
				EntityName:      fmt.Sprintf("%s-proxy", scopedID),
				SecretKey:       secretmanagersvc.SecretKeyAPIKey,
			}
			secretRefName, err = s.secretClient.CreateSecret(ctx, proxySecretLoc,
				map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
			if err != nil {
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after secret persistence failure", "environment", envName, "err", revokeErr)
				}
				s.cleanupNewMCPMapping(ctx, existingConfig, mapping, envName, orgName)
				return nil, fmt.Errorf("failed to store MCP API key in KV for environment %s: %w", envName, err)
			}
		}
		variables := make([]models.AgentEnvConfigVariable, 0, len(envTemplates))
		for _, envTemplate := range envTemplates {
			secretReference := ""
			if envTemplate.IsSecret {
				secretReference = secretRefName
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      existingConfig.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envTemplate.Name,
				VariableKey:     envTemplate.Key,
				SecretReference: secretReference,
			})
		}
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.envVariableRepo.CreateBatch(ctx, tx, variables)
		}); err != nil {
			if secured {
				if delErr := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefName); delErr != nil {
					s.logger.Warn("failed to delete MCP API key secret after env var persistence failure", "environment", envName, "err", delErr)
				}
				if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, proxyAPIKey.KeyID); revokeErr != nil {
					s.logger.Warn("failed to revoke MCP API key after env var persistence failure", "environment", envName, "err", revokeErr)
				}
			}
			s.cleanupNewMCPMapping(ctx, existingConfig, mapping, envName, orgName)
			return nil, fmt.Errorf("failed to create MCP environment variables for %s: %w", envName, err)
		}
		if !isExternalAgent {
			if err := s.injectMCPMappingEnvVars(ctx, existingConfig, mapping, sourceProxy, envName, orgName, envTemplates, firstEnvName); err != nil {
				s.logger.Warn("failed to inject MCP mapping env vars", "environment", envName, "err", err)
			}
		}
	}

	survivingEnvCount := len(req.EnvMappings)
	for envName, mapping := range existingEnvMap {
		isLastEnv := survivingEnvCount == 0
		if err := s.removeMCPMappingEnvironment(ctx, existingConfig, mapping, envName, orgName, projectName, agentName, envTemplates, isExternalAgent, isLastEnv); err != nil {
			return nil, err
		}
	}

	return s.GetMCP(ctx, existingConfig.UUID, orgName, projectName, agentName)
}

func (s *agentConfigurationService) updateMCPConfigEnvironmentVariableNames(
	ctx context.Context,
	config *models.AgentConfiguration,
	orgName, projectName, agentName string,
	uuidToEnvName map[string]string,
	isExternalAgent bool,
	firstEnvName string,
	overrides []models.EnvironmentVariableConfig,
) error {
	var oldVarNames map[string]string
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		vars, err := s.envVariableRepo.ListByConfigForUpdate(ctx, tx, config.UUID)
		if err != nil {
			return fmt.Errorf("failed to load existing variable names: %w", err)
		}
		persisted := make(map[string]string)
		for _, v := range vars {
			if _, ok := persisted[v.VariableKey]; !ok {
				persisted[v.VariableKey] = v.VariableName
			}
		}
		oldVarNames = persisted
		merged := varNamesToOverrides(persisted)
		for _, ev := range overrides {
			found := false
			for i := range merged {
				if merged[i].Key == ev.Key {
					merged[i].Name = ev.Name
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, ev)
			}
		}
		if _, err := s.buildMCPMappingEnvironmentVariables(config.Name, merged); err != nil {
			return errors.Join(utils.ErrInvalidInput, err)
		}
		keyNameMap := make(map[string]string, len(overrides))
		for _, ev := range overrides {
			keyNameMap[ev.Key] = ev.Name
		}
		return s.envVariableRepo.UpdateVariableNames(ctx, tx, config.UUID, keyNameMap)
	}); err != nil {
		return fmt.Errorf("failed to update MCP environment variable names: %w", err)
	}
	if isExternalAgent || len(oldVarNames) == 0 {
		return nil
	}

	changedOldKeys := make([]string, 0, len(overrides))
	mergedNames := varNamesToOverrides(oldVarNames)
	for _, ev := range overrides {
		if old, ok := oldVarNames[ev.Key]; ok && old != ev.Name {
			changedOldKeys = append(changedOldKeys, old)
		}
		for i := range mergedNames {
			if mergedNames[i].Key == ev.Key {
				mergedNames[i].Name = ev.Name
			}
		}
	}
	if len(changedOldKeys) == 0 {
		return nil
	}
	if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, changedOldKeys); err != nil {
		s.logger.Warn("failed to remove old MCP env vars from Component CR", "err", err)
	}
	newTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, mergedNames)
	if err != nil {
		return err
	}
	for i := range config.EnvMCPMappings {
		mapping := &config.EnvMCPMappings[i]
		envName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if envName == "" || mapping.MCPProxy == nil {
			continue
		}
		gateway, gwErr := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, orgName, mapping.EnvironmentUUID)
		if gwErr != nil {
			s.logger.Warn("failed to resolve MCP gateway for env var rename", "environment", envName, "err", gwErr)
			continue
		}
		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
		deployedProxy := buildAgentMCPConfigProxy(config, mapping, mapping.MCPProxy, envName, orgName, handle)
		secretRefName, refErr := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
		if refErr != nil {
			s.logger.Warn("failed to load MCP SecretReference for env var rename", "environment", envName, "err", refErr)
			continue
		}
		envVarsToInject := buildMCPEnvVars(newTemplates, buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context), secretRefName)
		if err := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, changedOldKeys, envVarsToInject); err != nil {
			s.logger.Warn("failed to replace MCP env vars in ReleaseBinding", "environment", envName, "err", err)
		}
		if firstEnvName != "" && envName == firstEnvName {
			if err := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentName, envVarsToInject); err != nil {
				s.logger.Warn("failed to update Component CR with renamed MCP env vars", "environment", envName, "err", err)
			}
		}
	}
	return nil
}

func (s *agentConfigurationService) refreshAllMCPMappings(ctx context.Context, config *models.AgentConfiguration, orgName string, uuidToEnvName map[string]string,
	envTemplates []EnvConfigTemplate, isExternalAgent bool, firstEnvName string,
) error {
	for i := range config.EnvMCPMappings {
		mapping := &config.EnvMCPMappings[i]
		envName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if envName == "" || mapping.MCPProxy == nil {
			continue
		}
		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
		artifactName := handle
		version := mcpProxyArtifactVersion(mapping.MCPProxy)
		if err := s.updateExistingMCPMapping(ctx, config, mapping, mapping.MCPProxy, envName, orgName, handle, artifactName, version, false); err != nil {
			return err
		}
		if err := s.reconcileMCPMappingCredentials(ctx, config, mapping, mapping.MCPProxy, envName, orgName, envTemplates, isExternalAgent, firstEnvName); err != nil {
			return err
		}
		if err := s.deployMCPMapping(ctx, config, mapping, mapping.MCPProxy, envName, orgName, handle); err != nil {
			return err
		}
		if !isExternalAgent {
			if err := s.injectMCPMappingEnvVars(ctx, config, mapping, mapping.MCPProxy, envName, orgName, envTemplates, firstEnvName); err != nil {
				s.logger.Warn("failed to inject refreshed MCP mapping env vars", "environment", envName, "err", err)
			}
		}
	}
	return nil
}

func (s *agentConfigurationService) updateExistingMCPMapping(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	sourceProxy *models.MCPProxy, envName, orgName, handle, artifactName, version string, redeploy bool,
) error {
	mapping.MCPProxyUUID = sourceProxy.UUID
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, orgName, handle)
	proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&models.Artifact{}).
			Where("uuid = ?", mapping.ArtifactUUID).
			Updates(map[string]interface{}{
				"handle":     handle,
				"name":       artifactName,
				"version":    version,
				"kind":       models.KindMCPMapping,
				"in_catalog": false,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&models.MCPProxyMapping{}).
			Where("uuid = ?", mapping.ArtifactUUID).
			Updates(map[string]interface{}{
				"source_mcp_proxy_uuid": proxyMapping.SourceMCPProxyUUID,
				"description":           proxyMapping.Description,
				"status":                proxyMapping.Status,
				"configuration":         proxyMapping.Configuration,
			}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&models.EnvAgentMCPMapping{}).
			Where("id = ?", mapping.ID).
			Updates(map[string]interface{}{
				"mcp_proxy_uuid": mapping.MCPProxyUUID,
				"artifact_uuid":  mapping.ArtifactUUID,
			}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update MCP mapping for environment %s: %w", envName, err)
	}
	if redeploy {
		return s.deployMCPMapping(ctx, config, mapping, sourceProxy, envName, orgName, handle)
	}
	return nil
}

func (s *agentConfigurationService) deployMCPMapping(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	sourceProxy *models.MCPProxy, envName, orgName, handle string,
) error {
	gateway, err := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, orgName, mapping.EnvironmentUUID)
	if err != nil {
		return fmt.Errorf("failed to resolve gateway for MCP environment %s: %w", envName, err)
	}
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, orgName, handle)
	if err := s.mcpProxyService.deployMCPProxyToGateway(ctx, deployedProxy, orgName, gateway); err != nil {
		return fmt.Errorf("failed to deploy MCP mapping for environment %s: %w", envName, err)
	}
	return nil
}

func (s *agentConfigurationService) injectMCPMappingEnvVars(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	sourceProxy *models.MCPProxy, envName, orgName string, envTemplates []EnvConfigTemplate, firstEnvName string,
) error {
	gateway, err := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, orgName, mapping.EnvironmentUUID)
	if err != nil {
		return err
	}
	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, orgName, handle)
	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		return err
	}
	envVarsToInject := buildMCPEnvVars(envTemplates, buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context), secretRefName)
	if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envName, envVarsToInject); err != nil {
		return err
	}
	if firstEnvName != "" && envName == firstEnvName {
		return s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject)
	}
	return nil
}

func (s *agentConfigurationService) cleanupMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, orgName string) {
	if config == nil || mapping == nil || envName == "" {
		return
	}
	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName)
	keyName := fmt.Sprintf("%s-key", scopedID)
	if err := s.revokeAllMCPMappingAPIKeys(ctx, orgName, mapping.ArtifactUUID); err != nil {
		s.logger.Warn("failed to revoke MCP mapping API key", "mappingHandle", handle, "keyName", keyName, "err", err)
	}

	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		s.logger.Warn("failed to load MCP SecretReference for cleanup", "environment", envName, "err", err)
	}
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: envName,
		ConfigName:      config.Name,
		EntityName:      fmt.Sprintf("%s-proxy", scopedID),
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	secretRefForDelete := secretRefName
	if secretRefForDelete == "" {
		secretRefForDelete = proxySecretLoc.SecretRefName()
	}
	if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
		s.logger.Warn("failed to delete MCP mapping API key secret", "mappingHandle", handle, "secretRef", secretRefForDelete, "err", err)
	}
}

func (s *agentConfigurationService) removeMCPMappingEnvironment(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping,
	envName, orgName, projectName, agentName string, envTemplates []EnvConfigTemplate, isExternalAgent, isLastEnv bool,
) error {
	if !isExternalAgent && envName != "" {
		keysToRemove := make([]string, 0, len(envTemplates))
		for _, t := range envTemplates {
			keysToRemove = append(keysToRemove, t.Name)
		}
		if len(keysToRemove) > 0 {
			if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove MCP env vars from ReleaseBinding", "environment", envName, "err", err)
			}
			if isLastEnv {
				if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, keysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Component CR", "environment", envName, "err", err)
				}
			}
		}
	}

	if s.mcpProxyService != nil {
		s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, orgName)
	}
	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, orgName)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, config.UUID, mapping.EnvironmentUUID); err != nil {
			return err
		}
		if err := s.envMCPMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error
	})
}

func mcpProxyArtifactVersion(source *models.MCPProxy) string {
	if source == nil {
		return models.DefaultProxyVersion
	}
	if source.Artifact != nil && source.Artifact.Version != "" {
		return source.Artifact.Version
	}
	if source.Version != "" {
		return source.Version
	}
	if source.Configuration.Version != "" {
		return source.Configuration.Version
	}
	return models.DefaultProxyVersion
}

// UpdateMCP updates an existing MCP proxy mapping with project and agent scoping validation.
func (s *agentConfigurationService) UpdateMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return nil, utils.ErrAgentConfigNotFound
	}
	return s.updateMCPConfig(ctx, config, orgName, projectName, agentName, req)
}

// Update updates an existing configuration with project and agent scoping validation.
// External network calls (proxy create/update/deploy, API key generation) are performed outside
// transactions. Only pure DB writes use short, focused transactions.
//
// NOTE: Partial failure across multiple environments is an accepted limitation (see SAGA.md).
// On failure in env N, envs 1..N-1 may already be updated. Retry is possible but not idempotent.
func (s *agentConfigurationService) Update(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	// Get existing configuration with all mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
		return s.updateMCPConfig(ctx, existingConfig, orgName, projectName, agentName, req)
	}

	// Load environments once; used to key existingEnvMap by name and to validate request envs.
	allEnvs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse, len(allEnvs))
	uuidToEnvName := make(map[string]string, len(allEnvs))
	for _, e := range allEnvs {
		envMap[e.Name] = e
		uuidToEnvName[e.UUID] = e.Name
	}

	// Build map of existing environment mappings for comparison, keyed by environment name.
	// The request uses env names, so we must match by name (not UUID).
	existingEnvMap := make(map[string]*models.EnvAgentModelMapping, len(existingConfig.EnvMappings))
	for i := range existingConfig.EnvMappings {
		envUUID := existingConfig.EnvMappings[i].EnvironmentUUID.String()
		name := uuidToEnvName[envUUID]
		if name == "" {
			name = envUUID // fall back to UUID if env was deleted
		}
		existingEnvMap[name] = &existingConfig.EnvMappings[i]
	}

	// Validate all providers exist and are in catalog (if envMappings provided)
	if req.EnvMappings != nil {
		for envName, envMapping := range req.EnvMappings {
			provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, orgName)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					s.logger.Warn("Provider not found", "env", envName, "error", err)
					return nil, fmt.Errorf("provider for environment %s not found: %w", envName, utils.ErrLLMProviderNotFound)
				}
				return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
			}
			if !provider.InCatalog {
				return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderName, envName)
			}
		}
	}

	// Phase 1 — Short TX: update name/description only.
	if req.Name != "" {
		existingConfig.Name = req.Name
	}
	if req.Description != "" {
		existingConfig.Description = req.Description
	}
	if req.Name != "" || req.Description != "" {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.agentConfigRepo.Update(ctx, tx, existingConfig)
		}); err != nil {
			return nil, fmt.Errorf("failed to update configuration: %w", err)
		}
	}

	// Phase 1b — Update env var names if provided (global rename across all environments).
	// Read, validate, and write happen inside a single transaction with a row-level lock to
	// prevent concurrent rename requests from bypassing uniqueness checks.
	if len(req.EnvironmentVariables) > 0 {
		// oldVarNames is populated inside the transaction (under the row lock) so the
		// snapshot is consistent with the locked state used for the rename.
		var oldVarNames map[string]string

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			// Lock the rows so concurrent renames on the same config are serialised.
			vars, err := s.envVariableRepo.ListByConfigForUpdate(ctx, tx, configUUID)
			if err != nil {
				return fmt.Errorf("failed to load existing variable names: %w", err)
			}
			// Build key→name map from locked rows (first-occurrence wins per key).
			persistedVarNames := make(map[string]string)
			for _, v := range vars {
				if _, already := persistedVarNames[v.VariableKey]; !already {
					persistedVarNames[v.VariableKey] = v.VariableName
				}
			}
			// Capture old names under the same lock used for the rename.
			oldVarNames = persistedVarNames
			// Merge requested renames over persisted names.
			mergedOverrides := make([]models.EnvironmentVariableConfig, 0, len(persistedVarNames))
			for key, name := range persistedVarNames {
				mergedOverrides = append(mergedOverrides, models.EnvironmentVariableConfig{Key: key, Name: name})
			}
			for _, ev := range req.EnvironmentVariables {
				found := false
				for i, mo := range mergedOverrides {
					if mo.Key == ev.Key {
						mergedOverrides[i].Name = ev.Name
						found = true
						break
					}
				}
				if !found {
					mergedOverrides = append(mergedOverrides, ev)
				}
			}
			// Validate using the merged result (catches uniqueness and format errors against locked names).
			var validateErr error
			if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
				_, validateErr = s.buildMCPMappingEnvironmentVariables(existingConfig.Name, mergedOverrides)
			} else {
				_, validateErr = s.buildEnvironmentVariables(existingConfig.Name, mergedOverrides)
			}
			if validateErr != nil {
				return errors.Join(utils.ErrInvalidInput, validateErr)
			}
			keyNameMap := make(map[string]string, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				keyNameMap[ev.Key] = ev.Name
			}
			return s.envVariableRepo.UpdateVariableNames(ctx, tx, configUUID, keyNameMap)
		}); err != nil {
			return nil, fmt.Errorf("failed to update environment variable names: %w", err)
		}

		// For internal agents: remove old env var names from the Component CR and all
		// per-environment ReleaseBindings so stale variables don't linger after a rename.
		// Only runs when at least one name actually changed; skipped entirely if nothing differed.
		// Best-effort — failures are logged but do not abort the update.
		if len(oldVarNames) > 0 {
			// Collect names that were actually renamed (old name != new name).
			changedOldKeys := make([]string, 0, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				if existing, ok := oldVarNames[ev.Key]; ok && existing != ev.Name {
					changedOldKeys = append(changedOldKeys, existing)
				}
			}
			if len(changedOldKeys) > 0 {
				agentComp, compErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
				if compErr != nil {
					s.logger.Warn("Phase 1b: failed to determine agent type for env var cleanup", "err", compErr)
				} else if agentComp.Provisioning.Type != string(utils.ExternalAgent) {
					// Remove old names from Component CR.
					if rmErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, changedOldKeys); rmErr != nil {
						s.logger.Warn("Phase 1b: failed to remove old env vars from Component CR", "err", rmErr)
					}

					// Build new env var templates for re-injection.
					newOverrides := make([]models.EnvironmentVariableConfig, 0, len(oldVarNames))
					for key, name := range oldVarNames {
						newOverrides = append(newOverrides, models.EnvironmentVariableConfig{Key: key, Name: name})
					}
					for _, ev := range req.EnvironmentVariables {
						for j, o := range newOverrides {
							if o.Key == ev.Key {
								newOverrides[j].Name = ev.Name
								break
							}
						}
					}
					var newEnvConfigTemplates []EnvConfigTemplate
					var buildErr error
					if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
						newEnvConfigTemplates, buildErr = s.buildMCPMappingEnvironmentVariables(existingConfig.Name, newOverrides)
					} else {
						newEnvConfigTemplates, buildErr = s.buildEnvironmentVariables(existingConfig.Name, newOverrides)
					}
					if buildErr != nil {
						s.logger.Warn("Phase 1b: failed to build new env var templates for re-injection after rename", "err", buildErr)
					}

					// Determine first env for Component CR bootstrap update.
					firstEnvName1b := ""
					if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName); pipelineErr == nil && pipeline != nil {
						firstEnvName1b = client.FindFirstEnvironment(pipeline.PromotionPaths)
					}

					// Atomic per-environment: remove old keys + inject new env vars in a single
					// ReleaseBinding Get/Update cycle to avoid resource version conflicts that
					// cause 500 errors when remove and add are separate API calls.
					if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
						for i := range existingConfig.EnvMCPMappings {
							mapping := &existingConfig.EnvMCPMappings[i]
							envUUID := mapping.EnvironmentUUID.String()
							envName := uuidToEnvName[envUUID]
							if envName == "" || buildErr != nil || mapping.MCPProxy == nil {
								continue
							}
							envEnvUUID, parseErr := uuid.Parse(envUUID)
							if parseErr != nil {
								continue
							}
							gateway, gwErr := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, orgName, envEnvUUID)
							if gwErr != nil {
								s.logger.Warn("Phase 1b: failed to resolve MCP gateway for re-injection", "environment", envName, "err", gwErr)
								continue
							}
							deployedProxy := buildAgentMCPConfigProxy(existingConfig, mapping, mapping.MCPProxy, envName, orgName,
								mcpMappingProxyName(existingConfig.ProjectName, existingConfig.AgentID, existingConfig.Name, envName))
							secretRefName, refErr := s.loadSecretRefForConfigEnv(ctx, existingConfig.UUID, mapping.EnvironmentUUID)
							if refErr != nil {
								s.logger.Warn("Phase 1b: failed to load MCP SecretReference for re-injection", "environment", envName, "err", refErr)
								continue
							}
							envVarsToInject := buildMCPEnvVars(newEnvConfigTemplates, buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context), secretRefName)
							s.logger.Info("Phase 1b: atomically replacing MCP env vars in ReleaseBinding",
								"environment", envName, "keysToRemove", changedOldKeys, "envVarsToAdd", len(envVarsToInject))
							if rbErr := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, changedOldKeys, envVarsToInject); rbErr != nil {
								s.logger.Warn("Phase 1b: failed to replace MCP env vars in ReleaseBinding", "environment", envName, "err", rbErr)
							}
							if firstEnvName1b != "" && envName == firstEnvName1b {
								if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentName, envVarsToInject); uvErr != nil {
									s.logger.Warn("Phase 1b: failed to re-inject new MCP env var names into Component CR", "environment", envName, "err", uvErr)
								}
							}
						}
					} else {
						for i := range existingConfig.EnvMappings {
							mapping := &existingConfig.EnvMappings[i]
							envUUID := mapping.EnvironmentUUID.String()
							envName := uuidToEnvName[envUUID]
							if envName == "" || buildErr != nil || mapping.LLMProxy == nil {
								continue
							}
							envEnvUUID, parseErr := uuid.Parse(envUUID)
							if parseErr != nil {
								continue
							}
							gateway, gwErr := s.resolveGatewayForProxy(ctx, mapping.LLMProxy.Handle, orgName, envEnvUUID)
							if gwErr != nil {
								s.logger.Warn("Phase 1b: failed to resolve gateway for re-injection", "environment", envName, "err", gwErr)
								continue
							}
							proxyURL := buildProxyURL(gateway, mapping.LLMProxy.Configuration.Context, true)
							// Use persisted SecretReference from DB rather than deriving from mutable config name.
							envVars1b, varErr1b := s.envVariableRepo.ListByConfigAndEnv(ctx, existingConfig.UUID, mapping.EnvironmentUUID)
							secretRefName := ""
							if varErr1b != nil {
								s.logger.Warn("Phase 1b: failed to load persisted SecretReference", "environment", envName, "err", varErr1b)
								continue
							}
							for _, v := range envVars1b {
								if v.SecretReference != "" {
									secretRefName = v.SecretReference
									s.logger.Info("Phase 1b: using persisted SecretReference for re-injection",
										"secretRef", secretRefName, "variableName", v.VariableName,
										"configUUID", existingConfig.UUID, "environment", envName)
									break
								}
							}
							if secretRefName == "" {
								s.logger.Warn("Phase 1b: no persisted SecretReference found, skipping re-injection", "environment", envName)
								continue
							}
							envVarsToInject := buildLLMEnvVars(newEnvConfigTemplates, proxyURL, secretRefName)
							s.logger.Info("Phase 1b: atomically replacing env vars in ReleaseBinding",
								"environment", envName, "keysToRemove", changedOldKeys, "envVarsToAdd", len(envVarsToInject))
							if rbErr := s.ocClient.ReplaceReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, changedOldKeys, envVarsToInject); rbErr != nil {
								s.logger.Warn("Phase 1b: failed to replace env vars in ReleaseBinding", "environment", envName, "err", rbErr)
							}
							if firstEnvName1b != "" && envName == firstEnvName1b {
								if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentName, envVarsToInject); uvErr != nil {
									s.logger.Warn("Phase 1b: failed to re-inject new env var names into Component CR", "environment", envName, "err", uvErr)
								}
							}
						}
					}
				}
			}
		}
	}

	// If no envMappings provided, return the updated config immediately.
	if req.EnvMappings == nil {
		return s.Get(ctx, configUUID, orgName, projectName, agentName)
	}

	// Load existing variable names so new/replaced envs get consistent names.
	existingVarNames, err := s.loadExistingVarNames(ctx, configUUID)
	if err != nil {
		return nil, err
	}

	// Determine agent type and first env for internal-agent env var injection.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return nil, fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
	}

	// Track resources for rollback and old proxies to clean up post-success.
	var rollbackResources []rollbackResource
	var proxiesToDelete []string

	// Phase 2/3 — Loop over requested environments, calling scenario helpers.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		select {
		case <-ctx.Done():
			// Use a fresh context for cleanup so cancelled ctx doesn't prevent rollback (CRIT-2).
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			s.rollbackProxies(cleanupCtx, rollbackResources, orgName)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		existingMapping, hasExisting := existingEnvMap[envName]

		if hasExisting {
			var newProviderUUID string
			if existingMapping.LLMProxy != nil {
				newProvider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, orgName)
				if err == nil {
					newProviderUUID = newProvider.UUID.String()
				}
			}
			providerChanged := existingMapping.LLMProxy != nil &&
				existingMapping.LLMProxy.Configuration.Provider != newProviderUUID

			if providerChanged {
				// Scenario A: provider changed — create new proxy, update mapping, schedule old proxy for cleanup.
				oldHandle, rbRes, err := s.processEnvProviderChange(
					ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName, existingVarNames, isExternalAgent, firstEnvName,
				)
				if err != nil {
					s.rollbackProxies(ctx, rollbackResources, orgName)
					return nil, err
				}
				rollbackResources = append(rollbackResources, rbRes)
				if oldHandle != "" {
					proxiesToDelete = append(proxiesToDelete, oldHandle)
				}
			} else {
				// Scenario B: same provider — update proxy config and redeploy. No DB TX needed.
				rbRes, err := s.processEnvProxyUpdate(
					ctx, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName,
				)
				if err != nil {
					s.rollbackProxies(ctx, rollbackResources, orgName)
					return nil, err
				}
				if rbRes.providerAPIKeyID != "" {
					rollbackResources = append(rollbackResources, rbRes)
				}
			}
			delete(existingEnvMap, envName)
		} else {
			// Scenario C: new environment — create proxy and mapping.
			rbRes, err := s.processNewEnv(
				ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, orgName, existingVarNames, isExternalAgent, firstEnvName,
			)
			if err != nil {
				s.rollbackProxies(ctx, rollbackResources, orgName)
				return nil, err
			}
			rollbackResources = append(rollbackResources, rbRes)
		}
	}

	// Phase 4 — Remove environments not in the request (Scenario D).
	// survivingEnvCount is the number of environments that will remain after all
	// removals — used to decide whether to clear the Component CR.
	survivingEnvCount := len(req.EnvMappings)
	for _, mapping := range existingEnvMap {
		if mapping.LLMProxy != nil {
			proxiesToDelete = append(proxiesToDelete, mapping.LLMProxy.Handle)
		}
		removedEnvName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		isLastEnv := survivingEnvCount == 0
		if err := s.processEnvRemoval(ctx, configUUID, mapping.EnvironmentUUID.String(), mapping, existingConfig.Name, removedEnvName, orgName, projectName, agentName, isExternalAgent, existingVarNames, isLastEnv); err != nil {
			// HIGH-6: Phase 2-3 DB changes are already committed. Log enough information for manual reconciliation.
			s.logger.Error(
				"Partial update failure — manual reconciliation required",
				"configUUID", configUUID,
				"action", "manual_cleanup_required",
				"failedAtEnv", mapping.EnvironmentUUID.String(),
				"error", err,
			)
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, err
		}
	}

	// Phase 5 — Post-success proxy cleanup (outside any transaction, best effort).
	cleanupErrors := 0
	for _, proxyHandle := range proxiesToDelete {
		s.logger.Info("Cleaning up replaced proxy", "proxyHandle", proxyHandle)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err != nil {
			s.logger.Error(
				"Failed to get deployments for proxy cleanup",
				"proxyHandle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		} else {
			for _, dep := range deployments {
				if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), orgName); err != nil {
					s.logger.Error(
						"Failed to delete deployment during cleanup",
						"proxyHandle", proxyHandle,
						"deploymentID", dep.DeploymentID,
						"error", err,
					)
					cleanupErrors++
				}
			}
		}

		if err := s.llmProxyService.Delete(proxyHandle, orgName); err != nil {
			s.logger.Error(
				"Failed to delete proxy during cleanup",
				"proxyHandle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		}
	}

	if cleanupErrors > 0 {
		s.logger.Warn(
			"Cleanup completed with errors",
			"totalProxies", len(proxiesToDelete),
			"errors", cleanupErrors,
		)
	}

	// Audit log for configuration update
	s.logger.Info(
		"Agent configuration updated successfully",
		"configUUID", configUUID,
		"orgName", orgName,
		"updatedFields", func() []string {
			fields := []string{}
			if req.Name != "" {
				fields = append(fields, "name")
			}
			if req.Description != "" {
				fields = append(fields, "description")
			}
			if req.EnvMappings != nil {
				fields = append(fields, "envMappings")
			}
			return fields
		}(),
	)

	// Return updated configuration
	return s.Get(ctx, configUUID, orgName, projectName, agentName)
}

// DeleteMCP deletes an MCP proxy mapping and all associated resources.
func (s *agentConfigurationService) DeleteMCP(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get MCP configuration: %w", err)
	}
	if config.ProjectName != projectName || config.AgentID != agentName || config.TypeID != models.AgentConfigTypeIDMCP {
		return utils.ErrAgentConfigNotFound
	}
	return s.deleteMCPConfig(ctx, config, orgName, projectName, agentName)
}

// Delete deletes a configuration and all associated resources with project and agent scoping validation
func (s *agentConfigurationService) Delete(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error {
	// Get configuration and mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return utils.ErrAgentConfigNotFound
	}

	switch existingConfig.TypeID {
	case models.AgentConfigTypeIDMCP:
		return s.deleteMCPConfig(ctx, existingConfig, orgName, projectName, agentName)
	default:
		return s.deleteLLMConfig(ctx, existingConfig, orgName, projectName, agentName)
	}
}

func (s *agentConfigurationService) deleteLLMConfig(ctx context.Context, existingConfig *models.AgentConfiguration, orgName, projectName, agentName string) error {
	configUUID := existingConfig.UUID

	// Determine agent type for internal-agent cleanup decisions.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)

	s.logger.Info("Deleting agent configuration", "configUUID", existingConfig.UUID, "name", existingConfig.Name)

	// Get all environment mappings
	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	environments, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return fmt.Errorf("error while list environments from open choreo. %w", err)
	}

	envIDNameMap := make(map[string]string)

	for _, env := range environments {
		envIDNameMap[env.UUID] = env.Name
	}

	// Steps 1-4: Per-mapping cleanup in strict order before DB deletion.
	// External resources are cleaned up before DB deletion so that if any step fails,
	// the DB row remains and the caller can retry. On retry, already-deleted external
	// resources are skipped gracefully.
	// Order matters: revoke API keys (1) before undeploying (2) so the gateway still has
	// the proxy config when it processes the revocation event.
	//
	// Key names mirror the naming convention used during Create/buildLLMProxyConfig:
	//   proxyHandle       = "{configPrefix}-{hash}-proxy"  (= Configuration.Name)
	//   proxy API key     = "{configPrefix}-{hash}-key"
	//   provider API key  = "{configPrefix}-{hash}-proxy"  (= proxyHandle)
	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
		if !ok {
			s.logger.Warn("environment is not available in openchoreo")
			continue
		}

		// Configuration.Name = proxyHandle = "{configPrefix}-{hash}-proxy".
		// Use it directly as the proxy handle (Handle field is gorm:"-" and not populated by Preload).
		proxyHandle := mapping.LLMProxy.Configuration.Name

		// Step 1: Revoke API keys (must happen before undeployment so the gateway still has
		// the proxy config when it processes the revocation event).
		proxyKeyName := fmt.Sprintf("%s-key", strings.TrimSuffix(proxyHandle, "-proxy"))
		providerKeyName := proxyHandle

		s.logger.Info("Revoking API keys", "proxyHandle", proxyHandle, "proxyKeyName", proxyKeyName, "providerKeyName", providerKeyName)

		if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, proxyHandle, proxyKeyName); err != nil {
			s.logger.Warn(
				"Failed to revoke proxy API key during deletion (best-effort)",
				"proxyHandle", proxyHandle,
				"keyName", proxyKeyName,
				"error", err,
			)
		}

		// Revoke provider API key (only if provider auth was configured).
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, providerUUID, providerKeyName); err != nil {
				s.logger.Warn(
					"Failed to revoke provider API key during deletion (best-effort)",
					"providerUUID", providerUUID,
					"keyName", providerKeyName,
					"error", err,
				)
			}
		}

		// Load the persisted SecretReference name from DB. This is the name returned by the
		// secret management system at creation time (e.g., "cred-wc-..." from the Secret Manager API)
		// and must be used instead of recomputing via SecretRefName() which may produce a different name.
		var persistedSecretRefName string
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, mapping.EnvironmentUUID)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for SecretReference lookup on delete", "err", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					persistedSecretRefName = v.SecretReference
					break
				}
			}
		}
		if persistedSecretRefName == "" {
			s.logger.Warn("no persisted SecretReference found for config, skipping SecretReference deletion",
				"configUUID", configUUID, "environment", env)
		}

		// Step 1b: Delete SecretReference CR (internal agents only, best-effort).
		if !isExternalAgent && persistedSecretRefName != "" {
			s.logger.Info("Delete: using persisted SecretReference for deletion",
				"secretRef", persistedSecretRefName,
				"configUUID", configUUID, "environment", env)
			if err := s.ocClient.DeleteSecretReference(ctx, orgName, persistedSecretRefName); err != nil {
				s.logger.Warn("failed to delete SecretReference on config delete",
					"name", persistedSecretRefName, "err", err)
			}
		}

		// Step 2: Undeploy proxy deployments.
		s.logger.Info(
			"Cleaning up proxy deployments for deleted config",
			"configUUID", configUUID,
			"proxyHandle", proxyHandle,
		)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err != nil {
			if errors.Is(err, utils.ErrLLMProxyNotFound) {
				// Proxy already gone — skip deployment cleanup for this mapping.
				s.logger.Info(
					"Proxy already deleted, skipping deployment cleanup",
					"proxyHandle", proxyHandle,
				)
			} else {
				return fmt.Errorf("failed to get deployments for proxy %q: %w", proxyHandle, err)
			}
		} else {
			for _, dep := range deployments {
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), orgName); err != nil {
					s.logger.Error(
						"Failed to undeploy deployment during cleanup",
						"proxyHandle", proxyHandle,
						"deploymentID", dep.DeploymentID,
						"gatewayID", dep.GatewayUUID,
						"error", err,
					)
				}
			}
		}

		// Step 3: Delete proxy record.
		if err := s.llmProxyService.Delete(proxyHandle, orgName); err != nil {
			// ErrLLMProxyNotFound means already deleted — treat as success.
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				return fmt.Errorf("failed to delete proxy %q: %w", proxyHandle, err)
			}
			s.logger.Info("Proxy already deleted, skipping", "proxyHandle", proxyHandle)
		}

		// Delete proxy API key secret
		// Step 4: Delete KV secrets for proxy API key (used by SecretReference CR).
		// Note: provider upstream auth is encrypted in the DB and deleted with the proxy record.
		// SecretReference CR is already deleted in Step 1b above, so we pass the persisted name
		// to avoid a redundant (and potentially incorrect) deletion attempt.
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         existingConfig.OrganizationName,
			ProjectName:     existingConfig.ProjectName,
			AgentName:       existingConfig.AgentID,
			EnvironmentName: env,
			ConfigName:      existingConfig.Name,
			EntityName:      proxyHandle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		// Use persisted name when available; fall back to computed name so the
		// KV secret deletion (location-based) still proceeds and DeleteSecret
		// receives a valid SecretReference name for its internal cleanup.
		secretRefForDelete := persistedSecretRefName
		if secretRefForDelete == "" {
			secretRefForDelete = proxySecretLoc.SecretRefName()
		}
		if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
			return fmt.Errorf("failed to delete proxy API key from KV for proxy %q: %w",
				proxyHandle, err)
		}
	}

	// Step 4b: Remove env vars from Component CR and all ReleaseBindings (internal agents only, best-effort).
	// Must use names from DB (not auto-generated) to handle user-overridden names correctly.
	if !isExternalAgent {
		existingVarNames, varErr := s.loadExistingVarNames(ctx, configUUID)
		if varErr != nil {
			s.logger.Warn("failed to load var names for cleanup, skipping env var removal", "err", varErr)
		} else {
			envConfigTemplates, _ := s.buildEnvironmentVariables(existingConfig.Name, varNamesToOverrides(existingVarNames))
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from Component CR.
			if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove env vars from Component CR on config delete", "err", err)
			}
			// Remove from Workload (live runtime resource) so stale env vars don't persist
			// and get re-injected by getSystemManagedEnvVars on the next deploy.
			if err := s.ocClient.RemoveWorkloadEnvVars(ctx, orgName, agentName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove env vars from Workload on config delete", "err", err)
			}
			// Remove from each environment's ReleaseBinding.
			for _, mapping := range mappings {
				env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
				if !ok {
					continue
				}
				if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, env, keysToRemove); err != nil {
					s.logger.Warn("failed to remove env vars from ReleaseBinding on config delete",
						"environment", env, "err", err)
				}
			}
		}
	}

	// Step 5: Delete DB records only after all external resources are confirmed cleaned up.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete configuration (cascades to mappings and variables)
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName); err != nil {
			return fmt.Errorf("failed to delete configuration: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Audit log for configuration deletion
	s.logger.Info(
		"Agent configuration deleted successfully",
		"configUUID", configUUID,
		"configName", existingConfig.Name,
		"orgName", orgName,
		"environmentCount", len(mappings),
	)

	return nil
}

// DeleteForAgentDeletion cleans up all external proxy resources for a single LLM config as part
// of agent deletion. Compared to Delete, it skips:
//   - GetComponent (caller resolves isExternalAgent once for all configs)
//   - SecretReference CR deletion (component teardown handles it)
//   - Component/Workload/ReleaseBinding env-var patching (component is being deleted)
//
// Steps retained: revoke API keys → undeploy proxy deployments → delete proxy record → delete KV secret → delete DB record.
// Best-effort: individual step failures are logged but do not abort the overall agent deletion.
func (s *agentConfigurationService) DeleteForAgentDeletion(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string, isExternalAgent bool) error {
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return utils.ErrAgentConfigNotFound
	}

	if existingConfig.TypeID == models.AgentConfigTypeIDMCP {
		return s.deleteMCPConfigForAgentDeletion(ctx, existingConfig, orgName)
	}

	s.logger.Info("Deleting agent configuration for agent deletion", "configUUID", existingConfig.UUID, "name", existingConfig.Name)

	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	environments, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return fmt.Errorf("failed to list environments: %w", err)
	}
	envIDNameMap := make(map[string]string, len(environments))
	for _, env := range environments {
		envIDNameMap[env.UUID] = env.Name
	}

	var cleanupErrs []string
	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
		if !ok {
			s.logger.Warn("environment not available in openchoreo, skipping mapping", "environmentUUID", mapping.EnvironmentUUID)
			continue
		}

		proxyHandle := mapping.LLMProxy.Configuration.Name
		proxyKeyName := fmt.Sprintf("%s-key", strings.TrimSuffix(proxyHandle, "-proxy"))
		providerKeyName := proxyHandle

		// Step 1: Revoke proxy API key. ErrLLMProxyNotFound means already gone — idempotent.
		if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, proxyHandle, proxyKeyName); err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to revoke proxy API key during agent deletion",
					"proxyHandle", proxyHandle, "keyName", proxyKeyName, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("revoke proxy key %s: %v", proxyKeyName, err))
			}
		}

		// Step 2: Revoke provider API key (only if provider auth was configured).
		// ErrLLMProviderNotFound means already gone — idempotent.
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, providerUUID, providerKeyName); err != nil {
				if !errors.Is(err, utils.ErrLLMProviderNotFound) {
					s.logger.Warn("Failed to revoke provider API key during agent deletion",
						"providerUUID", providerUUID, "keyName", providerKeyName, "error", err)
					cleanupErrs = append(cleanupErrs, fmt.Sprintf("revoke provider key %s: %v", providerKeyName, err))
				}
			}
		}

		// Step 3: Undeploy proxy deployments.
		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to get proxy deployments during agent deletion",
					"proxyHandle", proxyHandle, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("get deployments for proxy %s: %v", proxyHandle, err))
			}
		} else {
			for _, dep := range deployments {
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), orgName); err != nil {
					s.logger.Warn("Failed to undeploy proxy deployment during agent deletion",
						"proxyHandle", proxyHandle, "deploymentID", dep.DeploymentID, "error", err)
					cleanupErrs = append(cleanupErrs, fmt.Sprintf("undeploy %s deployment %s: %v", proxyHandle, dep.DeploymentID, err))
				}
			}
		}

		// Step 4: Delete proxy record.
		if err := s.llmProxyService.Delete(proxyHandle, orgName); err != nil {
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				s.logger.Warn("Failed to delete proxy record during agent deletion",
					"proxyHandle", proxyHandle, "error", err)
				cleanupErrs = append(cleanupErrs, fmt.Sprintf("delete proxy %s: %v", proxyHandle, err))
			}
		}

		// Step 5: Delete KV secret for proxy API key. Load the persisted SecretReference name
		// from DB so DeleteSecret receives the correct name even if recomputation would differ.
		var persistedSecretRefName string
		vars, varLoadErr := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, mapping.EnvironmentUUID)
		if varLoadErr != nil {
			s.logger.Warn("failed to load env config variables for KV secret deletion", "err", varLoadErr)
		} else {
			for _, v := range vars {
				if v.SecretReference != "" {
					persistedSecretRefName = v.SecretReference
					break
				}
			}
		}

		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         existingConfig.OrganizationName,
			ProjectName:     existingConfig.ProjectName,
			AgentName:       existingConfig.AgentID,
			EnvironmentName: env,
			ConfigName:      existingConfig.Name,
			EntityName:      proxyHandle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		secretRefForDelete := persistedSecretRefName
		if secretRefForDelete == "" {
			secretRefForDelete = proxySecretLoc.SecretRefName()
		}
		if err := s.secretClient.DeleteSecret(ctx, proxySecretLoc, secretRefForDelete); err != nil {
			s.logger.Warn("Failed to delete proxy API key from KV during agent deletion",
				"proxyHandle", proxyHandle, "error", err)
			cleanupErrs = append(cleanupErrs, fmt.Sprintf("delete KV secret for proxy %s: %v", proxyHandle, err))
		}
	}

	// Step 6: Delete DB record only when all external resources were cleaned up successfully.
	// If any step above failed, return an error so the DB row is preserved and the caller
	// (deleteAgentLLMConfigurations) can log it — the row will be retried on the next
	// agent deletion attempt via the idempotent delete path.
	if len(cleanupErrs) > 0 {
		return fmt.Errorf("external cleanup incomplete for config %s, DB record preserved for retry: %s",
			configUUID, strings.Join(cleanupErrs, "; "))
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName)
	}); err != nil {
		return fmt.Errorf("failed to delete configuration from DB: %w", err)
	}

	s.logger.Info("Agent configuration deleted for agent deletion",
		"configUUID", configUUID, "configName", existingConfig.Name, "orgName", orgName)

	return nil
}

func (s *agentConfigurationService) deleteMCPConfig(ctx context.Context, existingConfig *models.AgentConfiguration, orgName, projectName, agentName string) error {
	if s.envMCPMappingRepo == nil {
		return fmt.Errorf("MCP configuration repository is not configured")
	}
	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, existingConfig.UUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP environment mappings: %w", err)
	}

	envs, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return fmt.Errorf("error while list environments from open choreo. %w", err)
	}
	envIDNameMap := make(map[string]string, len(envs))
	for _, env := range envs {
		envIDNameMap[env.UUID] = env.Name
	}

	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	if !isExternalAgent {
		existingVars, varErr := s.envVariableRepo.ListByConfig(ctx, existingConfig.UUID)
		if varErr != nil {
			s.logger.Warn("failed to load MCP var names for cleanup, skipping env var removal", "err", varErr)
		} else {
			componentKeysToRemove := uniqueVariableNames(existingVars)
			if len(componentKeysToRemove) > 0 {
				if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, componentKeysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Component CR on config delete", "err", err)
				}
				if err := s.ocClient.RemoveWorkloadEnvVars(ctx, orgName, agentName, componentKeysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from Workload on config delete", "err", err)
				}
			}
			for _, mapping := range mappings {
				envName := envIDNameMap[mapping.EnvironmentUUID.String()]
				if envName == "" {
					continue
				}
				keysToRemove := variableNamesForEnv(existingVars, mapping.EnvironmentUUID)
				if len(keysToRemove) == 0 {
					continue
				}
				if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, keysToRemove); err != nil {
					s.logger.Warn("failed to remove MCP env vars from ReleaseBinding on config delete",
						"environment", envName, "err", err)
				}
			}
		}
	}

	for _, mapping := range mappings {
		if s.mcpProxyService != nil {
			s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, orgName)
		}
		envName := envIDNameMap[mapping.EnvironmentUUID.String()]
		s.cleanupMCPMappingCredentials(ctx, existingConfig, &mapping, envName, orgName)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Delete(ctx, tx, existingConfig.UUID, orgName); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete MCP configuration: %w", err)
	}
	return nil
}

func (s *agentConfigurationService) deleteMCPConfigForAgentDeletion(ctx context.Context, existingConfig *models.AgentConfiguration, orgName string) error {
	if s.envMCPMappingRepo == nil {
		return fmt.Errorf("MCP configuration repository is not configured")
	}
	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, existingConfig.UUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP environment mappings: %w", err)
	}
	envs, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		s.logger.Warn("failed to list environments for MCP credential cleanup during agent deletion", "err", err)
	}
	envIDNameMap := make(map[string]string, len(envs))
	for _, env := range envs {
		envIDNameMap[env.UUID] = env.Name
	}

	for _, mapping := range mappings {
		if s.mcpProxyService != nil {
			s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, orgName)
		}
		envName := envIDNameMap[mapping.EnvironmentUUID.String()]
		s.cleanupMCPMappingCredentials(ctx, existingConfig, &mapping, envName, orgName)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Delete(ctx, tx, existingConfig.UUID, orgName); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to delete MCP configuration for agent deletion: %w", err)
	}

	s.logger.Info("MCP agent configuration deleted for agent deletion",
		"configUUID", existingConfig.UUID, "configName", existingConfig.Name, "orgName", orgName)
	return nil
}

// Helper methods

// resolveGatewayForProvider looks up the gateway where the given LLM provider is deployed.
// This ensures the proxy is deployed to the same gateway as its provider.
// Falls back to resolveGatewayForEnvironment if the provider has no active deployments.
func (s *agentConfigurationService) resolveGatewayForProvider(ctx context.Context, providerUUIDStr string, orgName string, envUUID uuid.UUID) (*models.Gateway, error) {
	providerUUID, err := uuid.Parse(providerUUIDStr)
	if err != nil {
		s.logger.Warn("Invalid provider UUID, falling back to environment resolution",
			"providerUUID", providerUUIDStr, "error", err)
		return s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	}

	gatewayIDs, err := s.llmProxyDeploymentService.GetDeployedGatewaysByProvider(providerUUID, orgName)
	if err == nil && len(gatewayIDs) > 0 {
		envIDStr := envUUID.String()
		// Prefer a gateway that is mapped to the target environment
		for _, gwID := range gatewayIDs {
			exists, mapErr := s.gatewayRepo.EnvironmentMappingExists(gwID, envIDStr)
			if mapErr != nil || !exists {
				continue
			}
			gw, gwErr := s.gatewayRepo.GetByUUID(gwID)
			if gwErr == nil && gw != nil {
				return gw, nil
			}
		}
		// No environment-matched gateway; try first as fallback
		gw, gwErr := s.gatewayRepo.GetByUUID(gatewayIDs[0])
		if gwErr == nil && gw != nil {
			return gw, nil
		}
		s.logger.Warn("Gateway not found for provider deployment, falling back to environment resolution",
			"providerUUID", providerUUID, "gatewayUUID", gatewayIDs[0], "error", gwErr)
	}

	return s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
}

// resolveGatewayForProxy looks up the gateway that a proxy is actually deployed to.
// This avoids the bug where resolveGatewayForEnvironment picks the wrong gateway
// when multiple AI gateways are mapped to the same environment.
// Falls back to resolveGatewayForEnvironment if no active deployment is found.
func (s *agentConfigurationService) resolveGatewayForProxy(ctx context.Context, proxyHandle, orgName string, envUUID uuid.UUID) (*models.Gateway, error) {
	deployedStatus := string(models.DeploymentStatusDeployed)
	deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, &deployedStatus)
	if err == nil && len(deployments) > 0 {
		envIDStr := envUUID.String()
		// Find the deployment whose gateway is mapped to the target environment
		for _, dep := range deployments {
			gwUUID := dep.GatewayUUID.String()
			exists, mapErr := s.gatewayRepo.EnvironmentMappingExists(gwUUID, envIDStr)
			if mapErr != nil || !exists {
				continue
			}
			gw, gwErr := s.gatewayRepo.GetByUUID(gwUUID)
			if gwErr == nil && gw != nil {
				return gw, nil
			}
		}
		// No environment-matched deployment found; try first deployment as fallback
		gw, gwErr := s.gatewayRepo.GetByUUID(deployments[0].GatewayUUID.String())
		if gwErr == nil && gw != nil {
			return gw, nil
		}
		s.logger.Warn("Gateway not found for proxy deployment, falling back to environment resolution",
			"proxyHandle", proxyHandle, "gatewayUUID", deployments[0].GatewayUUID, "error", gwErr)
	}

	return s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
}

func (s *agentConfigurationService) resolveGatewayForMCPArtifact(ctx context.Context, artifactUUID uuid.UUID, orgName string, envUUID uuid.UUID) (*models.Gateway, error) {
	_ = ctx
	if s.mcpProxyService != nil && s.mcpProxyService.deploymentRepo != nil {
		gatewayIDs, err := s.mcpProxyService.deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, orgName)
		if err == nil && len(gatewayIDs) > 0 {
			envIDStr := envUUID.String()
			for _, gwID := range gatewayIDs {
				exists, mapErr := s.gatewayRepo.EnvironmentMappingExists(gwID, envIDStr)
				if mapErr != nil || !exists {
					continue
				}
				gw, gwErr := s.gatewayRepo.GetByUUID(gwID)
				if gwErr == nil && gw != nil {
					return gw, nil
				}
			}
			gw, gwErr := s.gatewayRepo.GetByUUID(gatewayIDs[0])
			if gwErr == nil && gw != nil {
				return gw, nil
			}
		}
	}
	return s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
}

// resolveGatewayForEnvironment selects gateway with AI-first preference
func (s *agentConfigurationService) resolveGatewayForEnvironment(ctx context.Context, envUUID uuid.UUID, orgName string) (*models.Gateway, error) {
	envIDStr := envUUID.String()
	aiType := "ai"
	activeStatus := true

	// Try AI gateway first
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID:    orgName,
		FunctionalityType: &aiType,
		Status:            &activeStatus,
		EnvironmentID:     &envIDStr,
		Limit:             1,
	})
	if err == nil && len(gateways) > 0 {
		return gateways[0], nil
	}

	// Fallback to any active gateway
	gateways, err = s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgName,
		Status:         &activeStatus,
		EnvironmentID:  &envIDStr,
		Limit:          1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway: %w", err)
	}
	if len(gateways) == 0 {
		return nil, errors.New("no active gateway found for environment")
	}

	return gateways[0], nil
}

// buildLLMProxyConfig constructs proxy configuration from request.
// Returns the proxy config, provider API key ID, provider UUID, provider secret KV path, and any error.
// The provider UUID is needed by rollbackProxies to revoke the provider API key on failure.
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envName string,
	envMapping models.EnvModelConfigRequest,
) (*models.LLMProxy, string, string, *secretmanagersvc.SecretLocation, error) {
	scopedID := scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName)
	proxyName := fmt.Sprintf("%s-proxy", scopedID)
	contextPath := fmt.Sprintf("/%s", scopedID)

	project, err := s.ocClient.GetProject(ctx, config.OrganizationName, config.ProjectName)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("failed to get project from openchoreo: %w", err)
	}

	// Get provider details
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OrganizationName)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("failed to get provider: %w", err)
	}

	apiKeyId := ""
	providerUUID := provider.UUID.String()
	var providerSecretLoc *secretmanagersvc.SecretLocation

	// Parse project UUID
	projectUUID, err := uuid.Parse(project.UUID)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("invalid project UUID from openchoreo: %w", err)
	}

	enabled := true
	// Build proxy configuration
	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: projectUUID,
		Configuration: models.LLMProxyConfig{
			Name:     proxyName,
			Version:  models.DefaultProxyVersion,
			Context:  &contextPath,
			Provider: provider.UUID.String(),
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     "API-Key",
					In:      "header",
				},
			},
			Policies: envMapping.Configuration.Policies,
		},
	}

	var upstreamAuthConfig models.UpstreamAuth

	providerSecurityConfig := provider.Configuration.Security
	if providerSecurityConfig != nil && providerSecurityConfig.Enabled != nil && *providerSecurityConfig.Enabled {
		// Provider is secured.
		providerApiKeyConfig := providerSecurityConfig.APIKey

		if providerApiKeyConfig != nil && providerApiKeyConfig.Enabled != nil && *providerApiKeyConfig.Enabled {
			// Provider api key security is enabled.
			apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OrganizationName, provider.UUID.String(), &models.CreateAPIKeyRequest{
				Name:        proxyName,
				DisplayName: proxyName,
			})
			s.logger.Info("Created provider API key", "providerUUID", provider.UUID.String(), "providerKeyName", proxyName)
			if err != nil {
				return nil, "", "", nil, fmt.Errorf("failed to create api key for provider: %w", err)
			}

			apiKeyId = apiKey.KeyID

			// Encrypt the provider API key for storage in UpstreamAuth.SecretRef
			encrypted, err := utils.EncryptBytes([]byte(apiKey.APIKey), s.encryptionKey)
			if err != nil {
				// revoke created api key
				if revokeErr := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, config.OrganizationName, provider.UUID.String(), proxyName); revokeErr != nil {
					s.logger.Error(
						"Failed to revoke provider API key after encryption failure",
						"providerUUID", provider.UUID.String(),
						"providerKeyName", proxyName,
						"error", revokeErr,
					)
				}
				return nil, "", "", nil, fmt.Errorf("failed to encrypt provider API key: %w", err)
			}
			encoded := base64.StdEncoding.EncodeToString(encrypted)
			upstreamAuthConfig.Type = utils.StrAsStrPointer(models.AuthTypeAPIKey)
			upstreamAuthConfig.Header = utils.StrAsStrPointer(providerApiKeyConfig.Key)
			upstreamAuthConfig.SecretRef = &encoded // Store encrypted value instead of plaintext
			upstreamAuthConfig.Value = nil          // No plaintext in DB
			proxyConfig.Configuration.UpstreamAuth = &upstreamAuthConfig
		}
	}

	return proxyConfig, apiKeyId, providerUUID, providerSecretLoc, nil
}

// buildLLMProxyUpdateConfig builds a proxy config for the Update flow (Scenario B).
// It preserves the existing proxy's Name, Context, Security, and ProjectUUID —
// only mutable fields (Provider, UpstreamAuth, Policies) are updated.
func (s *agentConfigurationService) buildLLMProxyUpdateConfig(
	config *models.AgentConfiguration,
	envMapping models.EnvModelConfigRequest,
	existingProxy *models.LLMProxy,
) (*models.LLMProxy, string, error) {
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OrganizationName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get provider: %w", err)
	}
	providerUUID := provider.UUID.String()

	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: existingProxy.ProjectUUID,
		Configuration: models.LLMProxyConfig{
			Name:         existingProxy.Configuration.Name,
			Version:      models.DefaultProxyVersion,
			Context:      existingProxy.Configuration.Context,
			Provider:     provider.UUID.String(),
			Security:     existingProxy.Configuration.Security,
			Policies:     envMapping.Configuration.Policies,
			UpstreamAuth: existingProxy.Configuration.UpstreamAuth,
		},
	}

	return proxyConfig, providerUUID, nil
}

// buildEnvironmentVariables generates environment variable templates from config name.
// If overrides are provided, user-supplied names take precedence over auto-generated ones.
// Validates all names using ValidateEnvironmentVariableName.
func (s *agentConfigurationService) buildEnvironmentVariables(configName string, overrides []models.EnvironmentVariableConfig) ([]EnvConfigTemplate, error) {
	// Sanitize: Replace any character not in A-Za-z0-9_ with '_'
	prefix := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, configName)

	// Convert to uppercase
	prefix = strings.ToUpper(prefix)

	// If prefix starts with a digit, prepend underscore
	if len(prefix) > 0 && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "_" + prefix
	}

	// Known keys with their secrets flag and auto-generated name
	type keyMeta struct {
		isSecret bool
		autoName string
	}
	knownKeys := map[string]keyMeta{
		"url":    {isSecret: false, autoName: fmt.Sprintf("%s_URL", prefix)},
		"apikey": {isSecret: true, autoName: fmt.Sprintf("%s_API_KEY", prefix)},
	}

	// Build override map from user input; reject unknown keys
	overrideMap := make(map[string]string)
	seen := make(map[string]bool)
	for _, ov := range overrides {
		if _, known := knownKeys[ov.Key]; !known {
			return nil, fmt.Errorf("unknown environment variable key %q: must be one of url, apikey", ov.Key)
		}
		if seen[ov.Key] {
			return nil, fmt.Errorf("duplicate environment variable key %q", ov.Key)
		}
		seen[ov.Key] = true
		overrideMap[ov.Key] = ov.Name
	}

	// Determine final name for each key (override wins, then auto-generate).
	// Iterate in a fixed order so the returned slice is deterministic.
	keyOrder := []string{"url", "apikey"}
	envConfigTemplates := make([]EnvConfigTemplate, 0, len(knownKeys))
	usedNames := make(map[string]string) // name -> key, for duplicate detection
	for _, key := range keyOrder {
		meta := knownKeys[key]
		name := meta.autoName
		if customName, ok := overrideMap[key]; ok {
			name = customName
		}
		if err := utils.ValidateEnvironmentVariableName(name); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %q for key %q: %w", name, key, err)
		}
		if conflictKey, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable name %q for keys %q and %q", name, conflictKey, key)
		}
		usedNames[name] = key
		envConfigTemplates = append(envConfigTemplates, EnvConfigTemplate{
			Key:             key,
			Name:            name,
			IsSecret:        meta.isSecret,
			Value:           "",
			SecretReference: "",
		})
	}

	return envConfigTemplates, nil
}

// buildMCPMappingEnvironmentVariables produces the URL and API key env var templates for an MCP mapping.
func (s *agentConfigurationService) buildMCPMappingEnvironmentVariables(mappingName string, overrides []models.EnvironmentVariableConfig) ([]EnvConfigTemplate, error) {
	prefix := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, mappingName)
	prefix = strings.ToUpper(prefix)
	if len(prefix) > 0 && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "_" + prefix
	}

	type keyMeta struct {
		isSecret bool
		autoName string
	}
	knownKeys := map[string]keyMeta{
		"url":    {isSecret: false, autoName: fmt.Sprintf("%s_URL", prefix)},
		"apikey": {isSecret: true, autoName: fmt.Sprintf("%s_API_KEY", prefix)},
	}
	overrideMap := make(map[string]string)
	seen := make(map[string]bool)
	for _, ov := range overrides {
		if _, known := knownKeys[ov.Key]; !known {
			return nil, fmt.Errorf("unknown environment variable key %q for mapping %q: must be one of url, apikey", ov.Key, mappingName)
		}
		if seen[ov.Key] {
			return nil, fmt.Errorf("duplicate environment variable key %q", ov.Key)
		}
		seen[ov.Key] = true
		overrideMap[ov.Key] = ov.Name
	}

	keyOrder := []string{"url", "apikey"}
	envConfigTemplates := make([]EnvConfigTemplate, 0, len(keyOrder))
	usedNames := make(map[string]string)
	for _, key := range keyOrder {
		meta := knownKeys[key]
		name := meta.autoName
		if customName, ok := overrideMap[key]; ok {
			name = customName
		}
		if err := utils.ValidateEnvironmentVariableName(name); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %q for key %q: %w", name, key, err)
		}
		if conflictKey, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable name %q for keys %q and %q", name, conflictKey, key)
		}
		usedNames[name] = key
		envConfigTemplates = append(envConfigTemplates, EnvConfigTemplate{
			Key:      key,
			Name:     name,
			IsSecret: meta.isSecret,
		})
	}
	return envConfigTemplates, nil
}

func buildAgentMCPConfigProxy(
	config *models.AgentConfiguration,
	mapping *models.EnvAgentMCPMapping,
	source *models.MCPProxy,
	envName string,
	orgName string,
	handle string,
) *models.MCPProxy {
	context := agentMCPMappingContext(source.Configuration.Context, scopedProxyIdentifier(config.ProjectName, config.AgentID, config.Name, envName))
	name := handle
	version := source.Version
	if source.Artifact != nil && source.Artifact.Version != "" {
		version = source.Artifact.Version
	}
	if version == "" {
		version = source.Configuration.Version
	}
	return &models.MCPProxy{
		UUID:        mapping.ArtifactUUID,
		Description: config.Description,
		Status:      source.Status,
		Configuration: models.MCPProxyConfig{
			Name:         name,
			Version:      version,
			Context:      &context,
			Vhost:        source.Configuration.Vhost,
			SpecVersion:  source.Configuration.SpecVersion,
			Upstream:     source.Configuration.Upstream,
			Policies:     source.Configuration.Policies,
			Capabilities: source.Configuration.Capabilities,
			Security:     source.Configuration.Security,
		},
		OrganizationName: orgName,
		ID:               handle,
		Name:             name,
		Handle:           handle,
		Version:          version,
	}
}

func buildMCPProxyMapping(sourceProxyUUID uuid.UUID, deployedProxy *models.MCPProxy) *models.MCPProxyMapping {
	return &models.MCPProxyMapping{
		UUID:               deployedProxy.UUID,
		SourceMCPProxyUUID: sourceProxyUUID,
		Description:        deployedProxy.Description,
		Status:             deployedProxy.Status,
		Configuration:      deployedProxy.Configuration,
	}
}

// RedeployMCPMappingsForSourceProxy redeploys every agent-scoped MCP mapping artifact
// that derives from the given source MCP proxy. Invoked by MCPProxyService.Update after
// the source proxy itself has been redeployed, so each agent-specific artifact picks up
// the new upstream URL, auth header/value, and policies on the gateway(s) where it
// currently lives. Best-effort: per-mapping failures are aggregated and returned so the
// caller can log without rolling back the proxy update.
func (s *agentConfigurationService) RedeployMCPMappingsForSourceProxy(ctx context.Context, source *models.MCPProxy, orgName string) error {
	if source == nil || s.envMCPMappingRepo == nil || s.mcpProxyService == nil {
		return nil
	}

	mappings, err := s.envMCPMappingRepo.ListByMCPProxy(ctx, source.UUID)
	if err != nil {
		return fmt.Errorf("list MCP mappings: %w", err)
	}
	if len(mappings) == 0 {
		return nil
	}

	envs, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return fmt.Errorf("list environments: %w", err)
	}
	envNameByUUID := make(map[string]string, len(envs))
	for _, env := range envs {
		if env == nil {
			continue
		}
		envNameByUUID[env.UUID] = env.Name
	}

	var errs []error
	for i := range mappings {
		mapping := &mappings[i]
		envName, ok := envNameByUUID[mapping.EnvironmentUUID.String()]
		if !ok || envName == "" {
			errs = append(errs, fmt.Errorf("mapping %s: environment %s not found", mapping.ArtifactUUID, mapping.EnvironmentUUID))
			continue
		}

		config, err := s.agentConfigRepo.GetByUUID(ctx, mapping.ConfigUUID, orgName)
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: load config %s: %w", mapping.ArtifactUUID, mapping.ConfigUUID, err))
			continue
		}
		if config == nil {
			errs = append(errs, fmt.Errorf("mapping %s: config %s not found", mapping.ArtifactUUID, mapping.ConfigUUID))
			continue
		}
		agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, config.ProjectName, config.AgentID)
		isExternalAgent := false
		if agentErr != nil {
			s.logger.Warn("failed to determine agent type during MCP mapping redeploy, assuming internal", "configUUID", config.UUID, "err", agentErr)
		} else {
			isExternalAgent = agentComp.Provisioning.Type == string(utils.ExternalAgent)
		}
		firstEnvName := ""
		if !isExternalAgent {
			if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, config.ProjectName); pipelineErr == nil && pipeline != nil {
				firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
			}
		}
		existingVarNames, err := s.loadExistingVarNames(ctx, config.UUID)
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: load env var names: %w", mapping.ArtifactUUID, err))
			continue
		}
		envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: build env vars: %w", mapping.ArtifactUUID, err))
			continue
		}

		gateways, err := s.trackedGatewaysForMCPArtifact(mapping.ArtifactUUID, orgName)
		if err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: resolve gateways: %w", mapping.ArtifactUUID, err))
			continue
		}
		if len(gateways) == 0 {
			// Artifact has never been pushed to any gateway; nothing to refresh.
			s.logger.Info("Skipping MCP mapping redeploy; artifact has no tracked deployments",
				"mappingArtifactUUID", mapping.ArtifactUUID, "configUUID", config.UUID, "envName", envName)
			continue
		}

		handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
		derived := buildAgentMCPConfigProxy(config, mapping, source, envName, orgName, handle)
		if err := s.reconcileMCPMappingCredentials(ctx, config, mapping, source, envName, orgName, envTemplates, isExternalAgent, firstEnvName); err != nil {
			errs = append(errs, fmt.Errorf("mapping %s: reconcile credentials: %w", mapping.ArtifactUUID, err))
			continue
		}
		for _, gateway := range gateways {
			if err := s.mcpProxyService.deployMCPProxyToGateway(ctx, derived, orgName, gateway); err != nil {
				errs = append(errs, fmt.Errorf("mapping %s on gateway %s: %w", mapping.ArtifactUUID, gateway.UUID, err))
			}
		}
		if !isExternalAgent {
			if err := s.injectMCPMappingEnvVars(ctx, config, mapping, source, envName, orgName, envTemplates, firstEnvName); err != nil {
				s.logger.Warn("failed to inject redeployed MCP mapping env vars", "mappingArtifactUUID", mapping.ArtifactUUID, "environment", envName, "err", err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to redeploy %d MCP mapping artifact(s): %w", len(errs), errors.Join(errs...))
	}
	return nil
}

// trackedGatewaysForMCPArtifact resolves every gateway that has a deployment_status row
// for the given MCP mapping artifact — including UNDEPLOYED ones whose last deploy ack
// failed. We only redeploy where the artifact has previously been pushed; a brand-new
// gateway destination should go through the normal config-create path, not this cascade.
func (s *agentConfigurationService) trackedGatewaysForMCPArtifact(artifactUUID uuid.UUID, orgName string) ([]*models.Gateway, error) {
	if s.mcpProxyService == nil || s.mcpProxyService.deploymentRepo == nil {
		return nil, nil
	}
	gatewayIDs, err := s.mcpProxyService.deploymentRepo.GetTrackedGatewaysByProvider(artifactUUID, orgName)
	if err != nil {
		return nil, err
	}
	gateways := make([]*models.Gateway, 0, len(gatewayIDs))
	for _, gwID := range gatewayIDs {
		if strings.TrimSpace(gwID) == "" {
			continue
		}
		gw, err := s.gatewayRepo.GetByUUID(gwID)
		if err != nil || gw == nil {
			s.logger.Warn("Skipping tracked gateway for MCP mapping redeploy",
				"gatewayID", gwID, "artifactUUID", artifactUUID, "error", err)
			continue
		}
		gateways = append(gateways, gw)
	}
	return gateways, nil
}

// varNamesToOverrides converts a key→name map to a slice of EnvironmentVariableConfig.
// Used when passing existing DB names as overrides to buildEnvironmentVariables.
func varNamesToOverrides(names map[string]string) []models.EnvironmentVariableConfig {
	if len(names) == 0 {
		return nil
	}
	overrides := make([]models.EnvironmentVariableConfig, 0, len(names))
	for key, name := range names {
		overrides = append(overrides, models.EnvironmentVariableConfig{Key: key, Name: name})
	}
	return overrides
}

// loadExistingVarNames loads the variable key→name mapping from DB for a config.
// Names are config-level (identical across all environments). The first occurrence per key
// is used; a warning is logged if divergence is detected (indicates a data integrity problem).
func (s *agentConfigurationService) loadExistingVarNames(ctx context.Context, configUUID uuid.UUID) (map[string]string, error) {
	vars, err := s.envVariableRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing variable names: %w", err)
	}
	return s.variableNameMap(configUUID, vars), nil
}

func (s *agentConfigurationService) variableNameMap(configUUID uuid.UUID, vars []models.AgentEnvConfigVariable) map[string]string {
	result := make(map[string]string)
	for _, v := range vars {
		if existing, already := result[v.VariableKey]; already {
			if existing != v.VariableName {
				s.logger.Warn(
					"environment variable name diverged across environments — using first-occurrence value",
					"configUUID", configUUID,
					"key", v.VariableKey,
					"firstValue", existing,
					"divergedValue", v.VariableName,
				)
			}
		} else {
			result[v.VariableKey] = v.VariableName
		}
	}
	return result
}

func (s *agentConfigurationService) loadSecretRefForConfigEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (string, error) {
	vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, configUUID, envUUID)
	if err != nil {
		return "", fmt.Errorf("failed to load environment variables for SecretReference lookup: %w", err)
	}
	for _, v := range vars {
		if v.SecretReference != "" {
			return v.SecretReference, nil
		}
	}
	return "", nil
}

func (s *agentConfigurationService) updateMCPMappingSecretReference(ctx context.Context, configUUID, envUUID uuid.UUID, secretRefName string) error {
	result := s.db.WithContext(ctx).Model(&models.AgentEnvConfigVariable{}).
		Where("config_uuid = ? AND environment_uuid = ? AND variable_key = ?", configUUID, envUUID, "apikey").
		Update("secret_reference", secretRefName)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.AgentEnvConfigVariable{}).
		Where("config_uuid = ? AND environment_uuid = ? AND variable_key = ?", configUUID, envUUID, "apikey").
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("apikey environment variable row not found")
	}
	return nil
}

func (s *agentConfigurationService) removeMCPMappingAPIKeyEnvVar(ctx context.Context, config *models.AgentConfiguration, envName string, envTemplates []EnvConfigTemplate, firstEnvName string) {
	var keysToRemove []string
	for _, t := range envTemplates {
		if t.Key == "apikey" && strings.TrimSpace(t.Name) != "" {
			keysToRemove = append(keysToRemove, t.Name)
		}
	}
	if len(keysToRemove) == 0 {
		return
	}
	if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, config.OrganizationName, config.ProjectName, config.AgentID, envName, keysToRemove); err != nil {
		s.logger.Warn("failed to remove MCP API key env var from ReleaseBinding", "environment", envName, "err", err)
	}
	if firstEnvName != "" && envName == firstEnvName {
		if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, config.OrganizationName, config.ProjectName, config.AgentID, keysToRemove); err != nil {
			s.logger.Warn("failed to remove MCP API key env var from Component CR", "environment", envName, "err", err)
		}
	}
}

func (s *agentConfigurationService) ensureMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, orgName string) (string, error) {
	keyName := mcpMappingAPIKeyName(config, envName)
	secretRefName, err := s.loadSecretRefForConfigEnv(ctx, config.UUID, mapping.EnvironmentUUID)
	if err != nil {
		return "", err
	}
	keyExists, err := s.mcpMappingAPIKeyExists(mapping.ArtifactUUID, keyName)
	if err != nil {
		return "", fmt.Errorf("failed to inspect MCP API key for environment %s: %w", envName, err)
	}
	if secretRefName != "" && keyExists {
		if err := s.revokeStaleMCPMappingAPIKeys(ctx, orgName, mapping.ArtifactUUID, keyName); err != nil {
			return "", fmt.Errorf("failed to revoke stale MCP API keys for environment %s: %w", envName, err)
		}
		return secretRefName, nil
	}

	proxyAPIKey, err := s.createMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, keyName)
	if err != nil {
		return "", fmt.Errorf("failed to generate MCP API key for environment %s: %w", envName, err)
	}
	createdKeyName := proxyAPIKey.KeyID

	agentAppHandle := agentAppIdentifier(config.ProjectName, config.AgentID, envName)
	if _, _, err = s.aiApplicationService.EnsureAndBind(
		ctx, orgName, config.ProjectName, config.AgentID, envName,
		agentAppHandle,
		fmt.Sprintf("%s Application", config.AgentID),
		proxyAPIKey.KeyID,
	); err != nil {
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after AI application failure", "environment", envName, "err", revokeErr)
		}
		return "", fmt.Errorf("failed to ensure AI application for MCP environment %s: %w", envName, err)
	}

	secretLoc := mcpMappingSecretLocation(config, orgName, envName)
	newSecretRefName, err := s.secretClient.CreateSecret(ctx, secretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after secret persistence failure", "environment", envName, "err", revokeErr)
		}
		return "", fmt.Errorf("failed to store MCP API key in KV for environment %s: %w", envName, err)
	}

	if err := s.updateMCPMappingSecretReference(ctx, config.UUID, mapping.EnvironmentUUID, newSecretRefName); err != nil {
		if delErr := s.secretClient.DeleteSecret(ctx, secretLoc, newSecretRefName); delErr != nil {
			s.logger.Warn("failed to delete MCP API key secret after env var update failure", "environment", envName, "err", delErr)
		}
		if revokeErr := s.revokeMCPMappingAPIKey(ctx, orgName, mapping.ArtifactUUID, createdKeyName); revokeErr != nil {
			s.logger.Warn("failed to revoke MCP API key after env var update failure", "environment", envName, "err", revokeErr)
		}
		return "", fmt.Errorf("failed to update MCP API key env reference for %s: %w", envName, err)
	}
	if secretRefName != "" && secretRefName != newSecretRefName {
		s.logger.Info("MCP mapping SecretReference replaced", "environment", envName, "oldSecretRef", secretRefName, "newSecretRef", newSecretRefName)
	}
	if err := s.revokeStaleMCPMappingAPIKeys(ctx, orgName, mapping.ArtifactUUID, keyName); err != nil {
		return "", fmt.Errorf("failed to revoke stale MCP API keys for environment %s: %w", envName, err)
	}
	return newSecretRefName, nil
}

func (s *agentConfigurationService) reconcileMCPMappingCredentials(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, sourceProxy *models.MCPProxy, envName, orgName string, envTemplates []EnvConfigTemplate, isExternalAgent bool, firstEnvName string) error {
	if mcpProxyAPIKeySecurityEnabled(sourceProxy) {
		if _, err := s.ensureMCPMappingCredentials(ctx, config, mapping, envName, orgName); err != nil {
			return err
		}
		return nil
	}

	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, orgName)
	if err := s.updateMCPMappingSecretReference(ctx, config.UUID, mapping.EnvironmentUUID, ""); err != nil {
		return fmt.Errorf("failed to clear MCP API key env reference for %s: %w", envName, err)
	}
	if !isExternalAgent {
		s.removeMCPMappingAPIKeyEnvVar(ctx, config, envName, envTemplates, firstEnvName)
	}
	return nil
}

func (s *agentConfigurationService) cleanupNewMCPMapping(ctx context.Context, config *models.AgentConfiguration, mapping *models.EnvAgentMCPMapping, envName, orgName string) {
	if s.mcpProxyService != nil {
		s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, orgName)
	}
	s.cleanupMCPMappingCredentials(ctx, config, mapping, envName, orgName)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, config.UUID, mapping.EnvironmentUUID); err != nil {
			return err
		}
		if mapping.ID != 0 {
			if err := s.envMCPMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
				return err
			}
		}
		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error
	}); err != nil {
		s.logger.Warn("failed to clean up new MCP mapping after partial failure", "environment", envName, "artifactUUID", mapping.ArtifactUUID, "err", err)
	}
}

func uniqueVariableNames(vars []models.AgentEnvConfigVariable) []string {
	seen := make(map[string]struct{}, len(vars))
	names := make([]string, 0, len(vars))
	for _, v := range vars {
		if strings.TrimSpace(v.VariableName) == "" {
			continue
		}
		if _, ok := seen[v.VariableName]; ok {
			continue
		}
		seen[v.VariableName] = struct{}{}
		names = append(names, v.VariableName)
	}
	return names
}

func variableNamesForEnv(vars []models.AgentEnvConfigVariable, envUUID uuid.UUID) []string {
	filtered := make([]models.AgentEnvConfigVariable, 0, len(vars))
	for _, v := range vars {
		if v.EnvironmentUUID == envUUID {
			filtered = append(filtered, v)
		}
	}
	return uniqueVariableNames(filtered)
}

// dedupeEnvVariablesByKey collapses per-environment DB rows into config-level entries.
// Rows are stored per-environment but the variable name is config-level — agent source code
// reads url/apikey by the same env var name regardless of which provider is bound to a given
// environment. First occurrence per key wins.
func (s *agentConfigurationService) dedupeEnvVariablesByKey(configUUID uuid.UUID, vars []models.AgentEnvConfigVariable) []models.EnvironmentVariableConfig {
	seen := make(map[string]string)
	result := make([]models.EnvironmentVariableConfig, 0, len(vars))
	for _, v := range vars {
		if existing, already := seen[v.VariableKey]; already {
			if existing != v.VariableName {
				s.logger.Warn(
					"environment variable name differs across environments — using first-occurrence value",
					"configUUID", configUUID,
					"key", v.VariableKey,
					"firstValue", existing,
					"otherValue", v.VariableName,
				)
			}
			continue
		}
		seen[v.VariableKey] = v.VariableName
		result = append(result, models.EnvironmentVariableConfig{Name: v.VariableName, Key: v.VariableKey})
	}
	return result
}

// rollbackProxies cleans up created proxies, deployments, and API keys on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, resources []rollbackResource, orgName string) {
	s.logger.Warn("Rolling back created proxies and API keys", "count", len(resources))

	// Track unique proxies to delete
	proxyHandles := make(map[string]bool)

	// Clean up each resource
	for _, res := range resources {
		// Delete provider API key from KV and SecretReference
		if res.providerSecretLoc != nil {
			if err := s.secretClient.DeleteSecret(ctx, *res.providerSecretLoc, res.secretRefName); err != nil {
				kvPath, _ := res.providerSecretLoc.KVPath()
				s.logger.Error("Failed to delete provider API key during rollback",
					"kvPath", kvPath, "error", err)
			}
		}
		// Delete proxy API key from KV and SecretReference
		if res.proxySecretLoc != nil {
			if err := s.secretClient.DeleteSecret(ctx, *res.proxySecretLoc, res.secretRefName); err != nil {
				kvPath, _ := res.proxySecretLoc.KVPath()
				s.logger.Error("Failed to delete proxy API key during rollback",
					"kvPath", kvPath, "error", err)
			}
		}

		// Revoke the proxy API key if one was created
		if res.proxyAPIKeyID != "" {
			if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, res.proxyHandle, res.proxyAPIKeyID); err != nil {
				s.logger.Error(
					"Failed to revoke proxy API key during rollback",
					"proxyHandle", res.proxyHandle,
					"apiKeyID", res.proxyAPIKeyID,
					"error", err,
				)
			} else {
				s.logger.Info(
					"Revoked proxy API key during rollback",
					"proxyHandle", res.proxyHandle,
					"apiKeyID", res.proxyAPIKeyID,
				)
			}
		}

		// Delete the AI application only if this rollback resource was the one that
		// created it (i.e. it didn't exist before this operation).
		if res.createdNewApp {
			if err := s.aiApplicationService.Delete(ctx, orgName, res.appProjectName, res.appAgentID, res.appEnvName); err != nil {
				s.logger.Warn("Failed to delete AI application during rollback (best-effort)",
					"agentID", res.appAgentID, "envName", res.appEnvName, "error", err)
			}
		}

		// Undeploy deployment — only if a deployment was actually created.
		if res.proxyHandle != "" && res.deploymentID != uuid.Nil {
			if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(res.proxyHandle, res.deploymentID.String(), orgName); err != nil {
				s.logger.Error(
					"Failed to undeploy proxy during rollback",
					"handle", res.proxyHandle,
					"deploymentID", res.deploymentID,
					"error", err,
				)
			}
		}

		// Revoke provider API key if one was created (CRIT-3).
		if res.providerAPIKeyID != "" && res.providerUUID != "" {
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, res.providerUUID, res.providerAPIKeyID); err != nil {
				s.logger.Error(
					"Failed to revoke provider API key during rollback",
					"providerAPIKeyID", res.providerAPIKeyID,
					"providerUUID", res.providerUUID,
					"error", err,
				)
			} else {
				s.logger.Info(
					"Revoked provider API key during rollback",
					"providerAPIKeyID", res.providerAPIKeyID,
				)
			}
		}

		if res.proxyHandle != "" {
			proxyHandles[res.proxyHandle] = true
		}
	}

	// Delete all unique proxies
	for handle := range proxyHandles {
		if err := s.llmProxyService.Delete(handle, orgName); err != nil {
			s.logger.Error(
				"Failed to delete proxy during rollback",
				"handle", handle,
				"error", err,
			)
		}
	}

	// Revert DB mappings for Scenario A: restore old proxy UUID so the mapping is not left dangling (HIGH-4).
	for _, res := range resources {
		if res.mappingID != 0 && res.oldProxyUUID != uuid.Nil {
			revertErr := s.db.Transaction(func(tx *gorm.DB) error {
				return tx.Model(&models.EnvAgentModelMapping{}).
					Where("id = ?", res.mappingID).
					Update("llm_proxy_uuid", res.oldProxyUUID).Error
			})
			if revertErr != nil {
				s.logger.Error(
					"Failed to revert DB mapping to old proxy UUID during rollback — mapping may be dangling",
					"mappingID", res.mappingID,
					"oldProxyUUID", res.oldProxyUUID,
					"error", revertErr,
				)
			}
		}
	}
}

// buildConfigResponse builds the full configuration response
func (s *agentConfigurationService) buildConfigResponse(ctx context.Context, config *models.AgentConfiguration, includeProxyURL bool) (*models.AgentModelConfigResponse, error) {
	// Get environment names from OpenChoreo
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	s.logger.Info("Building config response", "configUUID", config.UUID, "envCount", len(envs))

	// Build environment model config map
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range config.EnvMappings {
		envName := envMap[mapping.EnvironmentUUID.String()]
		// Fall back to UUID if environment was deleted
		if envName == "" {
			envName = mapping.EnvironmentUUID.String()
		}

		var proxyInfo *models.LLMProxyInfo = nil
		if mapping.LLMProxy != nil {
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				Policies:  mapping.PolicyConfiguration,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(mapping.LLMProxy.ProviderUUID.String(), config.OrganizationName); err == nil && provider.Artifact != nil {
				proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
			}

			// Add proxy URL for external agents (subsequent GET calls)
			if includeProxyURL {
				gateway, err := s.resolveGatewayForProxy(ctx, mapping.LLMProxy.Handle, config.OrganizationName, mapping.EnvironmentUUID)
				if err == nil && mapping.LLMProxy.Configuration.Context != nil {
					url := fmt.Sprintf("%s%s", gateway.Vhost, *mapping.LLMProxy.Configuration.Context)
					proxyInfo.URL = &url
				} else if err == nil {
					// If no context, just use gateway vhost
					url := gateway.Vhost
					proxyInfo.URL = &url
				}
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}
	for _, mapping := range config.EnvMCPMappings {
		envName := envMap[mapping.EnvironmentUUID.String()]
		if envName == "" {
			envName = mapping.EnvironmentUUID.String()
		}
		var proxyInfo *models.LLMProxyInfo
		if mapping.MCPProxy != nil {
			proxyName := ""
			if mapping.MCPProxy.Artifact != nil {
				proxyName = mapping.MCPProxy.Artifact.Handle
			}
			if proxyName == "" {
				proxyName = mapping.MCPProxy.Configuration.Name
			}
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:      utils.StrAsStrPointer(mapping.ArtifactUUID.String()),
				ProviderName:   utils.StrAsStrPointer(proxyName),
				AuthHeaderName: utils.StrAsStrPointer(mcpProxyAPIKeyHeaderName(mapping.MCPProxy)),
			}
			if gateway, err := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, config.OrganizationName, mapping.EnvironmentUUID); err == nil {
				deployedProxy := buildAgentMCPConfigProxy(config, &mapping, mapping.MCPProxy, envName, config.OrganizationName,
					mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName))
				url := buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context)
				proxyInfo.URL = &url
			}
		}
		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Variable rows are stored per-environment but names are config-level — collapse to one entry per key.
	envVars := s.dedupeEnvVariablesByKey(config.UUID, config.EnvVariables)

	return &models.AgentModelConfigResponse{
		UUID:                 config.UUID.String(),
		Name:                 config.Name,
		Description:          config.Description,
		AgentID:              config.AgentID,
		Type:                 models.AgentConfigTypeFromID(config.TypeID),
		OrganizationName:     config.OrganizationName,
		ProjectName:          config.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}, nil
}

// envCredentialKeys returns the keys (environment UUIDs) of the credential map, for safe logging.
func envCredentialKeys(m map[string]envCredentialData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// buildExternalAgentConfigResponse builds response with one-time credentials for external agents
func (s *agentConfigurationService) buildExternalAgentConfigResponse(
	ctx context.Context,
	config *models.AgentConfiguration,
	envCredentials map[string]envCredentialData,
) (*models.AgentModelConfigResponse, error) {
	// Reload configuration with relationships (EnvMappings, LLMProxy, etc.)
	reloadedConfig, err := s.agentConfigRepo.GetByUUID(ctx, config.UUID, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to reload configuration: %w", err)
	}

	s.logger.Info(
		"Building external agent config response",
		"configUUID", config.UUID,
		"envMappingCount", len(reloadedConfig.EnvMappings),
		"envMCPMappingCount", len(reloadedConfig.EnvMCPMappings),
		"envCredentialCount", len(envCredentials),
	)

	// Get environment names
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	// Build environment model config map WITH credentials
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range reloadedConfig.EnvMappings {
		envUUID := mapping.EnvironmentUUID.String()
		envName := envMap[envUUID]
		if envName == "" {
			envName = envUUID
		}

		var proxyInfo *models.LLMProxyInfo
		if mapping.LLMProxy != nil {
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				Policies:  mapping.PolicyConfiguration,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(mapping.LLMProxy.ProviderUUID.String(), config.OrganizationName); err == nil && provider.Artifact != nil {
				proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
			}

			// Add credentials for external agents
			if creds, ok := envCredentials[envUUID]; ok {
				proxyInfo.URL = &creds.proxyURL
				proxyInfo.APIKey = &creds.apiKey
				s.logger.Info(
					"Added credentials for external agent",
					"envUUID", envUUID,
					"hasProxyURL", creds.proxyURL != "",
					"hasAPIKey", creds.apiKey != "",
				)
			} else {
				s.logger.Warn(
					"No credentials found for environment",
					"envUUID", envUUID,
					"availableEnvUUIDs", envCredentialKeys(envCredentials),
				)
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}
	for _, mapping := range reloadedConfig.EnvMCPMappings {
		envUUID := mapping.EnvironmentUUID.String()
		envName := envMap[envUUID]
		if envName == "" {
			envName = envUUID
		}

		var proxyInfo *models.LLMProxyInfo
		if mapping.MCPProxy != nil {
			proxyName := ""
			if mapping.MCPProxy.Artifact != nil {
				proxyName = mapping.MCPProxy.Artifact.Handle
			}
			if proxyName == "" {
				proxyName = mapping.MCPProxy.Configuration.Name
			}
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID:      utils.StrAsStrPointer(mapping.ArtifactUUID.String()),
				ProviderName:   utils.StrAsStrPointer(proxyName),
				AuthHeaderName: utils.StrAsStrPointer(mcpProxyAPIKeyHeaderName(mapping.MCPProxy)),
			}
			if creds, ok := envCredentials[envUUID]; ok {
				proxyInfo.URL = &creds.proxyURL
				if creds.apiKey != "" {
					proxyInfo.APIKey = &creds.apiKey
				}
				s.logger.Info(
					"Added MCP credentials for external agent",
					"envUUID", envUUID,
					"hasProxyURL", creds.proxyURL != "",
					"hasAPIKey", creds.apiKey != "",
				)
			} else if gateway, err := s.resolveGatewayForMCPArtifact(ctx, mapping.ArtifactUUID, config.OrganizationName, mapping.EnvironmentUUID); err == nil {
				deployedProxy := buildAgentMCPConfigProxy(reloadedConfig, &mapping, mapping.MCPProxy, envName, config.OrganizationName,
					mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName))
				url := buildMCPProxyURL(gateway.Vhost, deployedProxy.Configuration.Context)
				proxyInfo.URL = &url
			} else {
				s.logger.Warn(
					"No MCP credentials found for environment",
					"envUUID", envUUID,
					"availableEnvUUIDs", envCredentialKeys(envCredentials),
				)
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Variable rows are stored per-environment but names are config-level — collapse to one entry per key.
	envVars := s.dedupeEnvVariablesByKey(reloadedConfig.UUID, reloadedConfig.EnvVariables)

	return &models.AgentModelConfigResponse{
		UUID:                 reloadedConfig.UUID.String(),
		Name:                 reloadedConfig.Name,
		Description:          reloadedConfig.Description,
		AgentID:              reloadedConfig.AgentID,
		Type:                 models.AgentConfigTypeFromID(reloadedConfig.TypeID),
		OrganizationName:     reloadedConfig.OrganizationName,
		ProjectName:          reloadedConfig.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            reloadedConfig.CreatedAt,
		UpdatedAt:            reloadedConfig.UpdatedAt,
	}, nil
}

func (s *agentConfigurationService) processRollBack(ctx context.Context, rollbackResources []rollbackResource, orgName string, configUUID uuid.UUID) {
	s.logger.Error("Rolling back created proxies and API keys", "count", len(rollbackResources))
	s.rollbackProxies(ctx, rollbackResources, orgName)
	s.compensatingDeleteConfig(ctx, configUUID, orgName)
	s.logger.Error("Rolled back created proxies and API keys", "count", len(rollbackResources))
}

func (s *agentConfigurationService) cleanupMCPConfig(ctx context.Context, configUUID uuid.UUID, orgName string) {
	if s.envMCPMappingRepo != nil && s.mcpProxyService != nil {
		if mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, configUUID); err == nil {
			for _, mapping := range mappings {
				s.mcpProxyService.BroadcastMCPArtifactDeletion(ctx, mapping.ArtifactUUID, orgName)
			}
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var mappings []models.EnvAgentMCPMapping
		if s.envMCPMappingRepo != nil {
			var err error
			mappings, err = s.envMCPMappingRepo.ListByConfig(ctx, configUUID)
			if err != nil {
				return err
			}
		}
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName); err != nil {
			return err
		}
		for _, mapping := range mappings {
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("artifact_uuid = ? AND organization_name = ?", mapping.ArtifactUUID, orgName).
				Delete(&models.Deployment{}).Error; err != nil {
				return err
			}
			if err := tx.Where("uuid = ?", mapping.ArtifactUUID).Delete(&models.Artifact{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		s.logger.Warn("failed to clean up MCP configuration", "configUUID", configUUID, "error", err)
	}
}

func (s *agentConfigurationService) ListAgentLLMConfigSecretReferences(ctx context.Context, agentID, orgName, environmentName string) (map[string]struct{}, error) {
	env, err := s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment %q: %w", environmentName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return nil, fmt.Errorf("invalid environment UUID %q: %w", env.UUID, err)
	}
	refs, err := s.envVariableRepo.ListSecretReferencesByAgentAndEnv(ctx, agentID, orgName, envUUID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		result[ref] = struct{}{}
	}
	return result, nil
}

func (s *agentConfigurationService) ListSystemManagedEnvVarKeys(ctx context.Context, agentID, orgName, environmentName string) (map[string]bool, error) {
	env, err := s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment %q: %w", environmentName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return nil, fmt.Errorf("invalid environment UUID %q: %w", env.UUID, err)
	}

	agentConfig, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No LLM configuration exists for this agent — no system-managed keys
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("failed to get agent configuration: %w", err)
	}

	vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, agentConfig.UUID, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list env config variables: %w", err)
	}

	keys := make(map[string]bool, len(vars))
	for _, v := range vars {
		keys[v.VariableName] = true
	}
	return keys, nil
}

// BuildSystemManagedEnvVarsFromConfig constructs the LLM env vars (URL + API key ref)
// for a given agent and environment from the DB config. Used during promotion when
// the target environment's ReleaseBinding doesn't have these vars yet.
func (s *agentConfigurationService) BuildSystemManagedEnvVarsFromConfig(ctx context.Context, agentID, orgName, environmentName string) ([]client.EnvVar, error) {
	env, err := s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment %q: %w", environmentName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return nil, fmt.Errorf("invalid environment UUID %q: %w", env.UUID, err)
	}

	agentConfig, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get agent configuration: %w", err)
	}

	// Get the env-agent mapping for this environment to find the proxy
	mapping, err := s.envMappingRepo.GetByConfigAndEnv(ctx, agentConfig.UUID, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get env mapping for %s: %w", environmentName, err)
	}

	// Use preloaded proxy from the mapping
	proxy := mapping.LLMProxy
	if proxy == nil {
		return nil, fmt.Errorf("LLM proxy not found for mapping in environment %s", environmentName)
	}

	// Resolve gateway for the proxy URL
	gateway, err := s.resolveGatewayForProxy(ctx, proxy.Handle, orgName, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve gateway: %w", err)
	}
	proxyURL := buildProxyURL(gateway, proxy.Configuration.Context, true)

	// Get the env config variables to find the secret reference and variable names
	vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, agentConfig.UUID, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list env config variables: %w", err)
	}

	// Build env vars from the DB config
	var result []client.EnvVar
	for _, v := range vars {
		if v.SecretReference != "" {
			result = append(result, client.EnvVar{
				Key: v.VariableName,
				ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: v.SecretReference,
						Key:  secretmanagersvc.SecretKeyAPIKey,
					},
				},
			})
		} else {
			result = append(result, client.EnvVar{
				Key:   v.VariableName,
				Value: proxyURL,
			})
		}
	}

	return result, nil
}
