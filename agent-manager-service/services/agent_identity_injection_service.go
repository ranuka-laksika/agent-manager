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
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// secretRotatedAtAnnotation is stamped (with a fresh timestamp) on the
// SecretReference's Secret template after a credential rotation. The value
// change is a real CR spec change, which forces the OpenChoreo controller to
// re-sync the Kubernetes Secret from OpenBao immediately — without it, the
// rotated value would only land at the next RefreshInterval tick, and the
// rollout below would restart the pod onto the STALE secret.
const secretRotatedAtAnnotation = "amp.wso2.com/secret-rotated-at"

// secretRotatedAtFormat is nanosecond-precision (not time.RFC3339, which
// only has second resolution) specifically so two rotations issued within
// the same second still produce distinct annotation values — otherwise the
// second rotation would write a byte-identical annotation, defeating the
// whole point of stamping it (nothing for the controller to notice as a
// spec change) and leaving the pod rolled out onto the FIRST rotation's
// already-invalidated secret instead of the second, real one.
const secretRotatedAtFormat = time.RFC3339Nano

// releaseBindingUpdateRetries/releaseBindingUpdateRetryDelay bound the retry
// of a ReleaseBinding/Workload env-var mutation. UpdateReleaseBindingEnvVars
// (and its Remove/RemoveWorkloadEnvVars siblings) do a plain Get-then-Update
// with no built-in retry, so a concurrent writer touching the SAME
// ReleaseBinding at the same moment (e.g. a user calling
// UpdateAgentConfigurations while this service injects/refreshes/removes
// identity vars) can lose an optimistic-concurrency race. Without a retry
// here, that loser's change is silently dropped with no recovery until the
// next unrelated deploy/promote/rotation happens to re-assert it.
const (
	releaseBindingUpdateRetries    = 3
	releaseBindingUpdateRetryDelay = 500 * time.Millisecond
)

// withReleaseBindingRetry retries fn a bounded number of times, sleeping
// between attempts. Every call site here already wraps a single, cheap,
// idempotent Get-then-Update — retrying is always safe.
func withReleaseBindingRetry(fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < releaseBindingUpdateRetries; attempt++ {
		if lastErr = fn(); lastErr == nil {
			return nil
		}
		if attempt < releaseBindingUpdateRetries-1 {
			time.Sleep(releaseBindingUpdateRetryDelay)
		}
	}
	return lastErr
}

// agentIdentitySecretRefKind distinguishes the AgentID SecretReference name
// from the "<entity>-<env>-secrets" names secretmanagersvc generates for
// LLM/MCP/user secrets, so the two families can never collide.
const agentIdentitySecretRefSuffix = "agent-identity"

// AgentIdentityEnvVarKeys returns the set of env var names owned by AgentID
// credential injection. Callers that rewrite an agent's env vars from user
// input use this to filter user-echoed copies (the console reads
// configurations back, so these keys can legitimately round-trip in requests)
// before re-appending the canonical values.
func AgentIdentityEnvVarKeys() map[string]bool {
	return map[string]bool{
		client.EnvVarAgentIdentityClientID:      true,
		client.EnvVarAgentIdentityClientSecret:  true,
		client.EnvVarAgentIdentityTokenEndpoint: true,
		client.EnvVarAgentIdentityScopes:        true,
	}
}

// AgentIdentityInjectionService delivers an internal agent's AgentID
// credentials (Thunder OAuth2 client) into its running workload — the
// "Gateway Binding" phase of the AgentID feature. It never handles the secret
// VALUE itself: the pod receives the client secret through a SecretKeyRef into
// the Kubernetes Secret that OpenChoreo materializes from a SecretReference CR
// pointing at the OpenBao path AgentSecretStore already writes.
//
// External agents are never injected — they run outside the platform and
// retrieve credentials via the one-time claim flow instead. Every method here
// silently no-ops for them.
type AgentIdentityInjectionService interface {
	// EnvVarsForEnvironment returns the identity env vars for one internal
	// agent in one environment, ensuring the backing SecretReference CR exists
	// first. Returns (nil, nil) when there is nothing to inject: no binding,
	// provisioning not completed, an external agent, or a revoked credential.
	// Callers treat nil as "skip identity injection" — a normal state, not an
	// error. A non-nil error means the current state could not be determined
	// (or the SecretReference could not be ensured) and the caller must NOT
	// proceed with an env-var rewrite that would silently drop the vars.
	EnvVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) ([]client.EnvVar, error)

	// InjectForEnvironment pushes the identity env vars into the agent's live
	// workload for one environment (ReleaseBinding merge + pod rollout).
	// No-op when EnvVarsForEnvironment returns nothing, and when the agent is
	// not deployed in that environment yet (the deploy flow injects at deploy
	// time). Used by the live-refresh hooks (MCP config / proxy scope changes)
	// and, via ReconcileForEnvironment, by the injection reconciler.
	InjectForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error

	// ReconcileForEnvironment idempotently brings the agent's live workload in
	// line with its desired AgentID env vars, writing ONLY when they are
	// missing or the scope list drifted — so it never causes a needless pod
	// rollout (mirrors the LLM/MCP paths, which also skip an unchanged
	// ReleaseBinding update). Reads the current env vars back via
	// GetComponentConfigurations to decide. Safe to run every reconciler tick:
	// this is what lands credentials on a brand-new git agent whose
	// ReleaseBinding doesn't exist yet when provisioning completes, the moment
	// its first build creates the workload. No-op for external/unprovisioned/
	// revoked bindings and for not-yet-deployed environments.
	ReconcileForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error

	// RefreshAfterRotation re-asserts the SecretReference CR with a fresh
	// rotated-at annotation (forcing OpenChoreo to re-read the rotated value
	// from OpenBao) and rolls the pod so it starts with the new secret.
	// No-op for external agents and unprovisioned bindings.
	RefreshAfterRotation(ctx context.Context, ouID, projectName, agentName, envName string) error

	// RemoveForEnvironment removes the identity env vars from the agent's
	// workload for one environment and deletes the backing SecretReference CR
	// — used after a revoke, so the pod does not keep serving a credential
	// that can no longer mint tokens. includeWorkloadLevel additionally
	// removes the vars from the shared Workload CR; callers set it only when
	// envName is the pipeline's lowest environment (the deploy flow writes
	// that environment's vars at Workload level, shared by all environments —
	// removing them while revoking a DIFFERENT environment's credential would
	// break the lowest environment's pod).
	RemoveForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, includeWorkloadLevel bool) error

	// CleanupForEnvironment deletes the AgentID SecretReference CR for one
	// (agent, environment) — used on agent deletion, where the workload itself
	// is being deleted so env var removal is pointless but the CR would leak.
	// Best-effort: not-found is success.
	CleanupForEnvironment(ctx context.Context, ouID, agentName, envName string) error
}

type agentIdentityInjectionService struct {
	repo                 repositories.AgentThunderClientRepository
	agentConfigRepo      repositories.AgentConfigurationRepository
	mcpProxyEndpointRepo repositories.MCPProxyEndpointRepository
	ocClient             client.OpenChoreoClient
	refreshInterval      string
	logger               *slog.Logger
	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

// NewAgentIdentityInjectionService creates a new AgentIdentityInjectionService.
// refreshInterval is the SecretReference refresh cadence (same value
// secretmanagersvc uses, e.g. "1h").
func NewAgentIdentityInjectionService(
	repo repositories.AgentThunderClientRepository,
	agentConfigRepo repositories.AgentConfigurationRepository,
	mcpProxyEndpointRepo repositories.MCPProxyEndpointRepository,
	ocClient client.OpenChoreoClient,
	refreshInterval string,
	logger *slog.Logger,
) AgentIdentityInjectionService {
	return &agentIdentityInjectionService{
		repo:                 repo,
		agentConfigRepo:      agentConfigRepo,
		mcpProxyEndpointRepo: mcpProxyEndpointRepo,
		ocClient:             ocClient,
		refreshInterval:      refreshInterval,
		logger:               logger,
		now:                  time.Now,
	}
}

// agentIdentityRefNameMaxLen is the Kubernetes object name length limit
// (DNS subdomain / RFC 1123).
const agentIdentityRefNameMaxLen = 63

// identityRefNameHashLen is deliberately short (this is a collision-avoidance
// suffix, not a security boundary) — just long enough that two different
// (agent, env) pairs sharing a truncated prefix essentially never produce
// the same suffix too.
const identityRefNameHashLen = 6

// AgentIdentitySecretRefName builds the SecretReference CR name for one
// (agent, environment): "<agent>-<env>-agent-identity", sanitized for
// Kubernetes naming and capped at 63 chars.
//
// If the plain form fits, it's returned as-is. If it doesn't, the prefix is
// truncated AND a 6-character hash of the full, untruncated "agent/env" pair
// is appended — plain truncation alone (the approach secretmanagersvc's own
// SecretRefName uses) can make two different long (agent, env) pairs that
// happen to share the same 63-character prefix collide onto the identical
// SecretReference name, silently corrupting one agent's credential delivery
// with another's KV path. This mirrors thundersvc's own
// ThunderReleaseName/ThunderHost collision-avoidance for the identical
// problem shape (truncate + hash-suffix), just implemented locally here
// since that helper isn't exported.
func AgentIdentitySecretRefName(agentName, envName string) string {
	agentSeg := sanitizeIdentityRefSegment(agentName)
	envSeg := sanitizeIdentityRefSegment(envName)
	name := fmt.Sprintf("%s-%s-%s", agentSeg, envSeg, agentIdentitySecretRefSuffix)
	if len(name) <= agentIdentityRefNameMaxLen {
		return name
	}

	hash := identityRefNameHash(agentName + "/" + envName)
	// Budget: total - "-" - hash - "-" - suffix, for the prefix built from
	// agentSeg+"-"+envSeg.
	maxPrefixLen := agentIdentityRefNameMaxLen - 1 - identityRefNameHashLen - 1 - len(agentIdentitySecretRefSuffix)
	prefix := sanitizeIdentityRefSegment(agentSeg + "-" + envSeg)
	if len(prefix) > maxPrefixLen {
		prefix = strings.TrimRight(prefix[:maxPrefixLen], "-")
	}
	return fmt.Sprintf("%s-%s-%s", prefix, hash, agentIdentitySecretRefSuffix)
}

// identityRefNameHash returns a short, deterministic, filesystem/DNS-safe
// hash of s (lowercase hex).
func identityRefNameHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:identityRefNameHashLen]
}

// sanitizeIdentityRefSegment converts s to a lowercase DNS-label-safe string,
// mirroring secretmanagersvc's sanitizeForK8sName (private there).
func sanitizeIdentityRefSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

// resolveAgentIdentityScopes returns the full set of OAuth2 scopes the agent
// should request when minting a token: the union of every catalog scope
// bound (via MCPToolScopeBinding) to a tool on any MCP proxy this agent is
// configured to use in this environment — sourced entirely from AMS's own
// DB, no Thunder role/group lookups. Requesting a scope the AgentID isn't
// actually authorized for is safe: Thunder filters requested scopes down to
// what the agent's role assignments actually grant at token-mint time, so
// this is a "what might I need" list, not the enforcement point.
//
// Fails closed: any lookup error (environment resolution, agent config load)
// logs a warning and returns nil (no scopes) rather than a stale or
// over-broad set. A transient DB/OpenChoreo blip costing an agent temporary
// MCP access is safe; silently keeping the wrong scopes is not.
func (s *agentIdentityInjectionService) resolveAgentIdentityScopes(ctx context.Context, binding *models.AgentThunderClient) []string {
	config, err := s.agentConfigRepo.GetByAgentID(ctx, binding.AgentName, binding.OUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // agent has no configuration yet — nothing to request
		}
		s.logger.Warn("resolve agent identity scopes: load agent configuration failed",
			"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
		return nil
	}
	if len(config.EnvMCPMappings) == 0 {
		return nil // no MCP bindings at all — skip resolving the environment UUID entirely
	}

	env, err := s.ocClient.GetEnvironment(ctx, binding.OUID, binding.EnvironmentName)
	if err != nil {
		s.logger.Warn("resolve agent identity scopes: resolve environment failed",
			"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
		return nil
	}

	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		s.logger.Warn("resolve agent identity scopes: invalid environment id",
			"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
		return nil
	}

	scopeSet := map[string]struct{}{}
	for _, mapping := range config.EnvMCPMappings {
		if mapping.EnvironmentUUID.String() != env.UUID {
			continue
		}
		// A proxy's per-environment tool-scope bindings live on the endpoint
		// deployed to this environment (MCPProxyEndpoint.Configuration), reached
		// via the endpoint<->environment join row — not on the proxy itself.
		ee, err := s.mcpProxyEndpointRepo.GetEndpointEnvByProxyAndEnv(ctx, mapping.MCPProxyUUID, envUUID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Warn("resolve agent identity scopes: resolve proxy endpoint for environment failed",
					"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
			}
			continue
		}
		endpoint, err := s.mcpProxyEndpointRepo.GetEndpoint(ctx, ee.EndpointUUID)
		if err != nil {
			s.logger.Warn("resolve agent identity scopes: load proxy endpoint failed",
				"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
			continue
		}
		for _, toolBinding := range endpoint.Configuration.ToolScopeBindings {
			for _, scope := range toolBinding.Scopes {
				if scope != "" {
					scopeSet[scope] = struct{}{}
				}
			}
		}
	}
	if len(scopeSet) == 0 {
		return nil
	}
	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes
}

// injectableBinding returns the binding when it is in an injectable state, or
// nil when there is (legitimately) nothing to inject for this agent+env.
func (s *agentIdentityInjectionService) injectableBinding(ctx context.Context, ouID, projectName, agentName, envName string) (*models.AgentThunderClient, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil, nil //nolint:nilnil // documented "nothing to inject" state, checked by every caller
		}
		return nil, fmt.Errorf("read agent thunder binding for identity injection: %w", err)
	}
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
		return nil, nil //nolint:nilnil // external agents are never injected
	}
	if binding.Status != models.AgentThunderStatusCompleted || binding.ThunderClientID == "" {
		return nil, nil //nolint:nilnil // provisioning not (successfully) finished yet
	}
	if binding.SecretRefPath == "" {
		return nil, nil //nolint:nilnil // credential revoked — nothing valid to inject
	}
	return binding, nil
}

// ensureSecretReference creates or updates the SecretReference CR that lets
// OpenChoreo materialize the OpenBao credential as a Kubernetes Secret.
// templateAnnotations may be nil; see secretRotatedAtAnnotation for when it
// is not.
func (s *agentIdentityInjectionService) ensureSecretReference(ctx context.Context, binding *models.AgentThunderClient, templateAnnotations map[string]string) (string, error) {
	refName := AgentIdentitySecretRefName(binding.AgentName, binding.EnvironmentName)
	req := client.CreateSecretReferenceRequest{
		Namespace:           binding.OUID,
		Name:                refName,
		ProjectName:         binding.ProjectName,
		ComponentName:       binding.AgentName,
		KVPath:              binding.SecretRefPath,
		SecretKeys:          []string{thundersvc.AgentSecretKeyClientSecret},
		RefreshInterval:     s.refreshInterval,
		TemplateAnnotations: templateAnnotations,
	}

	_, getErr := s.ocClient.GetSecretReference(ctx, binding.OUID, refName)
	if getErr != nil {
		if !errors.Is(getErr, utils.ErrNotFound) {
			return "", fmt.Errorf("check agent identity SecretReference %q: %w", refName, getErr)
		}
		if _, createErr := s.ocClient.CreateSecretReference(ctx, binding.OUID, req); createErr != nil {
			// A concurrent caller may have created it between Get and Create.
			if !errors.Is(createErr, utils.ErrConflict) {
				return "", fmt.Errorf("create agent identity SecretReference %q: %w", refName, createErr)
			}
			if _, updateErr := s.ocClient.UpdateSecretReference(ctx, binding.OUID, refName, req); updateErr != nil {
				return "", fmt.Errorf("update agent identity SecretReference %q after create conflict: %w", refName, updateErr)
			}
		}
		return refName, nil
	}

	if _, updateErr := s.ocClient.UpdateSecretReference(ctx, binding.OUID, refName, req); updateErr != nil {
		return "", fmt.Errorf("update agent identity SecretReference %q: %w", refName, updateErr)
	}
	return refName, nil
}

// buildEnvVars assembles the four identity env vars for one binding. The
// token endpoint uses the cluster-internal env-Thunder URL — pods run inside
// the cluster, matching the convention that internal agents reach platform
// services via in-cluster addresses (see buildProxyURL).
//
// The URL is built from ThunderOrgNamespace(), NOT binding.OUID: env-Thunder
// is addressed by the org's namespace/handle (e.g. "default"), and binding.OUID
// is the OU UUID from the JWT — passing it here would target a K8s Service
// that doesn't exist. See ThunderOrgNamespace's doc comment for why this is a
// config-pinned value shared with agentThunderProvisioningService, not an
// independent lookup.
func (s *agentIdentityInjectionService) buildEnvVars(ctx context.Context, binding *models.AgentThunderClient, secretRefName string) []client.EnvVar {
	scopes := s.resolveAgentIdentityScopes(ctx, binding)
	return []client.EnvVar{
		{Key: client.EnvVarAgentIdentityClientID, Value: binding.ThunderClientID},
		{
			Key: client.EnvVarAgentIdentityClientSecret,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: secretRefName,
					Key:  thundersvc.AgentSecretKeyClientSecret,
				},
			},
		},
		{Key: client.EnvVarAgentIdentityTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), binding.EnvironmentName)},
		{Key: client.EnvVarAgentIdentityScopes, Value: strings.Join(scopes, " ")},
	}
}

func (s *agentIdentityInjectionService) envVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, templateAnnotations map[string]string) ([]client.EnvVar, error) {
	binding, err := s.injectableBinding(ctx, ouID, projectName, agentName, envName)
	if err != nil || binding == nil {
		return nil, err
	}
	refName, err := s.ensureSecretReference(ctx, binding, templateAnnotations)
	if err != nil {
		return nil, err
	}
	return s.buildEnvVars(ctx, binding, refName), nil
}

func (s *agentIdentityInjectionService) EnvVarsForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) ([]client.EnvVar, error) {
	return s.envVarsForEnvironment(ctx, ouID, projectName, agentName, envName, nil)
}

func (s *agentIdentityInjectionService) InjectForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error {
	envVars, err := s.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	if len(envVars) == 0 {
		s.logger.Debug("No agent identity env vars to inject", "agentName", agentName, "envName", envName)
		return nil
	}
	// Merges into the ReleaseBinding's workload overrides and stamps
	// restartedAt for a pod rollout; silently no-ops when the agent is not
	// deployed in this environment yet (kind-sourced agents, which have no
	// ReleaseBinding, pick the vars up on their next deploy instead).
	if err := withReleaseBindingRetry(func() error {
		return s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, envVars)
	}); err != nil {
		return fmt.Errorf("inject agent identity env vars into release binding: %w", err)
	}
	s.logger.Info("Injected agent identity env vars", "agentName", agentName, "envName", envName)
	return nil
}

func (s *agentIdentityInjectionService) ReconcileForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error {
	desired, err := s.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	if len(desired) == 0 {
		// External/unprovisioned/revoked — nothing this service should inject.
		return nil
	}

	current, err := s.ocClient.GetComponentConfigurations(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return fmt.Errorf("read current agent env vars for identity reconcile: %w", err)
	}
	if identityEnvVarsInSync(desired, current) {
		// Live workload already carries the identity vars with the current
		// scope list — writing would only stamp a needless pod rollout.
		return nil
	}

	// Something is missing or stale (most commonly: the workload just came up
	// from a first build that finished after provisioning, so it has none of
	// the identity vars yet). InjectForEnvironment writes them and rolls the
	// pod; it still safely no-ops if the ReleaseBinding does not exist yet.
	return s.InjectForEnvironment(ctx, ouID, projectName, agentName, envName)
}

// identityEnvVarsInSync reports whether the live env vars already carry every
// AgentID key AND the scope list already matches desired. Client ID / secret
// ref / token endpoint are stable for a given (agent, environment) once set
// (rotation re-asserts them through RefreshAfterRotation, a separate path), so
// their mere presence is enough; the scope list is the value that legitimately
// changes over time, so it is compared exactly. Absent the workload entirely
// (nothing read back), this returns false so the caller attempts an inject —
// which itself no-ops when there is no ReleaseBinding to write to.
func identityEnvVarsInSync(desired []client.EnvVar, current []models.EnvVars) bool {
	currentByKey := make(map[string]models.EnvVars, len(current))
	for _, ev := range current {
		currentByKey[ev.Key] = ev
	}
	for _, want := range desired {
		got, ok := currentByKey[want.Key]
		if !ok {
			return false
		}
		// The scope list is the only identity var whose value changes in place;
		// compare it exactly so scope edits re-inject.
		if want.Key == client.EnvVarAgentIdentityScopes && got.Value != want.Value {
			return false
		}
	}
	return true
}

func (s *agentIdentityInjectionService) RefreshAfterRotation(ctx context.Context, ouID, projectName, agentName, envName string) error {
	annotations := map[string]string{
		secretRotatedAtAnnotation: s.now().UTC().Format(secretRotatedAtFormat),
	}
	envVars, err := s.envVarsForEnvironment(ctx, ouID, projectName, agentName, envName, annotations)
	if err != nil {
		return err
	}
	if len(envVars) == 0 {
		return nil
	}
	// Re-merging identical env vars is a no-op content-wise, but the call also
	// stamps restartedAt on the ReleaseBinding — the pod rollout that makes the
	// workload actually pick up the refreshed Secret.
	if err := withReleaseBindingRetry(func() error {
		return s.ocClient.UpdateReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, envVars)
	}); err != nil {
		return fmt.Errorf("roll out pod after agent identity secret rotation: %w", err)
	}
	s.logger.Info("Refreshed agent identity credentials in workload after rotation", "agentName", agentName, "envName", envName)
	return nil
}

func (s *agentIdentityInjectionService) RemoveForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string, includeWorkloadLevel bool) error {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return nil
		}
		return fmt.Errorf("read agent thunder binding for identity removal: %w", err)
	}
	if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
		return nil
	}

	keys := make([]string, 0, len(AgentIdentityEnvVarKeys()))
	for k := range AgentIdentityEnvVarKeys() {
		keys = append(keys, k)
	}

	// Remove from the per-environment ReleaseBinding overrides (idempotent —
	// nil when not deployed) and roll the pod.
	if err := withReleaseBindingRetry(func() error {
		return s.ocClient.RemoveReleaseBindingEnvVars(ctx, ouID, projectName, agentName, envName, keys)
	}); err != nil {
		return fmt.Errorf("remove agent identity env vars from release binding: %w", err)
	}
	// The deploy flow writes the lowest environment's vars at Workload level,
	// shared across environments — only strip those when we're actually
	// revoking that environment's credential.
	if includeWorkloadLevel {
		if err := withReleaseBindingRetry(func() error {
			return s.ocClient.RemoveWorkloadEnvVars(ctx, ouID, agentName, keys)
		}); err != nil {
			return fmt.Errorf("remove agent identity env vars from workload: %w", err)
		}
	}

	if err := s.deleteSecretReference(ctx, ouID, agentName, envName); err != nil {
		return err
	}
	s.logger.Info("Removed agent identity env vars from workload", "agentName", agentName, "envName", envName, "includeWorkloadLevel", includeWorkloadLevel)
	return nil
}

func (s *agentIdentityInjectionService) CleanupForEnvironment(ctx context.Context, ouID, agentName, envName string) error {
	return s.deleteSecretReference(ctx, ouID, agentName, envName)
}

func (s *agentIdentityInjectionService) deleteSecretReference(ctx context.Context, ouID, agentName, envName string) error {
	refName := AgentIdentitySecretRefName(agentName, envName)
	if err := s.ocClient.DeleteSecretReference(ctx, ouID, refName); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("delete agent identity SecretReference %q: %w", refName, err)
	}
	return nil
}
