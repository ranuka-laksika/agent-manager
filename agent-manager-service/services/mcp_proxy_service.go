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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const (
	mcpJSONRPCVersion      = "2.0"
	mcpProtocolVersion     = "2025-06-18"
	mcpMethodInitialize    = "initialize"
	mcpMethodInitialized   = "notifications/initialized"
	mcpMethodToolsList     = "tools/list"
	mcpMethodPromptsList   = "prompts/list"
	mcpMethodResourcesList = "resources/list"
	mcpClientName          = "agent-manager-mcp-client"
	mcpClientVersion       = "1.0.0"
	mcpSessionHeader       = "Mcp-Session-Id"
	mcpRequestTimeout      = 10 * time.Second
	maxMCPResponseBody     = 10 << 20
)

var excludedMCPProxyPolicyListNames = map[string]struct{}{
	"mcp-acl-list": {},
	"mcp-auth":     {},
	"mcp-authz":    {},
	"mcp-rewrite":  {},
}

// MCPProxyService handles MCP proxy operations.
type MCPProxyService struct {
	db                   *gorm.DB
	repo                 repositories.MCPProxyRepository
	deploymentRepo       repositories.DeploymentRepository
	gatewayRepo          repositories.GatewayRepository
	envMCPMappingRepo    repositories.EnvAgentMCPMappingRepository
	gatewayEventsService *GatewayEventsService
	apiKeyBroadcaster    apiKeyBroadcaster
	client               *http.Client
	logger               *slog.Logger
	encryptionKey        []byte
}

// NewMCPProxyService creates a new MCP proxy service.
func NewMCPProxyService(
	db *gorm.DB,
	repo repositories.MCPProxyRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	envMCPMappingRepo repositories.EnvAgentMCPMappingRepository,
	gatewayEventsService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
	logger *slog.Logger,
	encryptionKey []byte,
) *MCPProxyService {
	return &MCPProxyService{
		db:                   db,
		repo:                 repo,
		deploymentRepo:       deploymentRepo,
		gatewayRepo:          gatewayRepo,
		envMCPMappingRepo:    envMCPMappingRepo,
		gatewayEventsService: gatewayEventsService,
		apiKeyBroadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayEventsService,
			apiKeyRepo:     apiKeyRepo,
		},
		client:        ssrf.NewClient(mcpRequestTimeout),
		logger:        logger,
		encryptionKey: encryptionKey,
	}
}

// Create creates a new MCP proxy.
func (s *MCPProxyService) Create(ctx context.Context, orgUUID, createdBy string, req *models.MCPProxyDTO) (*models.MCPProxyDTO, error) {
	if req == nil {
		return nil, utils.ErrInvalidInput
	}

	handle := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	version := strings.TrimSpace(req.Version)
	if handle == "" || name == "" || version == "" {
		return nil, utils.ErrInvalidInput
	}
	if req.Upstream.Main == nil || strings.TrimSpace(req.Upstream.Main.URL) == "" {
		return nil, utils.ErrInvalidInput
	}
	if err := validateMCPProxyUpstreamURLs(ctx, req.Upstream); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	upstream := req.Upstream
	if upstream.Main != nil {
		auth, err := s.prepareMCPUpstreamAuthForStorage(nil, upstream.Main.Auth)
		if err != nil {
			return nil, err
		}
		upstream.Main.Auth = auth
	}

	exists, err := s.repo.Exists(ctx, handle, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to check MCP proxy existence: %w", err)
	}
	if exists {
		return nil, utils.ErrMCPProxyExists
	}

	proxy := &models.MCPProxy{
		Description: valueOrEmpty(req.Description),
		CreatedBy:   createdBy,
		Configuration: models.MCPProxyConfig{
			Name:         name,
			Version:      version,
			Context:      req.Context,
			Vhost:        req.Vhost,
			SpecVersion:  valueOrEmpty(req.McpSpecVersion),
			Upstream:     upstream,
			Policies:     copyMCPPoliciesPtr(req.Policies),
			Capabilities: copyMCPCapabilities(req.Capabilities),
			Security:     defaultMCPProxySecurity(req.Security),
		},
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.repo.Create(ctx, tx, proxy, handle, name, version, orgUUID)
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, utils.ErrMCPProxyExists
		}
		return nil, fmt.Errorf("failed to create MCP proxy: %w", err)
	}

	created, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to retrieve MCP proxy: %w", err)
	}

	if len(req.Gateways) > 0 {
		if err := s.deployMCPProxyToSelectedGateways(ctx, created, orgUUID, req.Gateways); err != nil {
			return convertModelMCPProxyToSpec(created), fmt.Errorf("proxy created but deployment failed: %w", err)
		}
	}

	return convertModelMCPProxyToSpec(created), nil
}

// List retrieves MCP proxies for an organization.
func (s *MCPProxyService) List(ctx context.Context, orgUUID string, limit, offset int) (*models.MCPProxyListResponse, error) {
	proxies, err := s.repo.List(ctx, orgUUID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxies: %w", err)
	}

	total, err := s.repo.Count(ctx, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to count MCP proxies: %w", err)
	}

	resp := &models.MCPProxyListResponse{
		Count: len(proxies),
		List:  make([]models.MCPProxyListItem, 0, len(proxies)),
		Pagination: models.PaginationInfo{
			Count:  total,
			Limit:  limit,
			Offset: offset,
		},
	}
	for _, proxy := range proxies {
		resp.List = append(resp.List, convertModelMCPProxyToListItem(proxy))
	}
	return resp, nil
}

// ListAvailableMCPPolicies returns policy versions reported by active gateways in the organization.
func (s *MCPProxyService) ListAvailableMCPPolicies(ctx context.Context, orgUUID string) (*models.MCPPolicyAvailabilityResponse, error) {
	_ = ctx
	if s.gatewayRepo == nil {
		return &models.MCPPolicyAvailabilityResponse{List: []models.MCPPolicyAvailableItem{}}, nil
	}

	active := true
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgUUID,
		Status:         &active,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list active gateways: %w", err)
	}

	var available map[string]models.MCPPolicyAvailableItem
	seenGateway := false
	for _, gateway := range gateways {
		if gateway == nil {
			continue
		}
		gatewayPolicies := map[string]models.MCPPolicyAvailableItem{}
		for _, policy := range extractGatewayPolicyManifestItems(gateway.Manifest) {
			if policy.Name == "" || policy.Version == "" {
				continue
			}
			key := policy.Name + "\x00" + policy.Version
			gatewayPolicies[key] = policy
		}
		if !seenGateway {
			available = gatewayPolicies
			seenGateway = true
			continue
		}
		for key := range available {
			if _, ok := gatewayPolicies[key]; !ok {
				delete(available, key)
			}
		}
	}

	if available == nil {
		available = map[string]models.MCPPolicyAvailableItem{}
	}
	items := make([]models.MCPPolicyAvailableItem, 0, len(available))
	for _, item := range available {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Version < items[j].Version
		}
		return items[i].Name < items[j].Name
	})

	return &models.MCPPolicyAvailabilityResponse{
		Count: int32(len(items)),
		List:  items,
	}, nil
}

// Get retrieves an MCP proxy by handle.
func (s *MCPProxyService) Get(ctx context.Context, orgUUID, proxyID string) (*models.MCPProxyDTO, error) {
	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}

	proxy, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to get MCP proxy: %w", err)
	}

	dto := convertModelMCPProxyToSpec(proxy)
	if dto != nil && s.deploymentRepo != nil {
		gatewayIDs, err := s.deploymentRepo.GetDeployedGatewaysByProvider(proxy.UUID, orgUUID)
		if err != nil {
			s.logger.Warn("Failed to list deployed gateways for MCP proxy", "proxyID", proxy.UUID, "orgName", orgUUID, "error", err)
		} else {
			dto.Gateways = gatewayIDs
		}
	}
	return dto, nil
}

// Update modifies an existing MCP proxy and redeploys it to active gateways. Returns
// the DTO for the response and the underlying model so the caller can cascade further
// work (e.g. redeploying agent-scoped mapping artifacts).
func (s *MCPProxyService) Update(ctx context.Context, orgUUID, proxyID string, req *models.MCPProxyDTO) (*models.MCPProxyDTO, *models.MCPProxy, error) {
	if req == nil {
		return nil, nil, utils.ErrInvalidInput
	}

	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, nil, utils.ErrInvalidInput
	}
	if strings.TrimSpace(req.ID) != "" && strings.TrimSpace(req.ID) != handle {
		return nil, nil, utils.ErrInvalidInput
	}

	name := strings.TrimSpace(req.Name)
	version := strings.TrimSpace(req.Version)
	if name == "" || version == "" {
		return nil, nil, utils.ErrInvalidInput
	}
	if req.Upstream.Main == nil || strings.TrimSpace(req.Upstream.Main.URL) == "" {
		return nil, nil, utils.ErrInvalidInput
	}
	if err := validateMCPProxyUpstreamURLs(ctx, req.Upstream); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	upstream := req.Upstream

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		proxy, err := s.repo.GetByHandleForUpdate(ctx, tx, handle, orgUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return utils.ErrMCPProxyNotFound
			}
			return fmt.Errorf("failed to get MCP proxy before update: %w", err)
		}

		var existingAuth *models.UpstreamAuth
		if proxy.Configuration.Upstream.Main != nil {
			existingAuth = proxy.Configuration.Upstream.Main.Auth
		}
		if upstream.Main != nil {
			auth, err := s.prepareMCPUpstreamAuthForStorage(existingAuth, upstream.Main.Auth)
			if err != nil {
				return err
			}
			upstream.Main.Auth = auth
		}

		proxy.Description = valueOrEmpty(req.Description)
		proxy.Name = name
		proxy.Version = version
		proxy.Configuration = models.MCPProxyConfig{
			Name:         name,
			Version:      version,
			Context:      req.Context,
			Vhost:        req.Vhost,
			SpecVersion:  valueOrEmpty(req.McpSpecVersion),
			Upstream:     upstream,
			Policies:     copyMCPPoliciesPtr(req.Policies),
			Capabilities: copyMCPCapabilities(req.Capabilities),
			Security:     req.Security,
		}

		return s.repo.Update(ctx, tx, proxy, orgUUID)
	}); err != nil {
		if errors.Is(err, utils.ErrMCPProxyNotFound) || errors.Is(err, utils.ErrInvalidInput) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("failed to update MCP proxy: %w", err)
	}

	updated, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, utils.ErrMCPProxyNotFound
		}
		return nil, nil, fmt.Errorf("failed to retrieve MCP proxy: %w", err)
	}

	if err := s.redeployMCPProxyToCurrentGateways(ctx, updated, orgUUID); err != nil {
		s.logger.Warn("Failed to redeploy updated MCP proxy", "proxyID", updated.UUID, "orgName", orgUUID, "error", err)
	}

	return convertModelMCPProxyToSpec(updated), updated, nil
}

// Delete removes an MCP proxy by handle. MCP proxy mappings are deployable artifacts
// derived from an MCP proxy, so the source proxy cannot be deleted while mappings exist.
func (s *MCPProxyService) Delete(ctx context.Context, orgUUID, proxyID string) error {
	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return utils.ErrInvalidInput
	}

	proxy, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMCPProxyNotFound
		}
		return fmt.Errorf("failed to get MCP proxy before delete: %w", err)
	}

	var mappings []models.EnvAgentMCPMapping
	if s.envMCPMappingRepo != nil {
		mappings, err = s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
		if err != nil {
			return fmt.Errorf("failed to list MCP mappings before delete: %w", err)
		}
	}
	if len(mappings) > 0 {
		return utils.ErrMCPProxyHasMappings
	}

	// Resolve gateway sets BEFORE deletion so deploymentRepo lookups still see the
	// existing deployment rows; afterwards only the fallback "active gateways" set
	// would be returned.
	proxyGatewayIDs := s.gatewayIDsForDeletion(ctx, proxy, orgUUID)

	if err := s.repo.Delete(ctx, handle, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMCPProxyNotFound
		}
		return fmt.Errorf("failed to delete MCP proxy: %w", err)
	}

	s.broadcastMCPProxyDeletion(ctx, proxy, proxyGatewayIDs)
	return nil
}

// CreateAPIKey generates an API key for a source MCP proxy and broadcasts it to all gateways.
func (s *MCPProxyService) CreateAPIKey(ctx context.Context, orgUUID, proxyID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return nil, err
	}
	proxyUUID := proxy.UUID.String()
	return s.apiKeyBroadcaster.broadcastCreate(orgUUID, proxyUUID, proxyUUID, req)
}

// RevokeAPIKey revokes an API key for a source MCP proxy and broadcasts the revocation.
func (s *MCPProxyService) RevokeAPIKey(ctx context.Context, orgUUID, proxyID, keyName string) error {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return err
	}
	proxyUUID := proxy.UUID.String()
	return s.apiKeyBroadcaster.broadcastRevoke(orgUUID, proxyUUID, proxyUUID, keyName)
}

// RotateAPIKey rotates an API key for a source MCP proxy and broadcasts the new hash.
func (s *MCPProxyService) RotateAPIKey(ctx context.Context, orgUUID, proxyID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return nil, err
	}
	proxyUUID := proxy.UUID.String()
	return s.apiKeyBroadcaster.broadcastRotate(orgUUID, proxyUUID, proxyUUID, keyName, req)
}

func (s *MCPProxyService) getMCPProxyByID(ctx context.Context, orgUUID, proxyID string) (*models.MCPProxy, error) {
	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}
	proxy, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to get MCP proxy: %w", err)
	}
	return proxy, nil
}

func extractGatewayPolicyManifestItems(value interface{}) []models.MCPPolicyAvailableItem {
	seen := map[string]struct{}{}
	items := make([]models.MCPPolicyAvailableItem, 0)
	var walk func(interface{})

	add := func(name, version string) {
		name = strings.TrimSpace(name)
		version = strings.TrimSpace(version)
		if name == "" || version == "" {
			return
		}
		if _, ok := excludedMCPProxyPolicyListNames[strings.ToLower(name)]; ok {
			return
		}
		key := name + "\x00" + version
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, models.MCPPolicyAvailableItem{Name: name, Version: version})
	}

	stringValue := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}

	walk = func(current interface{}) {
		switch typed := current.(type) {
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case []map[string]interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			name := stringValue(firstMapValue(typed, "name", "policyName", "id"))
			version := stringValue(firstMapValue(typed, "version", "policyVersion"))
			if name != "" && version != "" {
				add(name, version)
			}
			if name != "" {
				if versions, ok := firstMapValue(typed, "versions", "policyVersions").([]interface{}); ok {
					for _, rawVersion := range versions {
						add(name, stringValue(rawVersion))
					}
				}
				if versions, ok := firstMapValue(typed, "versions", "policyVersions").([]string); ok {
					for _, rawVersion := range versions {
						add(name, rawVersion)
					}
				}
			}
			for _, nested := range typed {
				walk(nested)
			}
		}
	}

	walk(value)
	return items
}

func firstMapValue(values map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func (s *MCPProxyService) prepareMCPUpstreamAuthForStorage(existing, updated *models.UpstreamAuth) (*models.UpstreamAuth, error) {
	auth := preserveUpstreamAuthCredential(existing, updated)
	if auth == nil {
		return nil, nil //nolint:nilnil // A nil auth value is valid when upstream auth is omitted.
	}

	if auth.Header != nil {
		header := strings.TrimSpace(*auth.Header)
		auth.Header = &header
	}
	if auth.Value != nil && *auth.Value == "" {
		auth.Value = nil
	}
	// A newly supplied plaintext value supersedes any preserved encrypted secret,
	// keeping Value and SecretRef mutually exclusive for Validate.
	if auth.Value != nil {
		auth.SecretRef = nil
	}

	if err := auth.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	hasHeader := auth.Header != nil && *auth.Header != ""
	hasCredential := auth.Value != nil || auth.SecretRef != nil
	if !hasHeader && !hasCredential {
		return nil, nil //nolint:nilnil // Empty auth fields mean the endpoint should not store auth.
	}
	if hasHeader != hasCredential {
		return nil, fmt.Errorf("%w: authentication header and value must be provided together", utils.ErrInvalidInput)
	}

	if auth.Value != nil {
		encrypted, err := utils.EncryptBytes([]byte(*auth.Value), s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt upstream auth: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		auth.SecretRef = &encoded
		auth.Value = nil
	}

	return auth, nil
}

// FetchServerInfo fetches server information from an MCP backend.
func (s *MCPProxyService) FetchServerInfo(ctx context.Context, req *models.MCPServerInfoFetchRequest) (*models.MCPServerInfoFetchResponse, error) {
	if req == nil || req.URL == nil || strings.TrimSpace(*req.URL) == "" {
		return nil, utils.ErrInvalidInput
	}
	if req.ProxyID != nil && strings.TrimSpace(*req.ProxyID) != "" {
		return nil, fmt.Errorf("proxyId refresh is not supported yet: %w", utils.ErrInvalidInput)
	}

	endpointURL := strings.TrimSpace(*req.URL)
	if err := ssrf.ValidateURL(ctx, endpointURL); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	headerName, headerValue := authHeader(req.Auth)
	return s.fetchMCPServerInfo(ctx, endpointURL, headerName, headerValue)
}

func authHeader(auth *models.UpstreamAuth) (string, string) {
	if auth == nil || auth.Header == nil || auth.Value == nil {
		return "", ""
	}
	return strings.TrimSpace(*auth.Header), *auth.Value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func copyMCPPoliciesPtr(policies *[]models.MCPPolicy) []models.MCPPolicy {
	if policies == nil || len(*policies) == 0 {
		return nil
	}
	return copyMCPPolicies(*policies)
}

func copyMCPPoliciesToPtr(policies []models.MCPPolicy) *[]models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := copyMCPPolicies(policies)
	return &out
}

func copyMCPPolicies(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]models.MCPPolicy, len(policies))
	copy(out, policies)
	return out
}

func copyMCPCapabilities(capabilities *models.MCPProxyCapabilities) *models.MCPProxyCapabilities {
	if capabilities == nil {
		return nil
	}
	return &models.MCPProxyCapabilities{
		Prompts:   capabilities.Prompts,
		Resources: capabilities.Resources,
		Tools:     capabilities.Tools,
	}
}

func sanitizeMCPUpstreamForResponse(upstream models.UpstreamConfig) models.UpstreamConfig {
	return models.UpstreamConfig{
		Main:    sanitizeMCPUpstreamEndpointForResponse(upstream.Main),
		Sandbox: sanitizeMCPUpstreamEndpointForResponse(upstream.Sandbox),
	}
}

func sanitizeMCPUpstreamEndpointForResponse(endpoint *models.UpstreamEndpoint) *models.UpstreamEndpoint {
	if endpoint == nil {
		return nil
	}
	sanitized := *endpoint
	sanitized.Auth = sanitizeMCPUpstreamAuthForResponse(endpoint.Auth)
	return &sanitized
}

func sanitizeMCPUpstreamAuthForResponse(auth *models.UpstreamAuth) *models.UpstreamAuth {
	if auth == nil {
		return nil
	}
	sanitized := *auth
	sanitized.Value = nil
	return &sanitized
}

func defaultMCPProxySecurity(security *models.SecurityConfig) *models.SecurityConfig {
	enabled := true
	if security != nil {
		out := *security
		if out.Enabled == nil {
			out.Enabled = &enabled
		}
		if !isBoolTrue(out.Enabled) {
			return &out
		}
		if out.APIKey == nil {
			out.APIKey = &models.APIKeySecurity{}
		} else {
			apiKey := *out.APIKey
			out.APIKey = &apiKey
		}
		if out.APIKey.Enabled == nil {
			out.APIKey.Enabled = &enabled
		}
		if isBoolTrue(out.APIKey.Enabled) {
			if strings.TrimSpace(out.APIKey.Key) == "" {
				out.APIKey.Key = "X-API-Key"
			}
			if strings.TrimSpace(out.APIKey.In) == "" {
				out.APIKey.In = "header"
			}
		}
		return &out
	}
	return &models.SecurityConfig{
		Enabled: &enabled,
		APIKey: &models.APIKeySecurity{
			Enabled: &enabled,
			Key:     "X-API-Key",
			In:      "header",
		},
	}
}

func convertModelMCPProxyToSpec(proxy *models.MCPProxy) *models.MCPProxyDTO {
	if proxy == nil {
		return nil
	}
	id := proxy.Handle
	name := proxy.Name
	version := proxy.Version
	createdAt := proxy.CreatedAt
	updatedAt := proxy.UpdatedAt
	inCatalog := false
	if proxy.Artifact != nil {
		id = proxy.Artifact.Handle
		name = proxy.Artifact.Name
		version = proxy.Artifact.Version
		createdAt = proxy.Artifact.CreatedAt
		updatedAt = proxy.Artifact.UpdatedAt
		inCatalog = proxy.Artifact.InCatalog
	}
	if name == "" {
		name = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	return &models.MCPProxyDTO{
		Capabilities:   copyMCPCapabilities(proxy.Configuration.Capabilities),
		Context:        proxy.Configuration.Context,
		CreatedAt:      timePtrIfNotZero(createdAt),
		CreatedBy:      stringPtrIfNotEmpty(proxy.CreatedBy),
		Description:    stringPtrIfNotEmpty(proxy.Description),
		ID:             id,
		InCatalog:      inCatalog,
		McpSpecVersion: stringPtrIfNotEmpty(proxy.Configuration.SpecVersion),
		Name:           name,
		Policies:       copyMCPPoliciesToPtr(proxy.Configuration.Policies),
		Security:       proxy.Configuration.Security,
		Upstream:       sanitizeMCPUpstreamForResponse(proxy.Configuration.Upstream),
		UpdatedAt:      timePtrIfNotZero(updatedAt),
		Version:        version,
		Vhost:          proxy.Configuration.Vhost,
	}
}

func convertModelMCPProxyToListItem(proxy *models.MCPProxy) models.MCPProxyListItem {
	status := proxy.Status
	id := proxy.Handle
	name := proxy.Name
	version := proxy.Version
	createdAt := proxy.CreatedAt
	updatedAt := proxy.UpdatedAt
	if proxy.Artifact != nil {
		id = proxy.Artifact.Handle
		name = proxy.Artifact.Name
		version = proxy.Artifact.Version
		createdAt = proxy.Artifact.CreatedAt
		updatedAt = proxy.Artifact.UpdatedAt
	}
	if name == "" {
		name = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	return models.MCPProxyListItem{
		Context:        proxy.Configuration.Context,
		CreatedAt:      timePtrIfNotZero(createdAt),
		CreatedBy:      stringPtrIfNotEmpty(proxy.CreatedBy),
		Description:    stringPtrIfNotEmpty(proxy.Description),
		ID:             stringPtrIfNotEmpty(id),
		McpSpecVersion: stringPtrIfNotEmpty(proxy.Configuration.SpecVersion),
		Name:           stringPtrIfNotEmpty(name),
		Status:         stringPtrIfNotEmpty(status),
		UpdatedAt:      timePtrIfNotZero(updatedAt),
		Version:        stringPtrIfNotEmpty(version),
	}
}

func validateMCPProxyUpstreamURLs(ctx context.Context, upstream models.UpstreamConfig) error {
	if upstream.Main != nil {
		mainURL := strings.TrimSpace(upstream.Main.URL)
		if mainURL == "" {
			return fmt.Errorf("main upstream url is required")
		}
		if err := ssrf.ValidateURL(ctx, mainURL); err != nil {
			return fmt.Errorf("main upstream url: %w", err)
		}
	}
	if upstream.Sandbox != nil {
		sandboxURL := strings.TrimSpace(upstream.Sandbox.URL)
		if sandboxURL == "" {
			return nil
		}
		if err := ssrf.ValidateURL(ctx, sandboxURL); err != nil {
			return fmt.Errorf("sandbox upstream url: %w", err)
		}
	}
	return nil
}

func (s *MCPProxyService) fetchMCPServerInfo(ctx context.Context, endpointURL string, headerName string, headerValue string) (*models.MCPServerInfoFetchResponse, error) {
	sessionID, serverInfo, err := s.initializeMCPServer(ctx, endpointURL, headerName, headerValue)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP server: %w", err)
	}

	notifyReq := mcpJSONRPCRequest{
		JSONRPC: mcpJSONRPCVersion,
		Method:  mcpMethodInitialized,
	}
	if _, err := s.postJSONRPCWithSession(ctx, endpointURL, notifyReq, sessionID, headerName, headerValue); err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	resp := &models.MCPServerInfoFetchResponse{}
	if serverInfo != nil {
		resp.ServerInfo = &serverInfo
	}

	if tools := s.fetchTools(ctx, endpointURL, sessionID, headerName, headerValue); len(tools) > 0 {
		resp.Tools = &tools
	}
	if prompts := s.fetchPrompts(ctx, endpointURL, sessionID, headerName, headerValue); len(prompts) > 0 {
		resp.Prompts = &prompts
	}
	if resources := s.fetchResources(ctx, endpointURL, sessionID, headerName, headerValue); len(resources) > 0 {
		resp.Resources = &resources
	}

	return resp, nil
}

func (s *MCPProxyService) initializeMCPServer(ctx context.Context, endpointURL string, headerName string, headerValue string) (string, map[string]interface{}, error) {
	initReq := mcpJSONRPCRequest{
		JSONRPC: mcpJSONRPCVersion,
		ID:      1,
		Method:  mcpMethodInitialize,
		Params: map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{"roots": map[string]bool{"listChanged": true}},
			"clientInfo":      map[string]string{"name": mcpClientName, "version": mcpClientVersion},
		},
	}

	body, headers, err := s.postJSONRPC(ctx, endpointURL, initReq, "", headerName, headerValue)
	if err != nil {
		return "", nil, err
	}

	var initResult mcpInitializeResult
	if err := json.Unmarshal(body, &initResult); err != nil {
		return "", nil, fmt.Errorf("failed to parse initialize response: %w, body: %s", err, string(body))
	}
	if initResult.Error != nil {
		return "", nil, fmt.Errorf("initialize request returned an error: %s", initResult.Error.Message)
	}

	return headers.Get(mcpSessionHeader), initResult.Result.ServerInfo, nil
}

func (s *MCPProxyService) fetchTools(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 2, Method: mcpMethodToolsList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP tools, continuing with available info", "error", err)
		return nil
	}
	var result mcpToolsResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP tools response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("tools/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Tools
}

func (s *MCPProxyService) fetchPrompts(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 3, Method: mcpMethodPromptsList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP prompts, continuing with available info", "error", err)
		return nil
	}
	var result mcpPromptsResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP prompts response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("prompts/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Prompts
}

func (s *MCPProxyService) fetchResources(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 4, Method: mcpMethodResourcesList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP resources, continuing with available info", "error", err)
		return nil
	}
	var result mcpResourcesResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP resources response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("resources/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Resources
}

func (s *MCPProxyService) postJSONRPCWithSession(ctx context.Context, endpointURL string, payload interface{}, sessionID string, headerName string, headerValue string) ([]byte, error) {
	body, _, err := s.postJSONRPC(ctx, endpointURL, payload, sessionID, headerName, headerValue)
	return body, err
}

func (s *MCPProxyService) postJSONRPC(ctx context.Context, endpointURL string, payload interface{}, sessionID string, headerName string, headerValue string) ([]byte, http.Header, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	setSafeHeader(httpReq.Header, headerName, headerValue)
	if sessionID != "" {
		httpReq.Header.Set(mcpSessionHeader, sessionID)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidURL) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: failed to reach MCP server: %w", utils.ErrURLUnreachable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.Warn("Failed to close MCP server response body", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMCPResponseBody+1))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxMCPResponseBody {
		return nil, nil, fmt.Errorf("%w: response body exceeds %d bytes", utils.ErrMCPResponseTooLarge, maxMCPResponseBody)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, utils.ErrMCPServerUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}
	if isMCPEventStream(resp) {
		data, err := parseMCPEventStream(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse event stream: %w, body: %s", err, string(body))
		}
		body = data
	}

	return body, resp.Header, nil
}

func setSafeHeader(headers http.Header, headerName string, headerValue string) {
	if strings.TrimSpace(headerName) == "" {
		return
	}
	switch strings.ToLower(headerName) {
	case strings.ToLower(mcpSessionHeader), "content-type", "accept":
		return
	default:
		headers.Set(headerName, headerValue)
	}
}

func parseMCPEventStream(body []byte) ([]byte, error) {
	lines := bytes.Split(body, []byte("\n"))
	var eventData bytes.Buffer
	hasData := false
	flushEvent := func() []byte {
		if !hasData {
			return nil
		}
		data := eventData.Bytes()
		eventData.Reset()
		hasData = false
		if len(data) > 0 && !bytes.Equal(data, []byte("{}")) {
			return data
		}
		return nil
	}

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if data := flushEvent(); data != nil {
				return data, nil
			}
			continue
		}
		data, ok := bytes.CutPrefix(line, []byte("data:"))
		if !ok {
			continue
		}
		data = bytes.TrimSpace(data)
		if hasData {
			eventData.WriteByte('\n')
		}
		eventData.Write(data)
		hasData = true
	}
	if data := flushEvent(); data != nil {
		return data, nil
	}
	return nil, errors.New("no data found in event stream")
}

func isMCPEventStream(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
}

type mcpJSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpInitializeResult struct {
	Result struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ServerInfo      map[string]interface{} `json:"serverInfo"`
		Capabilities    map[string]interface{} `json:"capabilities"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpToolsResult struct {
	Result struct {
		Tools []map[string]interface{} `json:"tools"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpPromptsResult struct {
	Result struct {
		Prompts []map[string]interface{} `json:"prompts"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpResourcesResult struct {
	Result struct {
		Resources []map[string]interface{} `json:"resources"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}
