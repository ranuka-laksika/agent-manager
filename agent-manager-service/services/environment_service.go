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
	"log/slog"
	"sync"

	"github.com/google/uuid"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentService defines the interface for environment operations
type EnvironmentService interface {
	CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error)
	ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error)
	UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	DeleteEnvironment(ctx context.Context, ouID string, envID string) error
	GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error)
	ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error)
}

type environmentService struct {
	logger             *slog.Logger
	ocClient           occlient.OpenChoreoClient
	gatewayRepo        repositories.GatewayRepository
	thunderProber      thundersvc.Prober
	agentConfigService AgentConfigurationService
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, gatewayRepo repositories.GatewayRepository, ocClient occlient.OpenChoreoClient, thunderProber thundersvc.Prober, agentConfigService AgentConfigurationService) EnvironmentService {
	return &environmentService{
		logger:             logger,
		gatewayRepo:        gatewayRepo,
		ocClient:           ocClient,
		thunderProber:      thunderProber,
		agentConfigService: agentConfigService,
	}
}

func (s *environmentService) CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Creating environment in OpenChoreo", "name", req.Name, "ouID", ouID)

	if req.DataplaneRef == "" {
		s.logger.Warn("No dataplaneRef provided", "name", req.Name, "ouID", ouID)
	}

	// Reject unsupported isolation tiers up front. An unknown value would otherwise be
	// persisted verbatim on the Environment CR and silently fall back to runc at render
	// time (runtimeClassForIsolationTier returns "" for anything unrecognised), hiding the
	// typo from the user until they notice their agents are not actually isolated.
	switch req.IsolationTier {
	case "", "gvisor", "kata":
	default:
		return nil, fmt.Errorf("%w: unsupported isolation tier %q (allowed: gvisor, kata)", utils.ErrInvalidInput, req.IsolationTier)
	}

	ocReq := occlient.CreateEnvironmentRequest{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		IsolationTier: req.IsolationTier,
		DataplaneRef:  req.DataplaneRef,
		IsProduction:  req.IsProduction,
		Gateway:       toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.CreateEnvironment(ctx, ouID, ocReq)
	if err != nil {
		s.logger.Error("Failed to create environment in OpenChoreo", "ouID", ouID, "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      req.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment from OpenChoreo", "envID", envID, "ouID", ouID)

	// envID in this context is the environment name (not UUID)
	// since OpenChoreo API uses environment name as identifier
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		// Check if it's a not-found error
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert OpenChoreo EnvironmentResponse to GatewayEnvironmentResponse
	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      env.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		DNSPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.logger.Info("Listing environments from OpenChoreo", "ouID", ouID, "limit", limit, "offset", offset)

	// Fetch environments directly from OpenChoreo
	ocEnvironments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list environments from OpenChoreo", "ouID", ouID, "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	total := int32(len(ocEnvironments))

	// Apply pagination
	start := int(offset)
	end := start + int(limit)

	if start >= len(ocEnvironments) {
		// Offset is beyond available data
		return &models.EnvironmentListResponse{
			Environments: []models.GatewayEnvironmentResponse{},
			Total:        total,
			Limit:        limit,
			Offset:       offset,
		}, nil
	}

	if end > len(ocEnvironments) {
		end = len(ocEnvironments)
	}

	paginatedEnvs := ocEnvironments[start:end]

	// Convert OpenChoreo environment responses to gateway environment responses
	responses := make([]models.GatewayEnvironmentResponse, len(paginatedEnvs))
	for i, env := range paginatedEnvs {
		responses[i] = models.GatewayEnvironmentResponse{
			UUID:             env.UUID,
			OrganizationName: ouID,
			Name:             env.Name,
			DisplayName:      env.DisplayName,
			Description:      env.Description,
			IsolationTier:    env.IsolationTier,
			DataplaneRef:     env.DataplaneRef,
			DNSPrefix:        env.DNSPrefix,
			IsProduction:     env.IsProduction,
			Gateway:          env.Gateway,
			CreatedAt:        env.CreatedAt,
			UpdatedAt:        env.CreatedAt,
		}
	}

	return &models.EnvironmentListResponse{
		Environments: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (s *environmentService) UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Updating environment in OpenChoreo", "envID", envID, "ouID", ouID)

	ocReq := occlient.UpdateEnvironmentRequest{
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		IsProduction: req.IsProduction,
		Gateway:      toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.UpdateEnvironment(ctx, ouID, envID, ocReq)
	if err != nil {
		s.logger.Error("Failed to update environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      description,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

// DeleteEnvironment removes an environment from OpenChoreo and cleans up local DB state.
//
// OpenChoreo is the source of truth for "is anything actually deployed here": it refuses
// Environment deletion while ReleaseBindings or workloads still reference it, so we let
// the OC API server enforce that and surface whatever it returns.
//
// On success: delete the OpenChoreo Environment CR, then delete any gateway↔env mapping rows
func (s *environmentService) DeleteEnvironment(ctx context.Context, ouID string, envID string) error {
	s.logger.Info("Deleting environment", "envID", envID, "ouID", ouID)

	// envID is the environment name (matching OpenChoreo's identifier); resolve the UUID via OC
	// because the local DB doesn't have its own environments table.
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to look up environment", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return utils.ErrEnvironmentNotFound
		}
		return fmt.Errorf("failed to look up environment: %w", err)
	}

	envUUID, parseErr := uuid.Parse(env.UUID)
	if parseErr != nil {
		s.logger.Error("Invalid env UUID from OpenChoreo", "uuid", env.UUID, "error", parseErr)
		return fmt.Errorf("invalid environment UUID: %w", parseErr)
	}

	// Block deletion if any deployment pipeline still references this environment in its
	// promotion paths (either as a source or a target environment).
	pipelines, err := s.ocClient.ListDeploymentPipelines(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list deployment pipelines while checking environment references", "ouID", ouID, "envID", envID, "error", err)
		return fmt.Errorf("failed to verify environment references: %w", err)
	}
	var referencingPipelines []string
	for _, pipeline := range pipelines {
		if pipeline != nil && pipelineReferencesEnvironment(pipeline, env.Name) {
			referencingPipelines = append(referencingPipelines, pipeline.Name)
		}
	}
	if len(referencingPipelines) > 0 {
		s.logger.Warn("Cannot delete environment referenced by deployment pipelines", "ouID", ouID, "envID", envID, "pipelines", referencingPipelines)
		return fmt.Errorf("%w: %v", utils.ErrEnvironmentInUse, referencingPipelines)
	}

	// Delete in OpenChoreo first. If OC refuses (release bindings still exist, etc.) we surface
	// that error without having touched local state. A not-found from OC after UUID resolution
	// is treated as idempotent so we still clean up local gateway↔env mappings.
	if err := s.ocClient.DeleteEnvironment(ctx, ouID, env.Name); err != nil {
		s.logger.Error("Failed to delete environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			s.logger.Warn("Environment already absent in OpenChoreo; continuing local cleanup",
				"ouID", ouID, "envID", envID, "envUUID", envUUID)
		} else {
			return fmt.Errorf("failed to delete environment in OpenChoreo: %w", err)
		}
	}

	// Cascade MCP-proxy cleanup: tear down every agent-scoped MCP mapping deployed into this
	// env and strip the env block from every org-level MCP proxy blueprint. Best-effort — the
	// environment is already gone from OpenChoreo, so cleanup errors are logged, not fatal.
	if s.agentConfigService != nil {
		if err := s.agentConfigService.CleanupEnvironmentMCPArtifacts(ctx, ouID, envUUID, env.Name); err != nil {
			s.logger.Warn("environment deleted; MCP artifact cleanup had errors",
				"ouID", ouID, "envID", envID, "envUUID", envUUID, "error", err)
		}
	}

	// Local cleanup: gateway↔env mapping rows. The gateway themselves are unaffected.
	deleted, err := s.gatewayRepo.DeleteEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Environment deleted in OpenChoreo but local gateway-mapping cleanup failed",
			"envUUID", envUUID, "error", err)
		return fmt.Errorf("environment deleted but gateway mapping cleanup failed: %w", err)
	}
	s.logger.Info("Deleted environment", "envID", envID, "envUUID", envUUID, "gatewayMappingsDeleted", deleted)
	return nil
}

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "ouID", ouID)

	// Verify environment exists in OpenChoreo (envID is environment name)
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to verify environment: %w", err)
	}

	// Parse environment UUID
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		s.logger.Error("Failed to parse environment UUID", "uuid", env.UUID, "error", err)
		return nil, fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Get gateway-environment mappings from repository
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Failed to get gateway mappings from repository", "environmentID", envUUID.String(), "error", err)
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from the gateway repository
	responses := make([]models.GatewayResponse, 0, len(mappings))
	for _, mapping := range mappings {
		gatewayID := mapping.GatewayUUID.String()

		// Get gateway details from repository
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway from repository", "gatewayID", gatewayID, "error", err)
			continue
		}
		if gateway == nil {
			s.logger.Warn("Gateway not found", "gatewayID", gatewayID)
			continue
		}

		// Convert gateway model to response
		status := string(models.GatewayStatusInactive)
		if gateway.IsActive {
			status = string(models.GatewayStatusActive)
		}

		responses = append(responses, models.GatewayResponse{
			UUID:             gateway.UUID.String(),
			OrganizationName: ouID,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
		})
	}

	return responses, nil
}

// pipelineReferencesEnvironment reports whether any promotion path in the pipeline
// references the given environment name, either as the source or as a target.
func pipelineReferencesEnvironment(pipeline *models.DeploymentPipelineResponse, envName string) bool {
	for _, path := range pipeline.PromotionPaths {
		if path.SourceEnvironmentRef == envName {
			return true
		}
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name == envName {
				return true
			}
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Gateway spec translation: models.GatewaySpec (internal DTO) → occlient
// GatewaySpec (OC-bound request type). Direct field-by-field copy; the OC
// client layer constructs the runtime-only fields (gateway resource name/
// namespace, listener name).
// -----------------------------------------------------------------------------

func toOCClientGatewaySpec(g *models.GatewaySpec) *occlient.GatewaySpec {
	if g == nil {
		return nil
	}
	return &occlient.GatewaySpec{
		Ingress: toOCClientGatewayNetworkSpec(g.Ingress),
		Egress:  toOCClientGatewayNetworkSpec(g.Egress),
	}
}

func toOCClientGatewayNetworkSpec(n *models.GatewayNetworkSpec) *occlient.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	return &occlient.GatewayNetworkSpec{
		External: toOCClientGatewayEndpointSpec(n.External),
		Internal: toOCClientGatewayEndpointSpec(n.Internal),
	}
}

func toOCClientGatewayEndpointSpec(e *models.GatewayEndpointSpec) *occlient.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	return &occlient.GatewayEndpointSpec{
		HTTP:  toOCClientGatewayListenerSpec(e.HTTP),
		HTTPS: toOCClientGatewayListenerSpec(e.HTTPS),
		TLS:   toOCClientGatewayListenerSpec(e.TLS),
	}
}

func toOCClientGatewayListenerSpec(l *models.GatewayListenerSpec) *occlient.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &occlient.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}

// ListThunderInstances returns the Thunder OAuth2 identity provider info for every
// environment in the org whose env-Thunder instance is reachable.
//
// Reachability is determined by a live HTTP probe to the environment's JWKS endpoint,
// NOT by inferring from gateway mappings. Gateway mappings only prove a gateway was
// provisioned — not that Thunder was ever deployed (e.g. PROVISION_THUNDER=false,
// a failed provision, or an environment created before this feature was added all have
// mappings but no Thunder instance). Advertising dead endpoints would cause the console
// Identity page to show broken issuer/token/JWKS URLs.
func (s *environmentService) ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error) {
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("list environments for org %s: %w", ouID, err)
	}

	// Thunder naming (release name, namespace, host, issuer URL) is keyed by the
	// org NAME the provisioning scripts deployed with (ORG_NAME, e.g. "default"),
	// which is the OpenChoreo namespace — not the OU id from the JWT. Probing or
	// advertising URLs derived from the OU id would point at instances that don't
	// exist.
	orgNamespace, err := ResolveNamespace(ctx, s.ocClient)
	if err != nil {
		return nil, err
	}

	// Probe every environment's env-Thunder JWKS endpoint concurrently. Each probe can take
	// up to ~8s in the worst case (ThunderProbe's 4-step fallback chain, 2s timeout each), so
	// probing sequentially would scale request latency linearly with environment count.
	reachable := make([]bool, len(envs))
	var wg sync.WaitGroup
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, envName string) {
			defer wg.Done()
			reachable[idx] = s.thunderProber.Probe(ctx, orgNamespace, envName)
		}(i, env.Name)
	}
	wg.Wait()

	instances := make([]models.ThunderInstanceResponse, 0, len(envs))
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}

		// Reachability is the only reliable signal: environments created with PROVISION_THUNDER=false,
		// environments whose provisioning failed silently (non-fatal by design), and
		// pre-PR environments all pass the gateway-mappings check but have no Thunder instance.
		if !reachable[i] {
			s.logger.Debug("env-Thunder not reachable, skipping", "envName", env.Name)
			continue
		}

		// All three URLs must be externally reachable — this response is developer-facing
		// (console Identity page, copy-buttons). The internal svc.cluster.local addresses
		// only resolve inside the cluster and would cause DNS failures for developers.
		// The env-Thunder HTTPRoute routes http://<org>-<env>.thunder.amp.localhost:8080
		// to the Thunder service, so all /oauth2/* paths are reachable via ThunderHost.
		instances = append(instances, models.ThunderInstanceResponse{
			EnvName:      env.Name,
			DisplayName:  env.DisplayName,
			IsProduction: env.IsProduction,
			IssuerURL:    thundersvc.ThunderIssuerURL(orgNamespace, env.Name),
			TokenURL:     thundersvc.ThunderExternalTokenURL(orgNamespace, env.Name),
			JWKSURL:      thundersvc.ThunderExternalJWKSURL(orgNamespace, env.Name),
			Namespace:    thundersvc.ThunderNamespace(orgNamespace, env.Name),
		})
	}
	return &models.ThunderInstanceListResponse{ThunderInstances: instances}, nil
}
