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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxProvisionAttempts is the retry budget for one binding: the inline
// fast-path attempt plus reconciler retries, five attempts total before a
// binding is marked FAILED.
const maxProvisionAttempts = 5

// provisionBackoffSchedule maps "attempts made so far" to the delay before the
// next attempt — a flat 3 minutes for every retry (no escalation to longer
// delays; a real outage is retried just as promptly on attempt 4 as attempt
// 1, since env-Thunder recovering doesn't get less likely over time). Four
// retries at 3 minutes apart means the 5th and final attempt starts at most
// 12 minutes after the first, so the whole retry budget for one binding
// resolves — success or FAILED — within the 15-minute SLA. Index 0 is unused
// (there is no "0 attempts made" retry); the last entry is reused for any
// attempt count beyond it, though maxProvisionAttempts stops that from
// actually happening.
var provisionBackoffSchedule = []time.Duration{
	0, // unused
	3 * time.Minute,
	3 * time.Minute,
	3 * time.Minute,
	3 * time.Minute,
}

// AgentThunderProvisioningService orchestrates AgentID provisioning: creating,
// regenerating, revoking, and deleting Thunder identities per agent per
// environment. Binding architecture (Section 7 — pushing a credential into the
// gateway) is a later phase and is deliberately not implemented here.
type AgentThunderProvisioningService interface {
	// ProvisionForAgent writes a PENDING binding for every environment in
	// envNames (write-ahead — Section 6.2), then attempts all of them
	// best-effort in the background. Never blocks the caller. requestedBy is
	// the calling user's own subject, captured for audit only (see
	// models.AgentThunderClient.RequestedBy) — it is never sent to Thunder.
	ProvisionForAgent(ctx context.Context, orgName, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string)

	// AttemptProvision performs one provisioning attempt for a single binding.
	// Exported so the retry reconciler can call it directly for PENDING rows
	// found on its sweep.
	AttemptProvision(ctx context.Context, binding models.AgentThunderClient)

	// ProvisionForEnvironmentIfMissing ensures a binding exists for one agent in
	// one environment — used both by the external-agent identity-provision
	// endpoint and by PromoteAgent's internal-agent hook, so an environment that
	// didn't exist yet when the agent was created (or wasn't part of its
	// pipeline) still gets an AgentID once it's actually needed there. If a
	// binding already exists (any status — pending, completed, or failed), it is
	// left untouched and alreadyExisted is true; the reconciler already owns
	// retrying anything not yet completed. If none exists, behaves exactly like
	// ProvisionForAgent for this one environment: write-ahead PENDING, then a
	// best-effort background attempt.
	ProvisionForEnvironmentIfMissing(ctx context.Context, orgName, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string) (alreadyExisted bool, err error)

	// GetCredentials returns the current client ID and secret for one binding
	// WITHOUT destroying the stored copy — repeatable, unlike GetIdentityViews'
	// one-time External claim. For Internal agents, which have no other way to
	// retrieve their credential today (Gateway Binding — automatically injecting
	// it into the workload — is a later phase). Returns
	// utils.ErrAgentIdentityNotProvisioned if the binding doesn't exist or hasn't
	// completed yet, utils.ErrAgentCredentialNotAvailable if it has completed but
	// there is currently no stored secret (e.g. right after a revoke).
	GetCredentials(ctx context.Context, orgName, projectName, agentName, envName string) (agentID, clientID, clientSecret string, err error)

	// RegenerateSecret rotates the secret for one binding and returns the
	// binding's ownership type, client ID, and the new secret. The caller (the
	// HTTP layer) decides whether to expose the secret in the response based on
	// ownership — this service always returns the true new secret.
	RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (ownership models.AgentProvisioningType, clientID string, newSecret string, err error)

	// RevokeSecret rotates the secret in Thunder (invalidating the old one) and
	// clears the stored copy, leaving no currently-valid credential until an
	// explicit regenerate. It does not return the new secret value to anyone —
	// only the (unchanged) client ID, so callers can build a response body.
	RevokeSecret(ctx context.Context, orgName, projectName, agentName, envName string) (clientID string, err error)

	// DeleteAllBindings deletes every Thunder identity, stored secret, and
	// binding row for the agent, across all environments. Best-effort: logs
	// failures and never blocks the caller.
	DeleteAllBindings(ctx context.Context, orgName, projectName, agentName string)

	// GetIdentityViews returns the current binding for every environment this
	// agent has been provisioned in, applying secret exposure rules: an
	// External agent's secret is included at most once — the first time it is
	// retrieved after becoming available — and is then permanently destroyed
	// so it can never be served again. An Internal agent's secret is never
	// included. Callers needing project-level visibility filtering (Section 2.1
	// of the architecture doc) apply it on top of this org-wide result.
	GetIdentityViews(ctx context.Context, orgName, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error)
}

type agentThunderProvisioningService struct {
	repo        repositories.AgentThunderClientRepository
	envResolver thundersvc.EnvThunderResolver
	secretStore thundersvc.AgentSecretStore
	logger      *slog.Logger
}

// NewAgentThunderProvisioningService creates a new AgentThunderProvisioningService.
func NewAgentThunderProvisioningService(
	repo repositories.AgentThunderClientRepository,
	envResolver thundersvc.EnvThunderResolver,
	secretStore thundersvc.AgentSecretStore,
	logger *slog.Logger,
) AgentThunderProvisioningService {
	return &agentThunderProvisioningService{
		repo:        repo,
		envResolver: envResolver,
		secretStore: secretStore,
		logger:      logger,
	}
}

func (s *agentThunderProvisioningService) ProvisionForAgent(
	ctx context.Context, orgName, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string,
) {
	bindings := make([]models.AgentThunderClient, 0, len(envNames))
	for _, env := range envNames {
		b := models.AgentThunderClient{
			OrgName:          orgName,
			ProjectName:      projectName,
			AgentName:        agentName,
			EnvironmentName:  env,
			ProvisioningType: ownership,
			Status:           models.AgentThunderStatusPending,
			RequestedBy:      requestedBy,
		}
		if err := s.repo.Upsert(&b); err != nil {
			s.logger.Error("Failed to write-ahead agent thunder binding", "agentName", agentName, "env", env, "error", err)
			continue
		}
		bindings = append(bindings, b)
	}

	// Detach from the request context so the background attempt survives the
	// HTTP handler returning, mirroring the existing deleteAgentLLMConfigurations
	// pattern in agent_manager.go.
	go s.attemptAll(context.WithoutCancel(ctx), bindings)
}

func (s *agentThunderProvisioningService) attemptAll(ctx context.Context, bindings []models.AgentThunderClient) {
	for _, b := range bindings {
		s.AttemptProvision(ctx, b)
	}
}

func (s *agentThunderProvisioningService) ProvisionForEnvironmentIfMissing(
	ctx context.Context, orgName, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string,
) (bool, error) {
	_, err := s.repo.Get(orgName, projectName, agentName, envName)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
		return false, fmt.Errorf("check existing agent thunder binding: %w", err)
	}

	s.ProvisionForAgent(ctx, orgName, projectName, agentName, ownership, []string{envName}, requestedBy)
	return false, nil
}

func (s *agentThunderProvisioningService) AttemptProvision(ctx context.Context, binding models.AgentThunderClient) {
	// Atomically claim the binding before doing anything else: the inline
	// fast-path goroutine (ProvisionForAgent) and the reconciler's sweep can
	// both land on the same freshly-written binding within the same ~60s
	// window, since a write-ahead row has next_retry_at=nil, which FindDue
	// matches immediately. Without this claim, both could concurrently call
	// Thunder's CreateAgentIdentity/RegenerateAgentSecret and UpdateAfterAttempt
	// on the same row. claimed=false means someone else already holds this
	// binding (or it raced past being pending/stale) — skip silently rather
	// than duplicate the work.
	claimed, claimErr := s.repo.ClaimForAttempt(binding.ID)
	if claimErr != nil {
		s.logger.Error("Failed to claim agent thunder binding for attempt", "bindingID", binding.ID, "error", claimErr)
		return
	}
	if !claimed {
		s.logger.Debug("Agent thunder binding already claimed by another attempt, skipping", "bindingID", binding.ID)
		return
	}

	thunderClient, err := s.envResolver.Resolve(ctx, binding.OrgName, binding.EnvironmentName)
	if err != nil {
		s.recordFailure(binding, "", "", err)
		return
	}

	thunderAgentID := binding.ThunderAgentID
	clientID := binding.ThunderClientID
	var clientSecret string

	if thunderAgentID == "" {
		ouID, err := thunderClient.GetDefaultOUID(ctx)
		if err != nil {
			s.recordFailure(binding, "", "", fmt.Errorf("get default OU: %w", err))
			return
		}

		appName := thundersvc.AgentThunderAppName(binding.OrgName, binding.EnvironmentName, binding.ProjectName, binding.AgentName)
		var created bool
		thunderAgentID, clientID, clientSecret, created, err = thunderClient.CreateAgentIdentity(ctx, ouID, appName, "")
		if err != nil {
			s.recordFailure(binding, "", "", fmt.Errorf("create agent identity: %w", err))
			return
		}

		// Partial-failure recovery (Section 6.8): if Thunder already had this
		// agent (created=false, found via the 409 fallback), it never returns a
		// secret. That only happens if a prior attempt got as far as creating
		// the identity in Thunder but failed before we could store a secret —
		// there is no way to retrieve the original one, so generate a fresh,
		// storable secret now instead of leaving the binding stuck forever.
		if !created && clientSecret == "" {
			clientSecret, err = thunderClient.RegenerateAgentSecret(ctx, thunderAgentID)
			if err != nil {
				// thunderAgentID/clientID are already resolved at this point —
				// pass them through so a failure here doesn't lose them (see
				// the identical reasoning on the secretStore.Store failure below).
				s.recordFailure(binding, thunderAgentID, clientID, fmt.Errorf("recover secret for existing agent identity: %w", err))
				return
			}
		}
	}

	secretRefPath := binding.SecretRefPath
	if clientSecret != "" {
		secretRefPath, err = s.secretStore.Store(ctx, binding.OrgName, binding.ProjectName, binding.EnvironmentName, binding.AgentName, clientID, clientSecret)
		if err != nil {
			// The Thunder identity was already created successfully above —
			// pass thunderAgentID/clientID through so recordFailure persists
			// them despite this failure. Without this, the next retry would
			// see ThunderAgentID=="" and call CreateAgentIdentity again for a
			// name that already exists, hitting the 409 fallback and forcing
			// an unnecessary secret rotation.
			s.recordFailure(binding, thunderAgentID, clientID, fmt.Errorf("store agent secret: %w", err))
			return
		}
	}

	if err := s.repo.UpdateAfterAttempt(binding.ID, repositories.AgentThunderAttemptUpdate{
		Status:          models.AgentThunderStatusCompleted,
		ThunderAgentID:  thunderAgentID,
		ThunderClientID: clientID,
		SecretRefPath:   secretRefPath,
	}); err != nil {
		s.logger.Error("Failed to record successful agent thunder provisioning", "bindingID", binding.ID, "error", err)
	}
}

// recordFailure classifies cause as permanent or transient and updates the
// binding accordingly. ErrThunderNotProvisioned is the one permanent case this
// phase recognizes (Section 6.4) — everything else is retried up to
// maxProvisionAttempts times before being marked FAILED. resolvedAgentID/
// resolvedClientID carry through a Thunder identity that was already created
// before a LATER step in this same attempt failed (e.g. secret storage) —
// passing "" for either is correct when nothing was resolved yet.
// UpdateAfterAttempt only writes a non-empty field, so this never clobbers an
// already-stored value with a blank one.
func (s *agentThunderProvisioningService) recordFailure(binding models.AgentThunderClient, resolvedAgentID, resolvedClientID string, cause error) {
	attemptsSoFar := binding.AttemptCount + 1
	permanent := errors.Is(cause, thundersvc.ErrThunderNotProvisioned)
	exhausted := attemptsSoFar >= maxProvisionAttempts

	update := repositories.AgentThunderAttemptUpdate{
		LastError:       cause.Error(),
		ThunderAgentID:  resolvedAgentID,
		ThunderClientID: resolvedClientID,
	}
	if permanent || exhausted {
		update.Status = models.AgentThunderStatusFailed
	} else {
		update.Status = models.AgentThunderStatusPending
		next := time.Now().Add(provisionBackoffSchedule[attemptsSoFar])
		update.NextRetryAt = &next
	}

	if err := s.repo.UpdateAfterAttempt(binding.ID, update); err != nil {
		s.logger.Error("Failed to record agent thunder provisioning failure", "bindingID", binding.ID, "error", err)
	}
}

func (s *agentThunderProvisioningService) RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	binding, err := s.repo.Get(orgName, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", "", "", err
	}
	if binding.ThunderAgentID == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := s.envResolver.Resolve(ctx, orgName, envName)
	if err != nil {
		return "", "", "", err
	}

	newSecret, err := thunderClient.RegenerateAgentSecret(ctx, binding.ThunderAgentID)
	if err != nil {
		return "", "", "", err
	}

	secretPath, err := s.secretStore.Store(ctx, orgName, projectName, envName, agentName, binding.ThunderClientID, newSecret)
	if err != nil {
		return "", "", "", fmt.Errorf("store regenerated secret: %w", err)
	}
	if err := s.repo.UpdateSecretRef(binding.ID, secretPath); err != nil {
		return "", "", "", fmt.Errorf("record regenerated secret location: %w", err)
	}
	// A prior claim (if any) was for the OLD secret, which no longer exists —
	// this new one has never been shown to anyone via GetIdentityViews' one-time
	// claim, so it must not inherit a stale "already claimed" flag from before.
	if err := s.repo.ClearClaim(binding.ID); err != nil {
		s.logger.Warn("Failed to clear claim state after regenerate", "bindingID", binding.ID, "error", err)
	}

	return binding.ProvisioningType, binding.ThunderClientID, newSecret, nil
}

func (s *agentThunderProvisioningService) GetCredentials(ctx context.Context, orgName, projectName, agentName, envName string) (string, string, string, error) {
	binding, err := s.repo.Get(orgName, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", "", "", err
	}
	if binding.ThunderAgentID == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}
	if binding.SecretRefPath == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentCredentialNotAvailable, agentName, envName)
	}

	clientID, clientSecret, err := s.secretStore.Get(ctx, binding.SecretRefPath)
	if err != nil {
		if errors.Is(err, thundersvc.ErrAgentSecretNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentCredentialNotAvailable, agentName, envName)
		}
		return "", "", "", fmt.Errorf("read stored agent secret: %w", err)
	}
	return binding.ThunderAgentID, clientID, clientSecret, nil
}

func (s *agentThunderProvisioningService) RevokeSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, error) {
	binding, err := s.repo.Get(orgName, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", err
	}
	if binding.ThunderAgentID == "" {
		return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := s.envResolver.Resolve(ctx, orgName, envName)
	if err != nil {
		return "", err
	}

	// Rotating without storing the result is deliberate: revoke is a kill
	// switch, not "give me a fresh usable one" — that is what regenerate is for.
	if _, err := thunderClient.RegenerateAgentSecret(ctx, binding.ThunderAgentID); err != nil {
		return "", fmt.Errorf("revoke (rotate) secret: %w", err)
	}

	if binding.SecretRefPath != "" {
		if err := s.secretStore.Delete(ctx, binding.SecretRefPath); err != nil {
			// Non-fatal: the stored copy is now stale (points at an invalidated
			// secret) either way, and cleanup can be retried by revoking again.
			s.logger.Warn("Failed to delete stored secret during revoke", "bindingID", binding.ID, "error", err)
		}
	}

	if err := s.repo.UpdateSecretRef(binding.ID, ""); err != nil {
		return "", err
	}
	return binding.ThunderClientID, nil
}

func (s *agentThunderProvisioningService) GetIdentityViews(ctx context.Context, orgName, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	bindings, err := s.repo.FindByAgent(orgName, projectName, agentName)
	if err != nil {
		return nil, err
	}

	views := make([]models.AgentIdentityEnvironmentView, 0, len(bindings))
	for _, b := range bindings {
		view := models.AgentIdentityEnvironmentView{
			EnvironmentName:  b.EnvironmentName,
			ProvisioningType: b.ProvisioningType,
			Status:           b.Status,
			AgentID:          b.ThunderAgentID,
			ClientID:         b.ThunderClientID,
			LastError:        b.LastError,
			RequestedBy:      b.RequestedBy,
		}

		canClaim := b.ProvisioningType == models.AgentProvisioningTypeExternal &&
			b.SecretRefPath != "" &&
			b.ClaimedAt == nil
		if canClaim {
			// MarkClaimed is a compare-and-swap (claimed_at IS NULL) and is the
			// sole gate for "shown exactly once": it must run BEFORE reading the
			// secret, not after. Two concurrent GetIdentityViews calls for the
			// same unclaimed binding (a double-click, a client retry after a
			// lost response, two backend replicas) both pass the canClaim check
			// above on their own in-memory snapshot — only the claim-first CAS
			// stops both from then reading and returning the same secret.
			claimed, claimErr := s.repo.MarkClaimed(b.ID, time.Now())
			if claimErr != nil {
				s.logger.Warn("Failed to mark external agent secret as claimed", "bindingID", b.ID, "error", claimErr)
			} else if claimed {
				_, secret, getErr := s.secretStore.Get(ctx, b.SecretRefPath)
				if getErr != nil {
					s.logger.Warn("Failed to read external agent secret after claim; rolling back claim so a retry can still see it", "bindingID", b.ID, "error", getErr)
					if clearErr := s.repo.ClearClaim(b.ID); clearErr != nil {
						s.logger.Warn("Failed to roll back claim after secret read failure", "bindingID", b.ID, "error", clearErr)
					}
				} else {
					view.ClientSecret = secret

					if delErr := s.secretStore.Delete(ctx, b.SecretRefPath); delErr != nil {
						s.logger.Warn("Failed to destroy claimed external agent secret", "bindingID", b.ID, "error", delErr)
					}
					if err := s.repo.UpdateSecretRef(b.ID, ""); err != nil {
						s.logger.Warn("Failed to clear claimed external agent secret reference", "bindingID", b.ID, "error", err)
					}
				}
			}
			// claimed == false means a concurrent caller already won the claim
			// between our read of b.ClaimedAt and now — correctly serve no
			// secret here.
		}

		views = append(views, view)
	}

	return views, nil
}

func (s *agentThunderProvisioningService) DeleteAllBindings(ctx context.Context, orgName, projectName, agentName string) {
	bindings, err := s.repo.FindByAgent(orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent thunder bindings for deletion", "agentName", agentName, "error", err)
		return
	}

	for _, b := range bindings {
		if b.ThunderAgentID == "" {
			continue // never made it to Thunder — nothing to delete there
		}
		thunderClient, err := s.envResolver.Resolve(ctx, orgName, b.EnvironmentName)
		if err != nil {
			s.logger.Warn("Env-thunder resolver error during agent binding cleanup", "agentName", agentName, "env", b.EnvironmentName, "error", err)
			continue
		}
		if _, err := thunderClient.DeleteAgentIdentity(ctx, b.ThunderAgentID); err != nil {
			s.logger.Warn("Failed to delete Thunder agent identity", "agentName", agentName, "env", b.EnvironmentName, "error", err)
		}
		if b.SecretRefPath != "" {
			if err := s.secretStore.Delete(ctx, b.SecretRefPath); err != nil {
				s.logger.Warn("Failed to delete stored agent secret", "agentName", agentName, "env", b.EnvironmentName, "error", err)
			}
		}
	}

	if err := s.repo.DeleteByAgent(orgName, projectName, agentName); err != nil {
		s.logger.Error("Failed to delete agent thunder client rows", "agentName", agentName, "error", err)
	}
}
