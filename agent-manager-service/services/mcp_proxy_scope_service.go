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
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyScopeService manages per-proxy MCP scopes: validation, CRUD, and
// (in later tasks) Thunder cleanup + re-emit.
type MCPProxyScopeService interface {
	List(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxyScopesResult, error)
	Create(ctx context.Context, ouID, orgName, proxyHandle string, in models.MCPProxyScopeInput) (*models.MCPProxyScopeResult, error)
	Update(ctx context.Context, ouID, orgName, proxyHandle, action string, in models.MCPProxyScopeUpdateInput) (*models.MCPProxyScopeResult, error)
	Delete(ctx context.Context, ouID, orgName, proxyHandle, action string) error
	ListEnvironmentScopes(ctx context.Context, ouID, envName string) ([]models.EnvironmentScopeEntry, error)
}

type mcpProxyScopeService struct {
	scopeRepo      repositories.MCPProxyScopeRepository
	proxyRepo      repositories.MCPProxyRepository
	deploymentRepo repositories.DeploymentRepository
	infraManager   InfraResourceManager
	resolver       thundersvc.EnvThunderResolver
	proxySvc       *MCPProxyService
	logger         *slog.Logger
}

// NewMCPProxyScopeService creates a new per-proxy MCP scope service.
func NewMCPProxyScopeService(
	scopeRepo repositories.MCPProxyScopeRepository,
	proxyRepo repositories.MCPProxyRepository,
	deploymentRepo repositories.DeploymentRepository,
	infraManager InfraResourceManager,
	resolver thundersvc.EnvThunderResolver,
	proxySvc *MCPProxyService,
	logger *slog.Logger,
) MCPProxyScopeService {
	return &mcpProxyScopeService{
		scopeRepo:      scopeRepo,
		proxyRepo:      proxyRepo,
		deploymentRepo: deploymentRepo,
		infraManager:   infraManager,
		resolver:       resolver,
		proxySvc:       proxySvc,
		logger:         logger,
	}
}

// mcpScopeActionRe constrains scope actions. No ":" (the scope string splits on the
// first ":") and no "/" (Thunder would accept it; we keep scope strings flat).
// 100 mirrors Thunder's per-handle cap; the combined permission column allows 1000.
var mcpScopeActionRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,100}$`)

// proxyHandleOf returns the proxy's artifact handle, the authoritative source
// (models.MCPProxy.Handle is a gorm:"-" transient that repos never populate).
func proxyHandleOf(proxy *models.MCPProxy) string {
	if proxy.Artifact != nil && proxy.Artifact.Handle != "" {
		return proxy.Artifact.Handle
	}
	return proxy.Handle
}

func (s *mcpProxyScopeService) resolveProxy(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxy, error) {
	proxy, err := s.proxyRepo.GetByHandle(ctx, proxyHandle, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: proxy handle %q", utils.ErrMCPProxyNotFound, proxyHandle)
		}
		return nil, fmt.Errorf("failed to resolve MCP proxy %q: %w", proxyHandle, err)
	}
	return proxy, nil
}

// proxyToolUnion collects every discovered tool name across the proxy's
// endpoints. An empty map means "no capabilities stored" — validation is then
// permissive (strict-when-known policy).
func proxyToolUnion(proxy *models.MCPProxy) map[string]struct{} {
	union := map[string]struct{}{}
	for i := range proxy.Endpoints {
		caps := proxy.Endpoints[i].Configuration.Capabilities
		if caps == nil || caps.Tools == nil {
			continue
		}
		for _, tool := range *caps.Tools {
			if name, ok := tool["name"].(string); ok && name != "" {
				union[name] = struct{}{}
			}
		}
	}
	return union
}

func validateScopeTools(tools []string, union map[string]struct{}) ([]string, error) {
	if len(tools) == 0 {
		return nil, fmt.Errorf("%w: a scope must authorize at least one tool", utils.ErrInvalidInput)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tools))
	for _, tl := range tools {
		tl = strings.TrimSpace(tl)
		if tl == "" {
			return nil, fmt.Errorf("%w: tool names must be non-empty", utils.ErrInvalidInput)
		}
		if _, dup := seen[tl]; dup {
			return nil, fmt.Errorf("%w: duplicate tool %q", utils.ErrInvalidInput, tl)
		}
		if len(union) > 0 {
			if _, known := union[tl]; !known {
				return nil, fmt.Errorf("%w: tool %q is not among the proxy's discovered tools", utils.ErrInvalidInput, tl)
			}
		}
		seen[tl] = struct{}{}
		out = append(out, tl)
	}
	sort.Strings(out)
	return out, nil
}

func (s *mcpProxyScopeService) Create(ctx context.Context, ouID, orgName, proxyHandle string, in models.MCPProxyScopeInput) (*models.MCPProxyScopeResult, error) {
	_ = orgName // Task 11 wires Thunder cleanup + re-emit

	if !mcpScopeActionRe.MatchString(in.Action) {
		return nil, fmt.Errorf("%w: action must match ^[A-Za-z0-9._\\-]{1,100}$", utils.ErrInvalidInput)
	}

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	if _, err := s.scopeRepo.Get(ctx, proxy.UUID, in.Action); err == nil {
		return nil, fmt.Errorf("%w: scope action %q already exists on proxy %q", utils.ErrConflict, in.Action, proxyHandle)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check scope existence: %w", err)
	}

	union := proxyToolUnion(proxy)
	tools, err := validateScopeTools(in.Tools, union)
	if err != nil {
		return nil, err
	}

	scope := &models.MCPProxyScope{
		MCPProxyUUID: proxy.UUID,
		Action:       in.Action,
		Description:  in.Description,
		Tools:        tools,
	}
	if err := s.scopeRepo.Create(ctx, scope); err != nil {
		// A concurrent create can win the race between the Get check above and
		// this insert; the unique constraint (uq_mcp_proxy_scopes_proxy_action)
		// then fires. Map it to a conflict so the API returns 409.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: scope action %q already exists on proxy %q", utils.ErrConflict, in.Action, proxyHandle)
		}
		return nil, fmt.Errorf("failed to create mcp proxy scope: %w", err)
	}

	return &models.MCPProxyScopeResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scope:       *scope,
	}, nil
}

func (s *mcpProxyScopeService) Update(ctx context.Context, ouID, orgName, proxyHandle, action string, in models.MCPProxyScopeUpdateInput) (*models.MCPProxyScopeResult, error) {
	_ = orgName // Task 11 wires Thunder cleanup + re-emit

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	existing, err := s.scopeRepo.Get(ctx, proxy.UUID, action)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrScopeNotFound
		}
		return nil, fmt.Errorf("failed to load scope: %w", err)
	}

	if in.Description != nil {
		existing.Description = *in.Description
	}
	if in.Tools != nil {
		union := proxyToolUnion(proxy)
		tools, err := validateScopeTools(in.Tools, union)
		if err != nil {
			return nil, err
		}
		existing.Tools = tools
	}

	if err := s.scopeRepo.Update(ctx, existing); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrScopeNotFound
		}
		return nil, fmt.Errorf("failed to update mcp proxy scope: %w", err)
	}

	return &models.MCPProxyScopeResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scope:       *existing,
	}, nil
}

func (s *mcpProxyScopeService) Delete(ctx context.Context, ouID, orgName, proxyHandle, action string) error {
	_ = orgName // Task 11 wires Thunder cleanup + re-emit

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return err
	}

	if err := s.scopeRepo.Delete(ctx, proxy.UUID, action); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrScopeNotFound
		}
		return fmt.Errorf("failed to delete mcp proxy scope: %w", err)
	}
	return nil
}

func (s *mcpProxyScopeService) List(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxyScopesResult, error) {
	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	scopes, err := s.scopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}

	return &models.MCPProxyScopesResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scopes:      scopes,
	}, nil
}

func (s *mcpProxyScopeService) ListEnvironmentScopes(ctx context.Context, ouID, envName string) ([]models.EnvironmentScopeEntry, error) {
	_ = ctx
	_ = ouID
	_ = envName
	// Task 6 implements the env-filtered scope aggregate.
	return []models.EnvironmentScopeEntry{}, nil
}
