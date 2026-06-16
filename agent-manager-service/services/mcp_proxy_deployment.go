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
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	apiVersionMCPProxy              = "gateway.api-platform.wso2.com/v1alpha1"
	kindMCPProxy                    = "Mcp"
	mcpSetHeadersPolicyName         = "set-headers"
	mcpRemoveHeadersPolicyName      = "remove-headers"
	mcpLogMessagePolicyName         = "log-message"
	mcpBackendAuthPolicyName        = mcpSetHeadersPolicyName
	mcpBackendAuthPolicyVersion     = "v1"
	mcpBackendAuthPolicyDisplayName = "Backend Authentication Header"
)

type mcpPolicyMergeStrategy string

const (
	mcpPolicyMergeStrategyMerge    mcpPolicyMergeStrategy = "merge"
	mcpPolicyMergeStrategyOverride mcpPolicyMergeStrategy = "override"
)

var mcpPolicyMergeStrategies = map[string]mcpPolicyMergeStrategy{
	mcpSetHeadersPolicyName:    mcpPolicyMergeStrategyMerge,
	mcpRemoveHeadersPolicyName: mcpPolicyMergeStrategyMerge,
	mcpLogMessagePolicyName:    mcpPolicyMergeStrategyMerge,
}

// MCPProxyDeploymentYAML represents the deployment YAML consumed by the gateway.
type MCPProxyDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       MCPProxyDeploymentSpec `yaml:"spec" json:"spec"`
}

// MCPProxyDeploymentSpec represents the deployment spec section.
type MCPProxyDeploymentSpec struct {
	DisplayName string             `yaml:"displayName" json:"displayName"`
	Version     string             `yaml:"version" json:"version"`
	Context     string             `yaml:"context" json:"context"`
	Vhost       *string            `yaml:"vhost" json:"vhost"`
	Upstream    MCPProxyUpstream   `yaml:"upstream" json:"upstream"`
	SpecVersion string             `yaml:"specVersion" json:"specVersion"`
	Policies    []models.MCPPolicy `yaml:"policies,omitempty" json:"policies,omitempty"`
}

// MCPProxyUpstream represents the flat upstream shape expected by the gateway.
type MCPProxyUpstream struct {
	URL string `yaml:"url" json:"url"`
}

func (s *MCPProxyService) deployMCPProxyToSelectedGateways(ctx context.Context, proxy *models.MCPProxy, orgName string, gatewayIDs []string) error {
	if s.deploymentRepo == nil || s.gatewayRepo == nil || s.gatewayEventsService == nil {
		return nil
	}

	gateways, err := s.resolveSelectedMCPProxyGateways(ctx, orgName, gatewayIDs)
	if err != nil {
		return err
	}
	return s.deployMCPProxyToGateways(ctx, proxy, orgName, gateways)
}

func (s *MCPProxyService) redeployMCPProxyToCurrentGateways(ctx context.Context, proxy *models.MCPProxy, orgName string) error {
	if s.deploymentRepo == nil || s.gatewayRepo == nil || s.gatewayEventsService == nil || proxy == nil {
		return nil
	}

	// Include gateways whose last deploy left the proxy in UNDEPLOYED state: a save
	// should retry those, and a previous failure must not strand future saves.
	gatewayIDs, err := s.deploymentRepo.GetTrackedGatewaysByProvider(proxy.UUID, orgName)
	if err != nil {
		return fmt.Errorf("failed to list tracked gateways: %w", err)
	}
	if len(gatewayIDs) == 0 {
		return nil
	}

	gateways, err := s.resolveSelectedMCPProxyGateways(ctx, orgName, gatewayIDs)
	if err != nil {
		return err
	}
	return s.deployMCPProxyToGateways(ctx, proxy, orgName, gateways)
}

func (s *MCPProxyService) resolveSelectedMCPProxyGateways(ctx context.Context, orgName string, gatewayIDs []string) ([]*models.Gateway, error) {
	_ = ctx
	if len(gatewayIDs) == 0 {
		return nil, utils.ErrInvalidInput
	}

	gateways := make([]*models.Gateway, 0, len(gatewayIDs))
	seen := map[string]struct{}{}
	for _, gatewayID := range gatewayIDs {
		gatewayID = strings.TrimSpace(gatewayID)
		if gatewayID == "" {
			return nil, fmt.Errorf("%w: gateway id is required", utils.ErrInvalidInput)
		}
		if _, err := uuid.Parse(gatewayID); err != nil {
			return nil, fmt.Errorf("%w: invalid gateway id %q", utils.ErrInvalidInput, gatewayID)
		}
		if _, ok := seen[gatewayID]; ok {
			continue
		}
		seen[gatewayID] = struct{}{}

		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			return nil, fmt.Errorf("failed to get gateway %s: %w", gatewayID, err)
		}
		if gateway == nil {
			return nil, fmt.Errorf("%w: gateway %s not found", utils.ErrInvalidInput, gatewayID)
		}
		if gateway.OrganizationName != orgName {
			return nil, fmt.Errorf("%w: gateway %s does not belong to organization", utils.ErrInvalidInput, gatewayID)
		}
		if !gateway.IsActive {
			return nil, fmt.Errorf("%w: gateway %s is not active", utils.ErrInvalidInput, gatewayID)
		}
		gateways = append(gateways, gateway)
	}

	if len(gateways) == 0 {
		return nil, fmt.Errorf("%w: at least one gateway is required", utils.ErrInvalidInput)
	}
	return gateways, nil
}

func (s *MCPProxyService) deployMCPProxyToGateways(ctx context.Context, proxy *models.MCPProxy, orgName string, gateways []*models.Gateway) error {
	var errs []error
	for _, gateway := range gateways {
		if gateway == nil {
			continue
		}
		if err := s.deployMCPProxyToGateway(ctx, proxy, orgName, gateway); err != nil {
			errs = append(errs, fmt.Errorf("gateway %s: %w", gateway.UUID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to deploy MCP proxy: %w", errors.Join(errs...))
	}
	return nil
}

func (s *MCPProxyService) deployMCPProxyToGateway(ctx context.Context, proxy *models.MCPProxy, orgName string, gateway *models.Gateway) error {
	_ = ctx
	deploymentYAML, err := s.generateMCPProxyDeploymentYAML(proxy)
	if err != nil {
		return fmt.Errorf("failed to generate deployment YAML: %w", err)
	}

	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed
	deployment := &models.Deployment{
		DeploymentID:     deploymentID,
		Name:             deploymentName(proxy),
		ArtifactUUID:     proxy.UUID,
		OrganizationName: orgName,
		GatewayUUID:      gateway.UUID,
		Content:          []byte(deploymentYAML),
		Status:           &deployed,
	}

	hardLimit := maxDeploymentsPerAPI + deploymentLimitBuffer
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, hardLimit); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	performedAt := time.Now().Truncate(time.Millisecond)
	event := &models.MCPProxyDeploymentEvent{
		ProxyID:      proxy.UUID.String(),
		DeploymentID: deployment.DeploymentID.String(),
		PerformedAt:  performedAt,
	}
	if err := s.gatewayEventsService.BroadcastMCPProxyDeploymentEvent(gateway.UUID.String(), event); err != nil {
		s.logger.Warn("Failed to broadcast MCP proxy deployment event",
			"proxyID", proxy.UUID, "deploymentID", deployment.DeploymentID, "gatewayID", gateway.UUID, "error", err)
		return fmt.Errorf("failed to broadcast MCP proxy deployment event for deployment %s: %w", deployment.DeploymentID, err)
	}
	return nil
}

func agentMCPMappingContext(baseContext *string, mappingName string) string {
	base := "/"
	if baseContext != nil && strings.TrimSpace(*baseContext) != "" {
		base = "/" + strings.Trim(strings.TrimSpace(*baseContext), "/")
	}
	return strings.TrimRight(base, "/") + "/" + url.PathEscape(mappingName)
}

func (s *MCPProxyService) BroadcastMCPArtifactDeletion(ctx context.Context, artifactUUID uuid.UUID, orgName string) {
	proxy := &models.MCPProxy{UUID: artifactUUID}
	s.broadcastMCPProxyDeletion(ctx, proxy, s.gatewayIDsForDeletion(ctx, proxy, orgName))
}

func (s *MCPProxyService) gatewayIDsForDeletion(ctx context.Context, proxy *models.MCPProxy, orgName string) []string {
	_ = ctx
	if proxy == nil {
		return nil
	}
	gatewayIDs := map[string]struct{}{}
	if s.deploymentRepo != nil {
		deployedGatewayIDs, err := s.deploymentRepo.GetDeployedGatewaysByProvider(proxy.UUID, orgName)
		if err != nil {
			s.logger.Warn("Failed to get deployed gateways for MCP proxy deletion", "proxyID", proxy.UUID, "orgName", orgName, "error", err)
		}
		for _, gatewayID := range deployedGatewayIDs {
			if strings.TrimSpace(gatewayID) != "" {
				gatewayIDs[gatewayID] = struct{}{}
			}
		}
	}

	if s.gatewayRepo != nil {
		active := true
		gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
			OrganizationID: orgName,
			Status:         &active,
		})
		if err != nil {
			s.logger.Warn("Failed to get active gateways for MCP proxy deletion", "proxyID", proxy.UUID, "orgName", orgName, "error", err)
		}
		for _, gateway := range gateways {
			if gateway != nil {
				gatewayIDs[gateway.UUID.String()] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(gatewayIDs))
	for gatewayID := range gatewayIDs {
		out = append(out, gatewayID)
	}
	return out
}

func (s *MCPProxyService) broadcastMCPProxyDeletion(ctx context.Context, proxy *models.MCPProxy, gatewayIDs []string) {
	_ = ctx
	if proxy == nil || s.gatewayEventsService == nil || len(gatewayIDs) == 0 {
		return
	}
	event := &models.MCPProxyDeletionEvent{
		ProxyID: proxy.UUID.String(),
	}
	for _, gatewayID := range gatewayIDs {
		if strings.TrimSpace(gatewayID) == "" {
			continue
		}
		if err := s.gatewayEventsService.BroadcastMCPProxyDeletionEvent(gatewayID, event); err != nil {
			s.logger.Warn("Failed to broadcast MCP proxy deletion event", "proxyID", proxy.UUID, "gatewayID", gatewayID, "error", err)
		} else {
			s.logger.Info("MCP proxy deletion event sent", "proxyID", proxy.UUID, "gatewayID", gatewayID)
		}
	}
}

func (s *MCPProxyService) generateMCPProxyDeploymentYAML(proxy *models.MCPProxy) (string, error) {
	deployment, err := s.buildMCPProxyDeploymentYAML(proxy)
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP proxy deployment YAML: %w", err)
	}
	return string(yamlBytes), nil
}

func (s *MCPProxyService) buildMCPProxyDeploymentYAML(proxy *models.MCPProxy) (*MCPProxyDeploymentYAML, error) {
	contextValue := "/"
	if proxy.Configuration.Context != nil && strings.TrimSpace(*proxy.Configuration.Context) != "" {
		contextValue = *proxy.Configuration.Context
	}

	specVersion := proxy.Configuration.SpecVersion
	if strings.TrimSpace(specVersion) == "" {
		specVersion = mcpProtocolVersion
	}

	upstream := MCPProxyUpstream{}
	var upstreamAuth *models.UpstreamAuth
	if proxy.Configuration.Upstream.Main != nil {
		upstream.URL = normalizeMCPUpstreamURLForDeployment(proxy.Configuration.Upstream.Main.URL)
		upstreamAuth = proxy.Configuration.Upstream.Main.Auth
	}
	if strings.TrimSpace(upstream.URL) == "" {
		return nil, fmt.Errorf("upstream URL is required")
	}
	policies, err := appendMCPAPIKeyAuthPolicy(proxy.Configuration.Policies, proxy.Configuration.Security)
	if err != nil {
		return nil, err
	}
	policies = appendMCPBackendAuthPolicy(policies, upstreamAuth)
	policies = mergeMCPPoliciesForDeployment(normalizeMCPPoliciesForDeployment(policies))
	handle := proxy.Handle
	displayName := proxy.Name
	version := proxy.Version
	if proxy.Artifact != nil {
		handle = proxy.Artifact.Handle
		displayName = proxy.Artifact.Name
		version = proxy.Artifact.Version
	}
	if displayName == "" {
		displayName = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	if handle == "" {
		handle = proxy.UUID.String()
	}

	return &MCPProxyDeploymentYAML{
		ApiVersion: apiVersionMCPProxy,
		Kind:       kindMCPProxy,
		Metadata:   DeploymentMetadata{Name: handle},
		Spec: MCPProxyDeploymentSpec{
			DisplayName: displayName,
			Version:     version,
			Context:     contextValue,
			Vhost:       proxy.Configuration.Vhost,
			Upstream:    upstream,
			SpecVersion: specVersion,
			Policies:    policies,
		},
	}, nil
}

func appendMCPBackendAuthPolicy(policies []models.MCPPolicy, auth *models.UpstreamAuth) []models.MCPPolicy {
	if auth == nil || auth.Header == nil || strings.TrimSpace(*auth.Header) == "" {
		return policies
	}

	header := strings.TrimSpace(*auth.Header)
	headerParam := map[string]interface{}{"name": header}
	switch {
	case auth.SecretRef != nil && *auth.SecretRef != "":
		headerParam["secretRef"] = *auth.SecretRef
	case auth.Value != nil && *auth.Value != "":
		headerParam["value"] = *auth.Value
	default:
		return policies
	}

	out := make([]models.MCPPolicy, 0, len(policies)+1)
	out = append(out, policies...)
	out = append(out, models.MCPPolicy{
		Name:        mcpBackendAuthPolicyName,
		Version:     mcpBackendAuthPolicyVersion,
		DisplayName: mcpBackendAuthPolicyDisplayName,
		Params: map[string]interface{}{
			"request": map[string]interface{}{
				"headers": []interface{}{headerParam},
			},
		},
	})
	return out
}

func appendMCPAPIKeyAuthPolicy(policies []models.MCPPolicy, security *models.SecurityConfig) ([]models.MCPPolicy, error) {
	if security == nil || !isBoolTrue(security.Enabled) {
		return policies, nil
	}
	if security.APIKey == nil || !isBoolTrue(security.APIKey.Enabled) {
		return policies, nil
	}

	key := strings.TrimSpace(security.APIKey.Key)
	if key == "" {
		return nil, fmt.Errorf("invalid api key security configuration: key is required")
	}

	in := strings.ToLower(strings.TrimSpace(security.APIKey.In))
	if in != "header" && in != "query" {
		return nil, fmt.Errorf("invalid api key security configuration: in must be 'header' or 'query', got %q", security.APIKey.In)
	}

	out := make([]models.MCPPolicy, 0, len(policies)+1)
	out = append(out, policies...)
	out = append(out, models.MCPPolicy{
		Name:    apiKeyAuthPolicyName,
		Version: apiKeyAuthPolicyVersion,
		Params: map[string]interface{}{
			"key": key,
			"in":  in,
		},
	})
	return out, nil
}

func normalizeMCPPoliciesForDeployment(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}

	out := make([]models.MCPPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, models.MCPPolicy{
			Name:               policy.Name,
			Version:            normalizePolicyVersionToMajor(policy.Version),
			DisplayName:        policy.DisplayName,
			ExecutionCondition: policy.ExecutionCondition,
			Params:             policy.Params,
		})
	}
	return out
}

func mergeMCPPoliciesForDeployment(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) < 2 {
		return policies
	}

	out := make([]models.MCPPolicy, 0, len(policies))
	policyIndexes := map[string]int{}
	for _, policy := range policies {
		key := mcpPolicyIdentityKey(policy)
		existingIndex, ok := policyIndexes[key]
		if !ok {
			policyIndexes[key] = len(out)
			out = append(out, policy)
			continue
		}

		switch mcpPolicyMergeStrategyFor(policy.Name) {
		case mcpPolicyMergeStrategyMerge:
			out[existingIndex] = mergeMCPPolicyForDeployment(out[existingIndex], policy)
		default:
			out[existingIndex] = policy
		}
	}
	return out
}

func mcpPolicyIdentityKey(policy models.MCPPolicy) string {
	return strings.TrimSpace(policy.Name) + "\x00" + strings.TrimSpace(policy.Version)
}

func mcpPolicyMergeStrategyFor(policyName string) mcpPolicyMergeStrategy {
	if strategy, ok := mcpPolicyMergeStrategies[strings.TrimSpace(policyName)]; ok {
		return strategy
	}
	return mcpPolicyMergeStrategyOverride
}

func mergeMCPPolicyForDeployment(base, next models.MCPPolicy) models.MCPPolicy {
	merged := next
	merged.Params = mergeMCPPolicyParams(base.Params, next.Params)
	return merged
}

func mergeMCPPolicyParams(base, next map[string]interface{}) map[string]interface{} {
	if len(base) == 0 {
		return next
	}
	if len(next) == 0 {
		return base
	}

	out := cloneStringInterfaceMap(base)
	for key, nextValue := range next {
		baseValue, exists := out[key]
		if !exists {
			out[key] = nextValue
			continue
		}
		out[key] = mergeMCPPolicyParamValue(baseValue, nextValue)
	}
	return out
}

func mergeMCPPolicyParamValue(base, next interface{}) interface{} {
	baseMap, baseMapOK := base.(map[string]interface{})
	nextMap, nextMapOK := next.(map[string]interface{})
	if baseMapOK && nextMapOK {
		return mergeMCPPolicyParams(baseMap, nextMap)
	}

	baseBool, baseBoolOK := base.(bool)
	nextBool, nextBoolOK := next.(bool)
	if baseBoolOK && nextBoolOK {
		return baseBool || nextBool
	}

	if merged, ok := mergeStringSlices(base, next); ok {
		return merged
	}

	if merged, ok := mergeInterfaceSlices(base, next); ok {
		return merged
	}

	return next
}

func mergeStringSlices(base, next interface{}) (interface{}, bool) {
	baseStrings, baseOK := base.([]string)
	nextStrings, nextOK := next.([]string)
	if !baseOK || !nextOK {
		return nil, false
	}

	out := make([]string, 0, len(baseStrings)+len(nextStrings))
	seen := map[string]struct{}{}
	for _, value := range append(baseStrings, nextStrings...) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, true
}

func mergeInterfaceSlices(base, next interface{}) (interface{}, bool) {
	baseItems, baseOK := base.([]interface{})
	nextItems, nextOK := next.([]interface{})
	if !baseOK || !nextOK {
		return nil, false
	}

	out := make([]interface{}, 0, len(baseItems)+len(nextItems))
	out = append(out, baseItems...)
	for _, item := range nextItems {
		if stringValue, ok := item.(string); ok && containsStringInterface(out, stringValue) {
			continue
		}
		out = append(out, item)
	}
	return out, true
}

func containsStringInterface(items []interface{}, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func cloneStringInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func normalizeMCPUpstreamURLForDeployment(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return trimmed
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		return trimmed
	}
	segments := strings.Split(path, "/")
	if len(segments) == 0 || segments[len(segments)-1] != "mcp" {
		return trimmed
	}

	segments = segments[:len(segments)-1]
	parsed.Path = strings.Join(segments, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawPath = ""
	return parsed.String()
}

func deploymentName(proxy *models.MCPProxy) string {
	if strings.TrimSpace(proxy.Handle) != "" {
		return fmt.Sprintf("%s-deployment", proxy.Handle)
	}
	if proxy.Artifact != nil && strings.TrimSpace(proxy.Artifact.Handle) != "" {
		return fmt.Sprintf("%s-deployment", proxy.Artifact.Handle)
	}
	return fmt.Sprintf("%s-deployment", proxy.UUID.String())
}
