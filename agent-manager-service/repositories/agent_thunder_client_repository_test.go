//go:build integration

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

package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newTestAgentThunderClient(org, project, agent, env string) *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OrgName:          org,
		ProjectName:      project,
		AgentName:        agent,
		EnvironmentName:  env,
		ProvisioningType: models.AgentProvisioningTypeExternal,
		Status:           models.AgentThunderStatusPending,
	}
}

func cleanupAgentThunderClients(t *testing.T, repo AgentThunderClientRepository, org, project, agent string) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DeleteByAgent(context.Background(), org, project, agent)
	})
}

func TestAgentThunderClientRepo_UpsertAndGet(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-upsert-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))

	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, got.Status)
	assert.Equal(t, models.AgentProvisioningTypeExternal, got.ProvisioningType)

	// Upsert is only ever called to write-ahead a BRAND NEW binding (see
	// ProvisionForAgent) — every real caller already confirmed via Get() that
	// no row exists yet. A second Upsert on the same key models a race between
	// two such callers (e.g. the explicit provision endpoint racing
	// PromoteAgent's hook): it must be a no-op that leaves the winner's row
	// untouched, not an overwrite — silently clobbering an already-completed
	// Thunder identity/secret reference back to blank would orphan it.
	c.ProvisioningType = models.AgentProvisioningTypeInternal
	require.NoError(t, repo.Upsert(context.Background(), c))

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a conflicting upsert must not insert a second row")
	assert.Equal(t, models.AgentProvisioningTypeExternal, rows[0].ProvisioningType,
		"a conflicting upsert must leave the existing row's fields untouched, not overwrite them")
}

func TestAgentThunderClientRepo_Upsert_PersistsRequestedBy(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-requested-by-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	c.RequestedBy = "platform-thunder-subject-abc123"
	require.NoError(t, repo.Upsert(context.Background(), c))

	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "platform-thunder-subject-abc123", got.RequestedBy)

	// A conflicting upsert (see the comment in TestAgentThunderClientRepo_UpsertAndGet
	// for why this must be a no-op) must not overwrite the original requester.
	c.RequestedBy = "platform-thunder-subject-xyz789"
	require.NoError(t, repo.Upsert(context.Background(), c))
	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "platform-thunder-subject-abc123", updated.RequestedBy)
}

func TestAgentThunderClientRepo_Get_NotFound(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	_, err := repo.Get(context.Background(), "test-org", "test-proj", "atc-missing-agent", "dev")
	assert.True(t, errors.Is(err, ErrAgentThunderClientNotFound))
}

func TestAgentThunderClientRepo_FindByAgent_MultipleEnvironments(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-multi-env-agent"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	for _, env := range []string{"dev", "staging", "prod"} {
		require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, env)))
	}

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	envs := []string{rows[0].EnvironmentName, rows[1].EnvironmentName, rows[2].EnvironmentName}
	assert.ElementsMatch(t, []string{"dev", "staging", "prod"}, envs)
}

func TestAgentThunderClientRepo_FindDue(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-find-due-agent"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	// A brand-new write-ahead row (no next_retry_at yet) is due immediately.
	fresh := newTestAgentThunderClient(org, project, agent, "dev")
	require.NoError(t, repo.Upsert(context.Background(), fresh))

	// A row scheduled in the future is not due yet.
	future := newTestAgentThunderClient(org, project, agent, "staging")
	nextRetry := time.Now().Add(1 * time.Hour)
	future.NextRetryAt = &nextRetry
	require.NoError(t, repo.Upsert(context.Background(), future))

	// A row scheduled in the past is due.
	past := newTestAgentThunderClient(org, project, agent, "prod")
	pastRetry := time.Now().Add(-1 * time.Minute)
	past.NextRetryAt = &pastRetry
	require.NoError(t, repo.Upsert(context.Background(), past))

	// A completed row must never be picked up regardless of next_retry_at.
	completed := newTestAgentThunderClient(org, project, agent, "qa")
	completed.Status = models.AgentThunderStatusCompleted
	require.NoError(t, repo.Upsert(context.Background(), completed))

	due, err := repo.FindDue(context.Background(), time.Now(), 100)
	require.NoError(t, err)

	dueEnvs := make(map[string]bool)
	for _, row := range due {
		if row.OrgName == org && row.ProjectName == project && row.AgentName == agent {
			dueEnvs[row.EnvironmentName] = true
		}
	}
	assert.True(t, dueEnvs["dev"], "a fresh row with no next_retry_at must be due immediately")
	assert.True(t, dueEnvs["prod"], "a row whose next_retry_at is in the past must be due")
	assert.False(t, dueEnvs["staging"], "a row scheduled in the future must not be due yet")
	assert.False(t, dueEnvs["qa"], "a completed row must never be due")
}

func TestAgentThunderClientRepo_UpdateAfterAttempt_Success(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-attempt-success-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	err = repo.UpdateAfterAttempt(context.Background(), got.ID, AgentThunderAttemptUpdate{
		Status:          models.AgentThunderStatusCompleted,
		ThunderAgentID:  "thunder-agent-uuid",
		ThunderClientID: "client-id-123",
	})
	require.NoError(t, err)

	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusCompleted, updated.Status)
	assert.Equal(t, "thunder-agent-uuid", updated.ThunderAgentID)
	assert.Equal(t, "client-id-123", updated.ThunderClientID)
	assert.Equal(t, 1, updated.AttemptCount)
	assert.Nil(t, updated.NextRetryAt)
	assert.NotNil(t, updated.LastAttemptedAt)
}

func TestAgentThunderClientRepo_UpdateAfterAttempt_FailureSchedulesRetry(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-attempt-fail-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	next := time.Now().Add(1 * time.Minute)
	err = repo.UpdateAfterAttempt(context.Background(), got.ID, AgentThunderAttemptUpdate{
		Status:      models.AgentThunderStatusPending,
		LastError:   "thunder instance unreachable",
		NextRetryAt: &next,
	})
	require.NoError(t, err)

	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, updated.Status)
	assert.Equal(t, "thunder instance unreachable", updated.LastError)
	assert.Equal(t, 1, updated.AttemptCount)
	require.NotNil(t, updated.NextRetryAt)
	assert.WithinDuration(t, next, *updated.NextRetryAt, time.Second)
	// A failed attempt must never clobber an identity already established by an
	// earlier successful attempt on the same row (e.g. Thunder created the agent
	// but a later step failed) — ThunderAgentID/ClientID were empty in this update,
	// so the previously-empty values are simply left as-is here (nothing to lose),
	// but the zero-value guard in UpdateAfterAttempt is what protects a non-empty
	// prior value from being overwritten by an empty one.
	assert.Empty(t, updated.ThunderAgentID)
}

func TestAgentThunderClientRepo_MarkClaimed(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-claim-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Nil(t, got.ClaimedAt)

	claimedAt := time.Now()
	claimed, err := repo.MarkClaimed(context.Background(), got.ID, claimedAt)
	require.NoError(t, err)
	assert.True(t, claimed, "the first claim on an unclaimed binding must succeed")

	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	require.NotNil(t, updated.ClaimedAt)
	assert.WithinDuration(t, claimedAt, *updated.ClaimedAt, time.Second)
}

func TestAgentThunderClientRepo_MarkClaimed_SecondClaimFails(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-claim-cas-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	firstClaimed, err := repo.MarkClaimed(context.Background(), got.ID, time.Now())
	require.NoError(t, err)
	require.True(t, firstClaimed)

	// A second claim attempt on the same already-claimed binding must fail —
	// this is the compare-and-swap that makes the one-time secret claim
	// actually atomic against a concurrent duplicate request.
	secondClaimed, err := repo.MarkClaimed(context.Background(), got.ID, time.Now())
	require.NoError(t, err)
	assert.False(t, secondClaimed, "a binding that is already claimed must not be claimable again")
}

func TestAgentThunderClientRepo_ClearClaim(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-clearclaim-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	wasClaimed, err := repo.MarkClaimed(context.Background(), got.ID, time.Now())
	require.NoError(t, err)
	require.True(t, wasClaimed)

	claimed, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	require.NotNil(t, claimed.ClaimedAt, "precondition: must actually be claimed before clearing")

	require.NoError(t, repo.ClearClaim(context.Background(), got.ID))

	cleared, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Nil(t, cleared.ClaimedAt, "a regenerated secret must be eligible for the one-time claim again")
}

func TestAgentThunderClientRepo_UpdateSecretRef(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "atc-secretref-agent", "dev"
	cleanupAgentThunderClients(t, repo, org, project, agent)

	c := newTestAgentThunderClient(org, project, agent, env)
	require.NoError(t, repo.Upsert(context.Background(), c))
	got, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateSecretRef(context.Background(), got.ID, "agent-thunder-clients/test-org/test-proj/dev/atc-secretref-agent"))
	updated, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Equal(t, "agent-thunder-clients/test-org/test-proj/dev/atc-secretref-agent", updated.SecretRefPath)

	// Clearing it back to "" (the revoke case) must work too.
	require.NoError(t, repo.UpdateSecretRef(context.Background(), got.ID, ""))
	cleared, err := repo.Get(context.Background(), org, project, agent, env)
	require.NoError(t, err)
	assert.Empty(t, cleared.SecretRefPath)
}

func TestAgentThunderClientRepo_DeleteByAgent(t *testing.T) {
	repo := NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "atc-delete-agent"

	require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, "dev")))
	require.NoError(t, repo.Upsert(context.Background(), newTestAgentThunderClient(org, project, agent, "prod")))

	require.NoError(t, repo.DeleteByAgent(context.Background(), org, project, agent))

	rows, err := repo.FindByAgent(context.Background(), org, project, agent)
	require.NoError(t, err)
	assert.Empty(t, rows)
}
