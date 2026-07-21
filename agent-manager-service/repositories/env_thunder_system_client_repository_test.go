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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func cleanupEnvThunderSystemClient(t *testing.T, repo EnvThunderSystemClientRepository, ouID, env string) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.Delete(context.Background(), ouID, env)
	})
}

// TestEnvThunderSystemClientRepo_UpsertIsIdempotentOnRetry is the critical
// correctness property add-environment-thunder.sh's retry-on-failure story
// depends on: re-running the provisioning script (e.g. after a transient AMS
// failure) re-PUTs the same (ouID, env), possibly with a different secret (a
// fresh mint if the K8s Secret itself also had to be regenerated). That
// second write must overwrite in place — never fail with a duplicate-key
// error, never create a second row.
func TestEnvThunderSystemClientRepo_UpsertIsIdempotentOnRetry(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())
	const ouID, env = "ou-test-retry", "env-thunder-upsert-retry"
	cleanupEnvThunderSystemClient(t, repo, ouID, env)

	first := &models.EnvThunderSystemClient{
		OUID:                  ouID,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext-v1"),
	}
	require.NoError(t, repo.Upsert(context.Background(), first))

	got, err := repo.Get(context.Background(), ouID, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("ciphertext-v1"), got.ClientSecretEncrypted)

	// Simulate the retry: same (ouID, env) — the lookup key — but a different
	// ciphertext (a fresh mint). Must overwrite, not error or duplicate.
	second := &models.EnvThunderSystemClient{
		OUID:                  ouID,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext-v2-after-retry"),
	}
	require.NoError(t, repo.Upsert(context.Background(), second), "a retry must overwrite the existing row via ON CONFLICT, not fail with a duplicate-key error")

	got, err = repo.Get(context.Background(), ouID, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("ciphertext-v2-after-retry"), got.ClientSecretEncrypted, "the retry's value must win")

	var count int64
	require.NoError(t, db.GetDB().Model(&models.EnvThunderSystemClient{}).
		Where("ou_id = ? AND env_name = ?", ouID, env).Count(&count).Error)
	assert.Equal(t, int64(1), count, "a retried Upsert must never create a second row for the same (ouID, env)")
}

// TestEnvThunderSystemClientRepo_DifferentOUIDsAreDistinctRows is the
// multi-tenant-safety property migration 036 exists for: the same env_name
// under two different OU IDs must never collide on the same credential row
// (unlike the old org_name-keyed shape, where two tenants sharing a handle
// once real multi-tenant SaaS arrives would have collided).
func TestEnvThunderSystemClientRepo_DifferentOUIDsAreDistinctRows(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())
	const env = "env-thunder-multi-tenant"
	const ouA, ouB = "ou-tenant-a", "ou-tenant-b"
	cleanupEnvThunderSystemClient(t, repo, ouA, env)
	cleanupEnvThunderSystemClient(t, repo, ouB, env)

	require.NoError(t, repo.Upsert(context.Background(), &models.EnvThunderSystemClient{
		OUID: ouA, EnvName: env,
		ClientID: "amp-system-client", ClientSecretEncrypted: []byte("secret-a"),
	}))
	require.NoError(t, repo.Upsert(context.Background(), &models.EnvThunderSystemClient{
		OUID: ouB, EnvName: env,
		ClientID: "amp-system-client", ClientSecretEncrypted: []byte("secret-b"),
	}))

	gotA, err := repo.Get(context.Background(), ouA, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("secret-a"), gotA.ClientSecretEncrypted)

	gotB, err := repo.Get(context.Background(), ouB, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("secret-b"), gotB.ClientSecretEncrypted, "a second tenant with the same env_name must get its own row, not overwrite the first")
}

func TestEnvThunderSystemClientRepo_GetNotFound(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())

	_, err := repo.Get(context.Background(), "no-such-ou", "no-such-env")
	require.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

// TestEnvThunderSystemClientRepo_DeleteIsIdempotent mirrors
// remove-environment-thunder.sh's expectation that deleting an
// already-removed (or never-created) credential is not an error — teardown
// must succeed even on a retry.
func TestEnvThunderSystemClientRepo_DeleteIsIdempotent(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())
	const ouID, env = "ou-test-delete-idempotent", "env-thunder-delete-idempotent"

	require.NoError(t, repo.Delete(context.Background(), ouID, env), "deleting a non-existent row must not be an error")

	require.NoError(t, repo.Upsert(context.Background(), &models.EnvThunderSystemClient{
		OUID:                  ouID,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext"),
	}))
	require.NoError(t, repo.Delete(context.Background(), ouID, env))
	_, err := repo.Get(context.Background(), ouID, env)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))

	// Deleting again (simulating a retried teardown) must still succeed.
	require.NoError(t, repo.Delete(context.Background(), ouID, env))
}
