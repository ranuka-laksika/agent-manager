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
	"strings"
	"time"

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

// placeholderAgentIdentityScopes is the fixed scope list injected into every
// internal agent until the resource-permission phase lands (see the TODO on
// resolveAgentIdentityScopes below). "amp:mcp:invoke" is just today's one
// concrete example — MCP is the first resource type Thunder-side permissions
// are being built for, not the only one this mechanism is meant to cover.
// Requesting a scope the AgentID is not authorized for is safe: Thunder
// filters requested scopes down to what the agent's role assignments
// actually grant, so an unauthorized placeholder scope is simply absent from
// the issued token.
var placeholderAgentIdentityScopes = []string{"amp:mcp:invoke"}

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
	// time). Used by the post-provisioning hook so an agent deployed BEFORE
	// its AgentID finished provisioning still receives the credential.
	InjectForEnvironment(ctx context.Context, ouID, projectName, agentName, envName string) error

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
	repo            repositories.AgentThunderClientRepository
	ocClient        client.OpenChoreoClient
	refreshInterval string
	logger          *slog.Logger
	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

// NewAgentIdentityInjectionService creates a new AgentIdentityInjectionService.
// refreshInterval is the SecretReference refresh cadence (same value
// secretmanagersvc uses, e.g. "1h").
func NewAgentIdentityInjectionService(
	repo repositories.AgentThunderClientRepository,
	ocClient client.OpenChoreoClient,
	refreshInterval string,
	logger *slog.Logger,
) AgentIdentityInjectionService {
	return &agentIdentityInjectionService{
		repo:            repo,
		ocClient:        ocClient,
		refreshInterval: refreshInterval,
		logger:          logger,
		now:             time.Now,
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

// resolveAgentIdentityScopes returns the FULL set of OAuth2 scopes the agent
// should request when minting a token — the union across every resource
// type the platform defines Thunder permissions for, not just one.
//
// TODO(agentid-resource-scopes): owned separately, comes after the
// resource-permission phase (starting with MCP, then LLM providers, APIs,
// and others using the identical mechanism). In Thunder's model, a
// permission is a scope string ("{resourceServerHandle}:{resource}:{action}")
// defined under a Resource Server; MCP proxies, LLM providers, and APIs each
// become their own Resource Server as their permission model lands. Scopes
// are bundled into Roles, and Roles are assigned to the agent's AgentID
// either directly or via Group membership — never scoped to a single
// resource type. The real implementation must:
//  1. Look up every Role assigned to this agent's Thunder identity (directly
//     assigned, and via any Group it belongs to) in this environment's
//     Thunder.
//  2. Collect every permission (scope string) each of those Roles bundles,
//     regardless of which Resource Server (MCP, LLM, API, ...) it belongs
//     to — the aggregation must NOT be filtered to one resource type.
//  3. Return the deduplicated union as a space-joined scope string.
//
// This method is intentionally a receiver method (not a free function) so
// that dependency — whatever Thunder client/cache is needed to do the
// role/group lookup above — has a natural home as a field on
// agentIdentityInjectionService, the same way ocClient/repo already do,
// without needing to change this method's signature or any call site.
//
// Until that lands, every internal agent requests the same placeholder
// regardless of what it's actually entitled to — see
// placeholderAgentIdentityScopes for why that's safe (Thunder filters
// unauthorized requested scopes rather than erroring).
func (s *agentIdentityInjectionService) resolveAgentIdentityScopes(_ context.Context, _, _, _, _ string) []string {
	return placeholderAgentIdentityScopes
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
func (s *agentIdentityInjectionService) buildEnvVars(ctx context.Context, binding *models.AgentThunderClient, secretRefName string) []client.EnvVar {
	scopes := s.resolveAgentIdentityScopes(ctx, binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName)
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
		{Key: client.EnvVarAgentIdentityTokenEndpoint, Value: thundersvc.ThunderTokenURL(binding.OUID, binding.EnvironmentName)},
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
