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
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func newTestProvisioningService(
	repo *repomocks.AgentThunderClientRepositoryMock,
	resolver *clientmocks.EnvThunderResolverMock,
	store *clientmocks.AgentSecretStoreMock,
) AgentThunderProvisioningService {
	return NewAgentThunderProvisioningService(repo, resolver, store, slog.Default())
}

func fakeThunderClientMock() *clientmocks.ThunderClientMock {
	return &clientmocks.ThunderClientMock{
		GetDefaultOUIDFunc: func(_ context.Context) (string, error) { return "ou-1", nil },
	}
}

func TestAttemptProvision_Success_CreatesIdentityAndStoresSecret(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, ouID, name, owner string) (string, string, string, bool, error) {
		assert.Equal(t, "ou-1", ouID)
		assert.Empty(t, owner, "owner must be left empty so Thunder defaults it to the caller's own subject")
		return "thunder-agent-1", "client-abc", "secret-xyz", true, nil
	}

	var storedPath string
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, org, project, env, agent, clientID, clientSecret string) (string, error) {
			assert.Equal(t, "client-abc", clientID)
			assert.Equal(t, "secret-xyz", clientSecret)
			storedPath = "agent-thunder-clients/" + org + "/" + project + "/" + env + "/" + agent
			return storedPath, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, orgName, envName string) (thundersvc.ThunderClient, error) { return tc, nil },
	}

	var recorded repositories.AgentThunderAttemptUpdate
	var recordedID uuid.UUID
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, id uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recordedID = id
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	bindingID := uuid.New()
	binding := models.AgentThunderClient{
		ID: bindingID, OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, bindingID, recordedID)
	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	assert.Equal(t, "thunder-agent-1", recorded.ThunderAgentID)
	assert.Equal(t, "client-abc", recorded.ThunderClientID)
	assert.Equal(t, storedPath, recorded.SecretRefPath)
	assert.Empty(t, recorded.LastError)
}

func TestAttemptProvision_AlreadyHasThunderAgentID_SkipsCreate(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the binding already has a thunderAgentID")
		return "", "", "", false, nil
	}
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		assert.Equal(t, "already-created", thunderAgentID)
		return "recovered-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret string
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, _, _, _, _, _, clientSecret string) (string, error) {
			storedSecret = clientSecret
			return "some/path", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ThunderAgentID: "already-created", ThunderClientID: "already-client-id",
		// SecretRefPath deliberately empty: models a binding whose identity was
		// created on a prior attempt, but that attempt failed before a secret
		// was ever stored for it.
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	assert.Equal(t, "recovered-secret", storedSecret,
		"a binding with a Thunder identity but no stored secret must recover one before completing")
	assert.Equal(t, "some/path", recorded.SecretRefPath,
		"the recovered secret's storage location must be persisted, not left empty")
}

// TestAttemptProvision_AlreadyHasSecretRef_SkipsRecovery guards the inverse of
// the recovery case above: a binding that already has both a Thunder identity
// AND a previously stored secret must not regenerate or re-store on a
// subsequent attempt (e.g. a retry triggered by an unrelated transient
// failure elsewhere) — that would needlessly invalidate a secret that is
// already valid and possibly already in use.
func TestAttemptProvision_AlreadyHasSecretRef_SkipsRecovery(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the binding already has a thunderAgentID")
		return "", "", "", false, nil
	}
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		t.Fatal("must not regenerate a secret when one is already stored")
		return "", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(context.Context, string, string, string, string, string, string) (string, error) {
			t.Fatal("must not store a new secret when one is already stored")
			return "", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
		ThunderAgentID: "already-created", ThunderClientID: "already-client-id", SecretRefPath: "existing/path",
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
	assert.Equal(t, "existing/path", recorded.SecretRefPath, "an already-stored secret ref must be left untouched")
}

func TestAttemptProvision_ConflictFallback_RegeneratesSecretToRecover(t *testing.T) {
	// This models the partial-failure case from Section 6.8 of the architecture
	// doc: Thunder already has the agent (a prior attempt's DB write must have
	// failed after Thunder succeeded), so create falls back to a name lookup —
	// which never returns a secret. Recovery must regenerate one so the binding
	// ends up with a usable, storable secret instead of getting stuck forever.
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "thunder-agent-1", "client-abc", "", false, nil // created=false, no secret
	}
	regenerateCalled := false
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		regenerateCalled = true
		assert.Equal(t, "thunder-agent-1", thunderAgentID)
		return "recovered-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret string
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, _, _, _, _, _, clientSecret string) (string, error) {
			storedSecret = clientSecret
			return "some/path", nil
		},
	}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
	}
	svc.AttemptProvision(context.Background(), binding)

	assert.True(t, regenerateCalled)
	assert.Equal(t, "recovered-secret", storedSecret)
	assert.Equal(t, models.AgentThunderStatusCompleted, recorded.Status)
}

func TestAttemptProvision_TransientFailure_SchedulesRetryWithBackoff(t *testing.T) {
	boom := errors.New("connection refused")
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "", "", "", false, boom
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.AgentSecretStoreMock{}

	tests := []struct {
		name              string
		attemptCountSoFar int
		expectedDelay     time.Duration
	}{
		{"first failure -> 3 minutes", 0, 3 * time.Minute},
		{"second failure -> 3 minutes", 1, 3 * time.Minute},
		{"third failure -> 3 minutes", 2, 3 * time.Minute},
		{"fourth failure -> 3 minutes", 3, 3 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recorded repositories.AgentThunderAttemptUpdate
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
				UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
					recorded = fields
					return nil
				},
			}
			svc := newTestProvisioningService(repo, resolver, store)
			binding := models.AgentThunderClient{
				ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent",
				EnvironmentName: "staging", AttemptCount: tt.attemptCountSoFar,
			}

			before := time.Now()
			svc.AttemptProvision(context.Background(), binding)

			assert.Equal(t, models.AgentThunderStatusPending, recorded.Status,
				"a transient failure must stay PENDING for the reconciler to retry, not FAILED")
			assert.Contains(t, recorded.LastError, "connection refused")
			require.NotNil(t, recorded.NextRetryAt)
			assert.WithinDuration(t, before.Add(tt.expectedDelay), *recorded.NextRetryAt, 2*time.Second)
		})
	}
}

func TestAttemptProvision_FifthFailure_MarksFailedNoMoreRetries(t *testing.T) {
	boom := errors.New("thunder unreachable")
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		return "", "", "", false, boom
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.AgentSecretStoreMock{}

	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	// 4 attempts already made; this is the 5th (and final, per the max-5 budget).
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent",
		EnvironmentName: "staging", AttemptCount: 4,
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusFailed, recorded.Status)
	assert.Nil(t, recorded.NextRetryAt)
	assert.Contains(t, recorded.LastError, "thunder unreachable")
}

// TestProvisionBackoffSchedule_TotalRetryWindowWithinSLA guards the actual
// requirement behind the schedule: whatever the individual per-retry delay
// is, the cumulative delay before the 5th (final) attempt fires must stay
// within the 15-minute SLA for one binding's retry budget to resolve.
func TestProvisionBackoffSchedule_TotalRetryWindowWithinSLA(t *testing.T) {
	var cumulative time.Duration
	for attempt := 1; attempt < maxProvisionAttempts; attempt++ {
		cumulative += provisionBackoffSchedule[attempt]
	}
	assert.LessOrEqualf(t, cumulative, 15*time.Minute,
		"cumulative delay before the final attempt (%s) must not exceed the 15-minute SLA", cumulative)
}

func TestAttemptProvision_ThunderNotProvisioned_MarksFailedImmediately(t *testing.T) {
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) {
			return nil, thundersvc.ErrThunderNotProvisioned
		},
	}
	store := &clientmocks.AgentSecretStoreMock{}
	var recorded repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, fields repositories.AgentThunderAttemptUpdate) error {
			recorded = fields
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	// First attempt (AttemptCount 0) — even so, an unprovisioned environment is a
	// permanent condition, not something 5 retries will fix.
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "no-thunder-env",
	}

	svc.AttemptProvision(context.Background(), binding)

	assert.Equal(t, models.AgentThunderStatusFailed, recorded.Status)
	assert.Nil(t, recorded.NextRetryAt)
}

// TestAttemptProvision_ClaimFails_SkipsWithoutTouchingThunder guards the fix
// for the dual-concurrent-provisioning race: the inline fast-path goroutine
// and the reconciler's sweep can both land on the same freshly-written
// binding within the same ~60s window. ClaimForAttempt is the atomic gate
// that must be checked BEFORE any Thunder call or DB write — if it reports
// the binding is already claimed (e.g. a concurrent AttemptProvision call
// beat us to it), this call must be a complete no-op.
func TestAttemptProvision_ClaimFails_SkipsWithoutTouchingThunder(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.CreateAgentIdentityFunc = func(_ context.Context, _, _, _ string) (string, string, string, bool, error) {
		t.Fatal("CreateAgentIdentity must not be called when the claim was not won")
		return "", "", "", false, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) {
			t.Fatal("Resolve must not be called when the claim was not won")
			return tc, nil
		},
	}
	store := &clientmocks.AgentSecretStoreMock{}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error {
			t.Fatal("UpdateAfterAttempt must not be called when the claim was not won")
			return nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)
	binding := models.AgentThunderClient{
		ID: uuid.New(), OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "staging",
	}

	svc.AttemptProvision(context.Background(), binding)
	// Reaching here without a t.Fatal means the no-op guard held.
}

func TestProvisionForAgent_WritesAheadPendingForEveryEnvironment(t *testing.T) {
	var upserted []models.AgentThunderClient
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		UpsertFunc: func(_ context.Context, c *models.AgentThunderClient) error {
			upserted = append(upserted, *c)
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) {
			// Block forever from the caller's perspective is unnecessary — just
			// return an error so the background attempt fails harmlessly; this
			// test only cares about the synchronous write-ahead behavior.
			return nil, errors.New("unused in this test")
		},
	}
	store := &clientmocks.AgentSecretStoreMock{}
	repo.UpdateAfterAttemptFunc = func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil }

	svc := newTestProvisioningService(repo, resolver, store)
	svc.ProvisionForAgent(context.Background(), "acme", "proj1", "my-agent", models.AgentProvisioningTypeInternal, []string{"dev", "staging", "prod"}, "platform-thunder-subject-abc123")

	require.Len(t, upserted, 3, "must write one PENDING row per environment, synchronously, before returning")
	envs := []string{upserted[0].EnvironmentName, upserted[1].EnvironmentName, upserted[2].EnvironmentName}
	assert.ElementsMatch(t, []string{"dev", "staging", "prod"}, envs)
	for _, u := range upserted {
		assert.Equal(t, models.AgentThunderStatusPending, u.Status)
		assert.Equal(t, models.AgentProvisioningTypeInternal, u.ProvisioningType)
		assert.Equal(t, "platform-thunder-subject-abc123", u.RequestedBy,
			"the caller's identity must be captured on every environment's write-ahead row, for audit — never sent to Thunder itself")
	}
}

func TestRegenerateSecret_Success(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, thunderAgentID string) (string, error) {
		assert.Equal(t, "thunder-agent-1", thunderAgentID)
		return "new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var storedSecret, storedPath string
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, _, _, _, _, _, clientSecret string) (string, error) {
			storedSecret = clientSecret
			storedPath = "agent-thunder-clients/acme/proj1/staging/my-agent"
			return storedPath, nil
		},
	}
	var updatedPath string
	var clearedClaimForID uuid.UUID
	clearClaimCalled := false
	bindingID := uuid.New()
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: bindingID, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeExternal,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			updatedPath = secretRefPath
			return nil
		},
		ClearClaimFunc: func(_ context.Context, id uuid.UUID) error {
			clearClaimCalled = true
			clearedClaimForID = id
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	ownership, clientID, newSecret, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeExternal, ownership)
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "new-secret", newSecret)
	assert.Equal(t, "new-secret", storedSecret)
	assert.Equal(t, storedPath, updatedPath)
	assert.True(t, clearClaimCalled, "regenerate must clear any prior claim so the new secret is eligible for the one-time claim again — otherwise GetIdentityViews will never show it, even though it's a valid, never-before-seen secret")
	assert.Equal(t, bindingID, clearedClaimForID)
}

// TestRegenerateSecret_ConcurrentCallsForSameBinding_Serialized guards against
// interleaving two rotations of the same binding: without a per-binding lock,
// two concurrent RegenerateSecret calls could rotate Thunder in one order but
// write to OpenBao in the other, leaving the stored secret mismatched with
// whatever Thunder actually considers active.
func TestRegenerateSecret_ConcurrentCallsForSameBinding_Serialized(t *testing.T) {
	var inFlight int32
	var maxObservedInFlight int32
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			max := atomic.LoadInt32(&maxObservedInFlight)
			if n <= max || atomic.CompareAndSwapInt32(&maxObservedInFlight, max, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // widen the race window
		atomic.AddInt32(&inFlight, -1)
		return "rotated-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, _, _, _, _, _, clientSecret string) (string, error) {
			return "agent-thunder-clients/acme/proj1/staging/my-agent", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeExternal,
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
		ClearClaimFunc:      func(_ context.Context, _ uuid.UUID) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, atomic.LoadInt32(&maxObservedInFlight),
		"concurrent RegenerateSecret calls for the same binding must be serialized, not interleaved")
}

// TestRegenerateSecret_ThenGetIdentityViews_NewSecretIsClaimableAgain is an
// end-to-end regression test for the exact bug found live: regenerating an
// already-once-claimed External environment's secret, then immediately
// checking identity, must show the NEW secret — not silently omit it because
// a stale claimed_at from the OLD secret survived the regenerate.
func TestRegenerateSecret_ThenGetIdentityViews_NewSecretIsClaimableAgain(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(context.Context, string) (string, error) { return "brand-new-secret", nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}

	bindingID := uuid.New()
	alreadyClaimedAt := time.Now().Add(-1 * time.Hour) // simulates a prior ClaimSecret claim
	var currentSecretRef string
	currentClaimedAt := &alreadyClaimedAt

	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: bindingID, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusCompleted,
				SecretRefPath: currentSecretRef, ClaimedAt: currentClaimedAt,
			}, nil
		},
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: bindingID, EnvironmentName: "staging", ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusCompleted,
				SecretRefPath: currentSecretRef, ClaimedAt: currentClaimedAt,
			}}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			currentSecretRef = secretRefPath
			return nil
		},
		ClearClaimFunc: func(_ context.Context, _ uuid.UUID) error { currentClaimedAt = nil; return nil },
		MarkClaimedFunc: func(_ context.Context, _ uuid.UUID, t time.Time) (bool, error) {
			if currentClaimedAt != nil {
				return false, nil
			}
			currentClaimedAt = &t
			return true, nil
		},
	}

	var storedSecret string
	store := &clientmocks.AgentSecretStoreMock{
		StoreFunc: func(_ context.Context, _, _, _, _, _, clientSecret string) (string, error) {
			storedSecret = clientSecret
			return "agent-thunder-clients/acme/proj1/staging/my-agent", nil
		},
		GetFunc:    func(_ context.Context, _ string) (string, string, error) { return "client-abc", storedSecret, nil },
		DeleteFunc: func(context.Context, string) error { return nil },
	}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, newSecret, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.NoError(t, err)
	require.Equal(t, "brand-new-secret", newSecret)

	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.True(t, views[0].HasUnclaimedSecret,
		"the regenerated secret must be claimable — a stale claim from the OLD secret must not suppress it")

	_, _, claimedSecret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.NoError(t, err)
	assert.Equal(t, "brand-new-secret", claimedSecret)
}

func TestRegenerateSecret_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

// TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500 guards a
// distinct case from the one above: no row exists at all yet (as opposed to a
// row existing with an empty ThunderAgentID). Before the fix, this returned
// the raw repositories.ErrAgentThunderClientNotFound unwrapped, which
// handleCommonErrors has no case for — resulting in an HTTP 500 for a
// routine, foreseeable caller state (e.g. calling regenerate immediately
// after CreateAgent, before the async provisioning goroutine has run).
func TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestRevokeSecret_RotatesAndClearsStoredCopy(t *testing.T) {
	tc := fakeThunderClientMock()
	rotateCalled := false
	tc.RegenerateAgentSecretFunc = func(_ context.Context, _ string) (string, error) {
		rotateCalled = true
		return "unused-new-secret", nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	deleteCalled := false
	store := &clientmocks.AgentSecretStoreMock{
		DeleteFunc: func(_ context.Context, secretPath string) error {
			deleteCalled = true
			assert.Equal(t, "existing/path", secretPath)
			return nil
		},
	}
	var clearedTo string
	clearedToSet := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc", SecretRefPath: "existing/path",
			}, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			clearedTo = secretRefPath
			clearedToSet = true
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, "client-abc", clientID, "revoke must return the (unchanged) client ID so the caller can build a response body instead of an empty 204")
	assert.True(t, rotateCalled, "revoke must invalidate the old secret in Thunder")
	assert.True(t, deleteCalled, "revoke must remove the stale stored copy, not leave an invalid secret sitting in the vault")
	require.True(t, clearedToSet)
	assert.Empty(t, clearedTo, "after revoke there must be no currently-valid stored secret until an explicit regenerate")
}

func TestRevokeSecret_NotYetProvisioned_Errors(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	assert.Empty(t, clientID)
}

// TestRevokeSecret_NoBindingRow_ReturnsNotProvisioned_Not500 is the Revoke
// counterpart to TestRegenerateSecret_NoBindingRow_ReturnsNotProvisioned_Not500
// — see that test's comment for why the no-row case is distinct and was
// previously unhandled.
func TestRevokeSecret_NoBindingRow_ReturnsNotProvisioned_Not500(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	clientID, err := svc.RevokeSecret(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	assert.Empty(t, clientID)
}

func TestGetIdentityViews_ExternalUnclaimed_IsSafeRead(t *testing.T) {
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("GetIdentityViews must never read the secret store — it is a safe, non-destructive read")
			return "", "", nil
		},
		DeleteFunc: func(context.Context, string) error {
			t.Fatal("GetIdentityViews must never destroy a secret — that is ClaimSecret's job")
			return nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-uuid-1", ThunderClientID: "client-abc",
				SecretRefPath: "some/path", ClaimedAt: nil,
			}}, nil
		},
		MarkClaimedFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) (bool, error) {
			t.Fatal("GetIdentityViews must never claim a secret")
			return false, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "thunder-agent-uuid-1", views[0].AgentID, "the view must expose Thunder's own identity UUID, not just the OAuth client_id")
	assert.True(t, views[0].HasUnclaimedSecret)
}

func TestClaimSecret_ExternalUnclaimedSecret_ReturnedOnceThenDestroyed(t *testing.T) {
	claimedAt := (*time.Time)(nil)
	getCalls := 0
	deleteCalls := 0
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(_ context.Context, secretPath string) (string, string, error) {
			getCalls++
			assert.Equal(t, "some/path", secretPath)
			return "client-abc", "one-time-secret", nil
		},
		DeleteFunc: func(_ context.Context, secretPath string) error {
			deleteCalls++
			assert.Equal(t, "some/path", secretPath)
			return nil
		},
	}
	var markClaimedCalled, clearedSecretRef bool
	bindingID := uuid.New()
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: bindingID, EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-uuid-1", ThunderClientID: "client-abc",
				SecretRefPath: "some/path", ClaimedAt: claimedAt,
			}, nil
		},
		MarkClaimedFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) (bool, error) {
			markClaimedCalled = true
			return true, nil
		},
		UpdateSecretRefFunc: func(_ context.Context, _ uuid.UUID, secretRefPath string) error {
			clearedSecretRef = secretRefPath == ""
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	agentID, clientID, secret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, "thunder-agent-uuid-1", agentID, "the claim response must expose Thunder's own identity UUID, not just the OAuth client_id")
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "one-time-secret", secret)
	assert.Equal(t, 1, getCalls)
	assert.Equal(t, 1, deleteCalls, "the secret must be destroyed immediately after being claimed")
	assert.True(t, markClaimedCalled)
	assert.True(t, clearedSecretRef)
}

// TestClaimSecret_ConcurrentClaim_SecretNotDoubleServed guards the fix for
// the one-time-claim race: MarkClaimed is the atomic compare-and-swap gate,
// checked BEFORE reading the secret, not after. This test simulates the
// concurrent caller having already won the claim between this call's read of
// b.ClaimedAt (nil, so canClaim==true) and its own MarkClaimed attempt — the
// mock's MarkClaimedFunc returns claimed=false to model that race outcome.
// The secret must not be read from the store or returned in that case.
func TestClaimSecret_ConcurrentClaim_SecretNotDoubleServed(t *testing.T) {
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("must not read the secret store when the claim CAS was lost to a concurrent caller")
			return "", "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				SecretRefPath: "some/path", ClaimedAt: nil, // looks unclaimed from this call's own read
			}, nil
		},
		MarkClaimedFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) (bool, error) {
			return false, nil // a concurrent caller already won the CAS
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, secret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentCredentialNotAvailable, "the losing side of a claim race must not also receive the secret")
	assert.Empty(t, secret)
}

// TestClaimSecret_SucceedsButSecretReadFails_RollsBackClaim guards a property
// that predates this fix and must survive it: if the vault read fails after
// the claim itself succeeded, the claim is rolled back (ClearClaim) so a
// future retry can still see the secret, rather than permanently losing it
// to a claim nobody was actually shown.
func TestClaimSecret_SucceedsButSecretReadFails_RollsBackClaim(t *testing.T) {
	boom := errors.New("openbao unreachable")
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			return "", "", boom
		},
	}
	var clearClaimCalledFor uuid.UUID
	bindingID := uuid.New()
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: bindingID, EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				SecretRefPath: "some/path", ClaimedAt: nil,
			}, nil
		},
		MarkClaimedFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) (bool, error) { return true, nil },
		ClearClaimFunc:  func(_ context.Context, id uuid.UUID) error { clearClaimCalledFor = id; return nil },
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, secret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.Error(t, err)
	assert.Empty(t, secret)
	assert.Equal(t, bindingID, clearClaimCalledFor, "the claim must be rolled back so a future call can still retrieve this secret")
}

func TestGetIdentityViews_ExternalAlreadyClaimed_SecretNotReturnedAgain(t *testing.T) {
	claimedAt := time.Now().Add(-1 * time.Hour)
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("must not read the secret store once a secret has already been claimed")
			return "", "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderClientID: "client-abc",
				SecretRefPath: "", ClaimedAt: &claimedAt,
			}}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.False(t, views[0].HasUnclaimedSecret)
	assert.Equal(t, "client-abc", views[0].ClientID)
}

func TestClaimSecret_AlreadyClaimed_ReturnsCredentialNotAvailable(t *testing.T) {
	claimedAt := time.Now().Add(-1 * time.Hour)
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("must not read the secret store once a secret has already been claimed")
			return "", "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), EnvironmentName: "staging", ProvisioningType: models.AgentProvisioningTypeExternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc",
				SecretRefPath: "", ClaimedAt: &claimedAt,
			}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, secret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentCredentialNotAvailable)
	assert.Empty(t, secret)
}

func TestGetIdentityViews_Internal_SecretNeverReturned(t *testing.T) {
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("an internal agent's secret must never be read for display, even if it exists")
			return "", "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{{
				ID: uuid.New(), EnvironmentName: "prod", ProvisioningType: models.AgentProvisioningTypeInternal,
				Status: models.AgentThunderStatusCompleted, ThunderClientID: "client-xyz",
				SecretRefPath: "internal/secret/path", RequestedBy: "platform-thunder-subject-abc123",
			}}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	views, err := svc.GetIdentityViews(context.Background(), "acme", "proj1", "my-agent")

	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.False(t, views[0].HasUnclaimedSecret)
	assert.Equal(t, "client-xyz", views[0].ClientID)
	assert.Equal(t, "platform-thunder-subject-abc123", views[0].RequestedBy,
		"the audit trail must be visible via GET .../identity regardless of ownership type")
}

func TestClaimSecret_Internal_RejectedAsInvalidInput(t *testing.T) {
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("an internal agent's secret must never be read via claim")
			return "", "", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), EnvironmentName: "prod", ProvisioningType: models.AgentProvisioningTypeInternal,
				Status: models.AgentThunderStatusCompleted, ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-xyz",
				SecretRefPath: "internal/secret/path",
			}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, _, secret, err := svc.ClaimSecret(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Empty(t, secret)
}

func TestDeleteAllBindings_DeletesThunderIdentitiesSecretsAndRows(t *testing.T) {
	tc := fakeThunderClientMock()
	var deletedIdentities []string
	tc.DeleteAgentIdentityFunc = func(_ context.Context, thunderAgentID string) (bool, error) {
		deletedIdentities = append(deletedIdentities, thunderAgentID)
		return true, nil
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	var deletedSecretPaths []string
	store := &clientmocks.AgentSecretStoreMock{
		DeleteFunc: func(_ context.Context, secretPath string) error {
			deletedSecretPaths = append(deletedSecretPaths, secretPath)
			return nil
		},
	}
	deleteByAgentCalled := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentThunderClient, error) {
			return []models.AgentThunderClient{
				{ThunderAgentID: "agent-in-dev", EnvironmentName: "dev", SecretRefPath: "path/dev"},
				{ThunderAgentID: "agent-in-prod", EnvironmentName: "prod", SecretRefPath: "path/prod"},
				{ThunderAgentID: "", EnvironmentName: "never-provisioned"}, // never made it to Thunder — nothing to delete there
			}, nil
		},
		DeleteByAgentFunc: func(_ context.Context, _, _, _ string) error {
			deleteByAgentCalled = true
			return nil
		},
	}

	svc := newTestProvisioningService(repo, resolver, store)
	svc.DeleteAllBindings(context.Background(), "acme", "proj1", "my-agent")

	assert.ElementsMatch(t, []string{"agent-in-dev", "agent-in-prod"}, deletedIdentities)
	assert.ElementsMatch(t, []string{"path/dev", "path/prod"}, deletedSecretPaths)
	assert.True(t, deleteByAgentCalled)
}

// TestProvisionForEnvironmentIfMissing_* cover the shared helper behind both the
// external-agent identity-provision endpoint and PromoteAgent's internal-agent
// hook: an environment that appeared (or entered the pipeline) after the agent
// was first created still gets an AgentID once it's actually needed there.

func TestProvisionForEnvironmentIfMissing_AlreadyExists_LeavesBindingUntouched(t *testing.T) {
	getCalled := false
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, org, project, agent, env string) (*models.AgentThunderClient, error) {
			getCalled = true
			assert.Equal(t, "acme", org)
			assert.Equal(t, "proj1", project)
			assert.Equal(t, "my-agent", agent)
			assert.Equal(t, "new-env", env)
			return &models.AgentThunderClient{ID: uuid.New(), Status: models.AgentThunderStatusCompleted}, nil
		},
		UpsertFunc: func(_ context.Context, _ *models.AgentThunderClient) error {
			t.Fatal("must not write a new binding when one already exists")
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeExternal, "user-1",
	)

	require.NoError(t, err)
	assert.True(t, alreadyExisted)
	assert.True(t, getCalled)
}

func TestProvisionForEnvironmentIfMissing_Missing_WritesAheadAndAttempts(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	var upserted *models.AgentThunderClient
	upsertDone := make(chan struct{})
	repo.UpsertFunc = func(_ context.Context, c *models.AgentThunderClient) error {
		upserted = c
		close(upsertDone)
		return nil
	}
	// The background attempt (triggered by the missing-binding path) will run and
	// fail fast against the resolver below — give it somewhere harmless to land
	// instead of panicking on a nil mock function.
	repo.ClaimForAttemptFunc = func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }
	repo.UpdateAfterAttemptFunc = func(_ context.Context, _ uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error { return nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(_ context.Context, _, _ string) (thundersvc.ThunderClient, error) {
			return nil, thundersvc.ErrThunderNotProvisioned // attempt fails fast; not under test here
		},
	}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeInternal, "user-1",
	)

	require.NoError(t, err)
	assert.False(t, alreadyExisted)

	select {
	case <-upsertDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected a write-ahead binding to be upserted for the missing environment")
	}
	require.NotNil(t, upserted)
	assert.Equal(t, "new-env", upserted.EnvironmentName)
	assert.Equal(t, models.AgentProvisioningTypeInternal, upserted.ProvisioningType)
	assert.Equal(t, "user-1", upserted.RequestedBy)
}

func TestProvisionForEnvironmentIfMissing_RepoErrorPropagates(t *testing.T) {
	boom := errors.New("connection refused")
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, boom
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	_, err := svc.ProvisionForEnvironmentIfMissing(
		context.Background(), "acme", "proj1", "my-agent", "new-env", models.AgentProvisioningTypeExternal, "user-1",
	)

	require.Error(t, err)
	assert.False(t, errors.Is(err, repositories.ErrAgentThunderClientNotFound), "a real repo error must not be mistaken for not-found")
}

// TestGetCredentials_* cover the repeatable credential-retrieval path used by
// the Internal-agent-only GET .../identity/credentials endpoint — deliberately
// separate from GetIdentityViews' one-time External claim, since these callers
// (a workload, an admin, future injection tooling) may need the credential more
// than once before Gateway Binding ships.

func TestGetCredentials_Success_DoesNotDestroyStoredSecret(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, org, project, agent, env string) (*models.AgentThunderClient, error) {
			assert.Equal(t, "acme", org)
			assert.Equal(t, "proj1", project)
			assert.Equal(t, "my-agent", agent)
			assert.Equal(t, "staging", env)
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "thunder-agent-1", ThunderClientID: "client-abc", SecretRefPath: "path/to/secret",
			}, nil
		},
	}
	deleteCalled := false
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(_ context.Context, secretPath string) (string, string, error) {
			assert.Equal(t, "path/to/secret", secretPath)
			return "client-abc", "s3cr3t", nil
		},
		DeleteFunc: func(context.Context, string) error {
			deleteCalled = true
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}

	svc := newTestProvisioningService(repo, resolver, store)
	agentID, clientID, clientSecret, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, "thunder-agent-1", agentID, "must return Thunder's own identity UUID, not just the OAuth client_id")
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "s3cr3t", clientSecret)
	assert.False(t, deleteCalled, "GetCredentials must be repeatable — it must never destroy the stored secret")
}

func TestGetCredentials_CalledTwice_BothCallsSucceed(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1", SecretRefPath: "path/to/secret"}, nil
		},
	}
	getCalls := 0
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(_ context.Context, _ string) (string, string, error) {
			getCalls++
			return "client-abc", "s3cr3t", nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err1 := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	_, _, _, err2 := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, 2, getCalls, "a second call must succeed exactly like the first — nothing was consumed")
}

func TestGetCredentials_NotYetProvisioned_NoBindingRow(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestGetCredentials_NotYetProvisioned_EmptyThunderAgentID(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New()}, nil // ThunderAgentID empty — still pending
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestGetCredentials_NoCredentialAvailable_AfterRevoke(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1", SecretRefPath: ""}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			t.Fatal("must not call the secret store when there is no stored secret path")
			return "", "", nil
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentCredentialNotAvailable)
	assert.False(t, errors.Is(err, utils.ErrAgentIdentityNotProvisioned), "revoked-but-completed must not be mistaken for never-provisioned")
}

func TestGetCredentials_SecretStoreErrorPropagates(t *testing.T) {
	boom := errors.New("openbao unreachable")
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1", SecretRefPath: "path/to/secret"}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			return "", "", boom
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.False(t, errors.Is(err, utils.ErrAgentIdentityNotProvisioned))
	assert.False(t, errors.Is(err, utils.ErrAgentCredentialNotAvailable))
}

// TestGetCredentials_SecretMissingFromStore_ReturnsCredentialNotAvailable
// guards the case where SecretRefPath is non-empty (so it passes the earlier
// check) but the OpenBao key itself has already been deleted — e.g. a
// concurrent RevokeSecret or a prior GetIdentityViews claim-and-destroy raced
// with this call. Before the fix, this fell through to the generic wrapped
// error below instead of the same 404-mapped utils.ErrAgentCredentialNotAvailable
// used a few lines above for the "SecretRefPath empty" case — an inconsistency
// for what is, from the caller's perspective, the exact same situation
// ("nothing usable is stored right now").
func TestGetCredentials_SecretMissingFromStore_ReturnsCredentialNotAvailable(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{ID: uuid.New(), ThunderAgentID: "thunder-agent-1", SecretRefPath: "path/to/secret"}, nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{}
	store := &clientmocks.AgentSecretStoreMock{
		GetFunc: func(context.Context, string) (string, string, error) {
			return "", "", thundersvc.ErrAgentSecretNotFound
		},
	}
	svc := newTestProvisioningService(repo, resolver, store)

	_, _, _, err := svc.GetCredentials(context.Background(), "acme", "proj1", "my-agent", "staging")
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrAgentCredentialNotAvailable)
}
