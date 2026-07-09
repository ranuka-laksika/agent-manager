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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxProvisionAttempts is the retry budget for one binding: the inline
// fast-path attempt plus reconciler retries, five attempts total before a
// binding is marked FAILED.
const maxProvisionAttempts = 5

// writeAheadUpsertAttempts/writeAheadUpsertRetryDelay cover a momentary DB
// blip on the write-ahead insert itself — there is no row for the reconciler
// to find and retry later if this insert never lands.
const (
	writeAheadUpsertAttempts   = 3
	writeAheadUpsertRetryDelay = 100 * time.Millisecond
)

// provisionRetryDelay is the flat delay before every retry (no escalation to
// longer delays; a real outage is retried just as promptly on attempt 4 as
// attempt 1, since env-Thunder recovering doesn't get less likely over
// time). maxProvisionAttempts-1 retries at this interval means the final
// attempt starts at most (maxProvisionAttempts-1)*provisionRetryDelay after
// the first, so the whole retry budget for one binding resolves — success or
// FAILED — within the 15-minute SLA.
const provisionRetryDelay = 3 * time.Minute

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
	ProvisionForAgent(ctx context.Context, ouID, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string)

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
	ProvisionForEnvironmentIfMissing(ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string) (alreadyExisted bool, err error)

	// GetCredentials returns the current client ID and secret for one binding
	// WITHOUT destroying the stored copy — repeatable, unlike ClaimSecret's
	// one-time External claim. For Internal agents, which have no other way to
	// retrieve their credential today (Gateway Binding — automatically injecting
	// it into the workload — is a later phase). Returns
	// utils.ErrAgentIdentityNotProvisioned if the binding doesn't exist or hasn't
	// completed yet, utils.ErrAgentCredentialNotAvailable if it has completed but
	// there is currently no stored secret (e.g. right after a revoke).
	GetCredentials(ctx context.Context, ouID, projectName, agentName, envName string) (agentID, clientID, clientSecret string, err error)

	// RegenerateSecret rotates the secret for one binding and returns the
	// binding's ownership type, client ID, and the new secret. The caller (the
	// HTTP layer) decides whether to expose the secret in the response based on
	// ownership — this service always returns the true new secret.
	RegenerateSecret(ctx context.Context, ouID, projectName, agentName, envName string) (ownership models.AgentProvisioningType, clientID string, newSecret string, err error)

	// RevokeSecret rotates the secret in Thunder (invalidating the old one) and
	// clears the stored copy, leaving no currently-valid credential until an
	// explicit regenerate. It does not return the new secret value to anyone —
	// only the (unchanged) client ID, so callers can build a response body.
	RevokeSecret(ctx context.Context, ouID, projectName, agentName, envName string) (clientID string, err error)

	// DeleteAllBindings deletes every Thunder identity, stored secret, and
	// binding row for the agent, across all environments. Best-effort: logs
	// failures and never blocks the caller.
	DeleteAllBindings(ctx context.Context, ouID, projectName, agentName string)

	// GetIdentityViews returns the current binding for every environment this
	// agent has been provisioned in. A safe, side-effect-free read: it never
	// returns or destroys a secret, even for an unclaimed External binding —
	// each view's HasUnclaimedSecret flag reports whether one is available,
	// and ClaimSecret is the only way to actually retrieve and consume it.
	// Callers needing project-level visibility filtering (Section 2.1 of the
	// architecture doc) apply it on top of this org-wide result.
	GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error)

	// ClaimSecret performs the one-time claim of an External agent's secret
	// for one environment: the first successful call destroys the stored copy
	// and returns it; every call after that fails with
	// utils.ErrAgentCredentialNotAvailable. This is the only endpoint-facing
	// operation that actually exposes an External agent's secret — GetIdentityViews
	// never does. Internal agents are rejected with utils.ErrInvalidInput;
	// they have no claim state, and use GetCredentials instead.
	ClaimSecret(ctx context.Context, ouID, projectName, agentName, envName string) (agentID, clientID, clientSecret string, err error)
}

type agentThunderProvisioningService struct {
	repo        repositories.AgentThunderClientRepository
	envResolver thundersvc.EnvThunderResolver
	secretStore thundersvc.AgentSecretStore
	// workloadInjector pushes a freshly-provisioned internal agent's credential
	// into its live workload (Gateway Binding). Optional (nil skips injection):
	// the deploy flow independently injects at deploy time, so this hook only
	// covers agents that were deployed BEFORE provisioning completed.
	workloadInjector AgentIdentityInjectionService
	logger           *slog.Logger
	bindingLocks     keyedMutex
}

// NewOpenBaoAgentThunderProvisioning returns the deployment factory that builds
// the OpenBao-backed provisioning service once the DB is available (see
// app.Options.AgentThunderProvisioning). Used by the open-source deployment,
// which provisions AgentIDs against per-environment Thunder via OpenBao; panics
// on a missing/invalid OPENBAO_* config so startup fails fast.
//
// workloadInjector is deliberately nil here: this factory runs in app.Run
// before the OpenChoreo client exists (that's built inside wiring.InitializeAppParams,
// which this factory's result feeds INTO — see app/app.go), so there is nothing
// to construct a real AgentIdentityInjectionService with at this point. This
// narrows exactly two secondary, best-effort code paths inside this specific
// service instance: the post-provisioning-completion inject hook in
// AttemptProvision (for an agent deployed before its identity finished
// provisioning) and the SecretReference cleanup in DeleteAllBindings. Every
// primary credential-injection path — deploy, promote, regenerate, revoke,
// config updates — is unaffected: those run through AgentManagerService's own
// AgentIdentityInjectionService, wired independently via wire
// (ProvideAgentIdentityInjectionService in serviceProviderSet), not through
// this provisioning service's internal field.
func NewOpenBaoAgentThunderProvisioning(cfg config.Config) func(db *gorm.DB) AgentThunderProvisioningService {
	return func(db *gorm.DB) AgentThunderProvisioningService {
		envResolver, err := thundersvc.NewEnvThunderResolver(cfg.OpenBao.URL, cfg.OpenBao.Token, cfg.OpenBao.Path)
		if err != nil {
			panic(fmt.Errorf("create env thunder resolver: %w", err))
		}
		secretStore, err := thundersvc.NewAgentSecretStore(cfg.OpenBao.URL, cfg.OpenBao.Token, cfg.OpenBao.Path)
		if err != nil {
			panic(fmt.Errorf("create agent secret store: %w", err))
		}
		return NewAgentThunderProvisioningService(
			repositories.NewAgentThunderClientRepo(db),
			envResolver,
			secretStore,
			nil, // see doc comment above
			slog.Default(),
		)
	}
}

// NewAgentThunderProvisioningService creates a new AgentThunderProvisioningService.
// workloadInjector may be nil (no workload injection on provisioning completion).
func NewAgentThunderProvisioningService(
	repo repositories.AgentThunderClientRepository,
	envResolver thundersvc.EnvThunderResolver,
	secretStore thundersvc.AgentSecretStore,
	workloadInjector AgentIdentityInjectionService,
	logger *slog.Logger,
) AgentThunderProvisioningService {
	return &agentThunderProvisioningService{
		repo:             repo,
		envResolver:      envResolver,
		secretStore:      secretStore,
		workloadInjector: workloadInjector,
		logger:           logger,
	}
}

// keyedMutex serializes RegenerateSecret/RevokeSecret per binding within this
// process, so two concurrent rotations for the same binding can't interleave
// their Thunder call and OpenBao write and leave the stored secret mismatched
// with what Thunder actually considers active. In-process only — it does not
// protect across multiple replicas of this service. Entries are refcounted
// and removed once the last waiter releases, so the map doesn't grow
// unbounded with every binding ever rotated over a long-lived process.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*refCountedMutex
}

type refCountedMutex struct {
	mu   sync.Mutex
	refs int
}

func (m *keyedMutex) Lock(key string) func() {
	m.mu.Lock()
	if m.locks == nil {
		m.locks = make(map[string]*refCountedMutex)
	}
	l, ok := m.locks[key]
	if !ok {
		l = &refCountedMutex{}
		m.locks[key] = l
	}
	l.refs++
	m.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		m.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

func bindingLockKey(ouID, projectName, agentName, envName string) string {
	return ouID + "|" + projectName + "|" + agentName + "|" + envName
}

func (s *agentThunderProvisioningService) ProvisionForAgent(
	ctx context.Context, ouID, projectName, agentName string, ownership models.AgentProvisioningType, envNames []string, requestedBy string,
) {
	bindings := make([]models.AgentThunderClient, 0, len(envNames))
	for _, env := range envNames {
		b := models.AgentThunderClient{
			OUID:             ouID,
			ProjectName:      projectName,
			AgentName:        agentName,
			EnvironmentName:  env,
			ProvisioningType: ownership,
			Status:           models.AgentThunderStatusPending,
			RequestedBy:      requestedBy,
		}
		// Retry a few times before giving up: this row is the ONLY thing the
		// reconciler can later find and heal, so a momentary DB blip here must
		// not permanently drop the environment with no path to provisioning.
		var err error
		for attempt := 0; attempt < writeAheadUpsertAttempts; attempt++ {
			if err = s.repo.Upsert(ctx, &b); err == nil {
				break
			}
			time.Sleep(writeAheadUpsertRetryDelay)
		}
		if err != nil {
			s.logger.Error("Failed to write-ahead agent thunder binding after retries", "agentName", agentName, "env", env, "error", err)
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
	ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string,
) (bool, error) {
	_, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
		return false, fmt.Errorf("check existing agent thunder binding: %w", err)
	}

	s.ProvisionForAgent(ctx, ouID, projectName, agentName, ownership, []string{envName}, requestedBy)
	return false, nil
}

func (s *agentThunderProvisioningService) AttemptProvision(ctx context.Context, binding models.AgentThunderClient) {
	// Held for the entire attempt so this can't interleave its Thunder
	// RegenerateAgentSecret/OpenBao Store calls with a concurrent
	// RegenerateSecret/RevokeSecret on the same binding.
	defer s.bindingLocks.Lock(bindingLockKey(binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName))()

	// A panic here (e.g. AgentThunderAppName's on an invalid slug) would
	// otherwise crash the whole process — this runs on a detached goroutine
	// or the reconciler's per-binding goroutine, never the request path.
	// Recovering isolates it to just this one binding.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Recovered from panic during agent thunder provisioning attempt",
				"bindingID", binding.ID, "agentName", binding.AgentName, "envName", binding.EnvironmentName, "panic", r)
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("panic during provisioning attempt: %v", r))
		}
	}()

	// Atomically claim the binding before doing anything else: the inline
	// fast-path goroutine (ProvisionForAgent) and the reconciler's sweep can
	// both land on the same freshly-written binding within the same ~60s
	// window, since a write-ahead row has next_retry_at=nil, which FindDue
	// matches immediately. Without this claim, both could concurrently call
	// Thunder's CreateAgentIdentity/RegenerateAgentSecret and UpdateAfterAttempt
	// on the same row. claimed=false means someone else already holds this
	// binding (or it raced past being pending/stale) — skip silently rather
	// than duplicate the work.
	claimed, claimErr := s.repo.ClaimForAttempt(ctx, binding.ID)
	if claimErr != nil {
		s.logger.Error("Failed to claim agent thunder binding for attempt", "bindingID", binding.ID, "error", claimErr)
		return
	}
	if !claimed {
		s.logger.Debug("Agent thunder binding already claimed by another attempt, skipping", "bindingID", binding.ID)
		return
	}

	thunderClient, err := s.envResolver.Resolve(ctx, binding.OUID, binding.EnvironmentName)
	if err != nil {
		s.recordFailure(ctx, binding, "", "", err)
		return
	}

	thunderAgentID := binding.ThunderAgentID
	clientID := binding.ThunderClientID
	var clientSecret string

	if thunderAgentID == "" {
		ouID, err := thunderClient.GetDefaultOUID(ctx)
		if err != nil {
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("get default OU: %w", err))
			return
		}

		appName := thundersvc.AgentThunderAppName(binding.OUID, binding.EnvironmentName, binding.ProjectName, binding.AgentName)
		var created bool
		thunderAgentID, clientID, clientSecret, created, err = thunderClient.CreateAgentIdentity(ctx, ouID, appName, "")
		if err != nil {
			s.recordFailure(ctx, binding, "", "", fmt.Errorf("create agent identity: %w", err))
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
				s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("recover secret for existing agent identity: %w", err))
				return
			}
		}
	} else if binding.SecretRefPath == "" {
		// Retry of an attempt that created the identity but failed before storing a secret.
		clientSecret, err = thunderClient.RegenerateAgentSecret(ctx, thunderAgentID)
		if err != nil {
			s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("recover secret for existing agent identity: %w", err))
			return
		}
	}

	secretRefPath := binding.SecretRefPath
	if clientSecret != "" {
		secretRefPath, err = s.secretStore.Store(ctx, binding.OUID, binding.ProjectName, binding.EnvironmentName, binding.AgentName, clientID, clientSecret)
		if err != nil {
			// The Thunder identity was already created successfully above —
			// pass thunderAgentID/clientID through so recordFailure persists
			// them despite this failure. Without this, the next retry would
			// see ThunderAgentID=="" and call CreateAgentIdentity again for a
			// name that already exists, hitting the 409 fallback and forcing
			// an unnecessary secret rotation.
			s.recordFailure(ctx, binding, thunderAgentID, clientID, fmt.Errorf("store agent secret: %w", err))
			return
		}
	}

	if err := s.repo.UpdateAfterAttempt(ctx, binding.ID, repositories.AgentThunderAttemptUpdate{
		Status:          models.AgentThunderStatusCompleted,
		ThunderAgentID:  &thunderAgentID,
		ThunderClientID: &clientID,
		SecretRefPath:   &secretRefPath,
	}); err != nil {
		s.logger.Error("Failed to record successful agent thunder provisioning", "bindingID", binding.ID, "error", err)
		return
	}

	// Gateway Binding: push the credential into the live workload for internal
	// agents that were already deployed before this attempt completed. Purely
	// best-effort — the binding is COMPLETED either way, and the deploy flow
	// re-derives these env vars on every (re)deploy regardless.
	if s.workloadInjector != nil && binding.ProvisioningType == models.AgentProvisioningTypeInternal {
		if err := s.workloadInjector.InjectForEnvironment(ctx, binding.OUID, binding.ProjectName, binding.AgentName, binding.EnvironmentName); err != nil {
			s.logger.Warn("Failed to inject agent identity credentials into workload after provisioning",
				"agentName", binding.AgentName, "envName", binding.EnvironmentName, "error", err)
		}
	}
}

// recordFailure retries cause up to maxProvisionAttempts times before marking
// the binding FAILED. ErrThunderNotProvisioned is treated the same as any
// other failure, not as an immediate permanent one — the environment may just
// be mid-bootstrap (add-environment-thunder.sh still running), a window the
// existing retry budget already covers. resolvedAgentID/resolvedClientID
// carry through a Thunder identity that was already created before a LATER
// step in this same attempt failed (e.g. secret storage) — passing "" for
// either means nothing was resolved yet, so the update leaves the
// corresponding field nil (unchanged) rather than clobbering an
// already-stored value with a blank one.
func (s *agentThunderProvisioningService) recordFailure(ctx context.Context, binding models.AgentThunderClient, resolvedAgentID, resolvedClientID string, cause error) {
	attemptsSoFar := binding.AttemptCount + 1
	exhausted := attemptsSoFar >= maxProvisionAttempts

	update := repositories.AgentThunderAttemptUpdate{
		LastError: cause.Error(),
	}
	if resolvedAgentID != "" {
		update.ThunderAgentID = &resolvedAgentID
	}
	if resolvedClientID != "" {
		update.ThunderClientID = &resolvedClientID
	}
	if exhausted {
		update.Status = models.AgentThunderStatusFailed
	} else {
		update.Status = models.AgentThunderStatusPending
		next := time.Now().Add(provisionRetryDelay)
		update.NextRetryAt = &next
	}

	if err := s.repo.UpdateAfterAttempt(ctx, binding.ID, update); err != nil {
		s.logger.Error("Failed to record agent thunder provisioning failure", "bindingID", binding.ID, "error", err)
	}
}

func (s *agentThunderProvisioningService) RegenerateSecret(ctx context.Context, ouID, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	defer s.bindingLocks.Lock(bindingLockKey(ouID, projectName, agentName, envName))()

	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", "", "", err
	}
	if binding.ThunderAgentID == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := s.envResolver.Resolve(ctx, ouID, envName)
	if err != nil {
		return "", "", "", err
	}

	newSecret, err := thunderClient.RegenerateAgentSecret(ctx, binding.ThunderAgentID)
	if err != nil {
		return "", "", "", err
	}

	secretPath, err := s.secretStore.Store(ctx, ouID, projectName, envName, agentName, binding.ThunderClientID, newSecret)
	if err != nil {
		return "", "", "", fmt.Errorf("store regenerated secret: %w", err)
	}
	if err := s.repo.UpdateSecretRef(ctx, binding.ID, secretPath); err != nil {
		return "", "", "", fmt.Errorf("record regenerated secret location: %w", err)
	}
	// Regenerate's own response already hands the caller this secret directly
	// (see RegenerateAgentIdentitySecret), so it must not also show up as
	// unclaimed — mark it claimed now rather than leaving/reopening a claim
	// for a secret that's already been shown.
	if _, err := s.repo.MarkClaimed(ctx, binding.ID, time.Now()); err != nil {
		s.logger.Warn("Failed to mark claim state after regenerate", "bindingID", binding.ID, "error", err)
	}

	return binding.ProvisioningType, binding.ThunderClientID, newSecret, nil
}

func (s *agentThunderProvisioningService) GetCredentials(ctx context.Context, ouID, projectName, agentName, envName string) (string, string, string, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
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

func (s *agentThunderProvisioningService) RevokeSecret(ctx context.Context, ouID, projectName, agentName, envName string) (string, error) {
	defer s.bindingLocks.Lock(bindingLockKey(ouID, projectName, agentName, envName))()

	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", err
	}
	if binding.ThunderAgentID == "" {
		return "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}

	thunderClient, err := s.envResolver.Resolve(ctx, ouID, envName)
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

	if err := s.repo.UpdateSecretRef(ctx, binding.ID, ""); err != nil {
		return "", err
	}
	return binding.ThunderClientID, nil
}

func (s *agentThunderProvisioningService) GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	bindings, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}

	views := make([]models.AgentIdentityEnvironmentView, 0, len(bindings))
	for _, b := range bindings {
		views = append(views, models.AgentIdentityEnvironmentView{
			EnvironmentName:  b.EnvironmentName,
			ProvisioningType: b.ProvisioningType,
			Status:           b.Status,
			AgentID:          b.ThunderAgentID,
			ClientID:         b.ThunderClientID,
			LastError:        b.LastError,
			RequestedBy:      b.RequestedBy,
			HasUnclaimedSecret: b.ProvisioningType == models.AgentProvisioningTypeExternal &&
				b.SecretRefPath != "" && b.ClaimedAt == nil,
		})
	}

	return views, nil
}

func (s *agentThunderProvisioningService) ClaimSecret(ctx context.Context, ouID, projectName, agentName, envName string) (string, string, string, error) {
	binding, err := s.repo.Get(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		if errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
		}
		return "", "", "", err
	}
	if binding.ProvisioningType != models.AgentProvisioningTypeExternal {
		return "", "", "", fmt.Errorf("%w: agent %q is an internal agent — internal agent credentials are retrieved via GetAgentCredentials, not claim", utils.ErrInvalidInput, agentName)
	}
	if binding.ThunderAgentID == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, envName)
	}
	if binding.SecretRefPath == "" {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentCredentialNotAvailable, agentName, envName)
	}

	// MarkClaimed is a compare-and-swap (claimed_at IS NULL) and is the sole
	// gate for "shown exactly once": it must run BEFORE reading the secret,
	// not after, so two concurrent claims for the same binding can't both
	// read and return the same secret.
	claimed, claimErr := s.repo.MarkClaimed(ctx, binding.ID, time.Now())
	if claimErr != nil {
		return "", "", "", fmt.Errorf("mark agent secret as claimed: %w", claimErr)
	}
	if !claimed {
		return "", "", "", fmt.Errorf("%w: %s in %s", utils.ErrAgentCredentialNotAvailable, agentName, envName)
	}

	_, secret, getErr := s.secretStore.Get(ctx, binding.SecretRefPath)
	if getErr != nil {
		// Roll back the claim so a retry can still see this secret — the read
		// failure means it was never actually shown to anyone.
		if clearErr := s.repo.ClearClaim(ctx, binding.ID); clearErr != nil {
			s.logger.Warn("Failed to roll back claim after secret read failure", "bindingID", binding.ID, "error", clearErr)
		}
		return "", "", "", fmt.Errorf("read claimed agent secret: %w", getErr)
	}

	if delErr := s.secretStore.Delete(ctx, binding.SecretRefPath); delErr != nil {
		s.logger.Warn("Failed to destroy claimed external agent secret", "bindingID", binding.ID, "error", delErr)
	}
	if err := s.repo.UpdateSecretRef(ctx, binding.ID, ""); err != nil {
		s.logger.Warn("Failed to clear claimed external agent secret reference", "bindingID", binding.ID, "error", err)
	}

	return binding.ThunderAgentID, binding.ThunderClientID, secret, nil
}

func (s *agentThunderProvisioningService) DeleteAllBindings(ctx context.Context, ouID, projectName, agentName string) {
	bindings, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent thunder bindings for deletion", "agentName", agentName, "error", err)
		return
	}

	// Delete the rows right after snapshotting them, by ID — before the slow
	// per-environment Thunder cleanup below, not after. Upsert's OnConflict
	// DoNothing means a same-named agent recreated while an old row is still
	// present silently gets no fresh row at all, not just a wiped one — doing
	// this late (or by agent name instead of ID) leaves that window open for
	// as long as the Thunder calls below take. A crash between here and the
	// Thunder cleanup can orphan a Thunder identity, an accepted, harmless
	// tradeoff already made elsewhere in this file.
	ids := make([]uuid.UUID, 0, len(bindings))
	for _, b := range bindings {
		ids = append(ids, b.ID)
	}
	if err := s.repo.DeleteByIDs(ctx, ids); err != nil {
		// Do NOT return here: by the time this method runs, the agent's own
		// primary record (the OpenChoreo Component) is already gone from the
		// caller's perspective — this is best-effort cleanup of everything
		// ELSE the agent left behind. A failed DB-row delete leaves a few
		// stale local rows, which is comparatively inert and can be swept up
		// later; but bailing out here would ALSO skip deleting the live
		// Thunder identities, SecretReference CRs, and OpenBao secrets below
		// — external, still-active resources whose leak is a materially
		// worse outcome (an orphaned Thunder identity can still mint valid
		// tokens indefinitely) than a few dangling database rows.
		s.logger.Error("Failed to delete agent thunder client rows; continuing to clean up external resources anyway", "agentName", agentName, "error", err)
	}

	for _, b := range bindings {
		// The AgentID SecretReference CR is independent of whether the identity
		// ever landed in Thunder — clean it up for every internal binding, even
		// ones that never completed.
		if s.workloadInjector != nil && b.ProvisioningType == models.AgentProvisioningTypeInternal {
			if err := s.workloadInjector.CleanupForEnvironment(ctx, ouID, agentName, b.EnvironmentName); err != nil {
				s.logger.Warn("Failed to delete agent identity SecretReference during agent deletion", "agentName", agentName, "env", b.EnvironmentName, "error", err)
			}
		}
		if b.ThunderAgentID == "" {
			continue // never made it to Thunder — nothing to delete there
		}
		thunderClient, err := s.envResolver.Resolve(ctx, ouID, b.EnvironmentName)
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
}
