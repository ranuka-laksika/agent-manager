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

func cleanupEnvThunderSystemClient(t *testing.T, repo EnvThunderSystemClientRepository, org, env string) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.Delete(context.Background(), org, env)
	})
}

// TestEnvThunderSystemClientRepo_UpsertIsIdempotentOnRetry is the critical
// correctness property add-environment-thunder.sh's retry-on-failure story
// depends on: re-running the provisioning script (e.g. after a transient AMS
// failure) re-PUTs the same (org, env) with a possibly-different secret value
// (a fresh mint if the K8s Secret itself also had to be regenerated). That
// second write must overwrite in place — never fail with a duplicate-key
// error, never create a second row.
func TestEnvThunderSystemClientRepo_UpsertIsIdempotentOnRetry(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())
	const org, env = "test-org", "env-thunder-upsert-retry"
	cleanupEnvThunderSystemClient(t, repo, org, env)

	first := &models.EnvThunderSystemClient{
		OrgName:               org,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext-v1"),
	}
	require.NoError(t, repo.Upsert(context.Background(), first))

	got, err := repo.Get(context.Background(), org, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("ciphertext-v1"), got.ClientSecretEncrypted)

	// Simulate the retry: same (org, env), different ciphertext (a fresh mint).
	// Must overwrite, not error or duplicate.
	second := &models.EnvThunderSystemClient{
		OrgName:               org,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext-v2-after-retry"),
	}
	require.NoError(t, repo.Upsert(context.Background(), second), "a retry must overwrite the existing row via ON CONFLICT, not fail with a duplicate-key error")

	got, err = repo.Get(context.Background(), org, env)
	require.NoError(t, err)
	assert.Equal(t, []byte("ciphertext-v2-after-retry"), got.ClientSecretEncrypted, "the retry's value must win")

	var count int64
	require.NoError(t, db.GetDB().Model(&models.EnvThunderSystemClient{}).
		Where("org_name = ? AND env_name = ?", org, env).Count(&count).Error)
	assert.Equal(t, int64(1), count, "a retried Upsert must never create a second row for the same (org, env)")
}

func TestEnvThunderSystemClientRepo_GetNotFound(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())

	_, err := repo.Get(context.Background(), "no-such-org", "no-such-env")
	require.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

// TestEnvThunderSystemClientRepo_DeleteIsIdempotent mirrors
// remove-environment-thunder.sh's expectation that deleting an
// already-removed (or never-created) credential is not an error — teardown
// must succeed even on a retry.
func TestEnvThunderSystemClientRepo_DeleteIsIdempotent(t *testing.T) {
	repo := NewEnvThunderSystemClientRepo(db.GetDB())
	const org, env = "test-org", "env-thunder-delete-idempotent"

	require.NoError(t, repo.Delete(context.Background(), org, env), "deleting a non-existent row must not be an error")

	require.NoError(t, repo.Upsert(context.Background(), &models.EnvThunderSystemClient{
		OrgName:               org,
		EnvName:               env,
		ClientID:              "amp-system-client",
		ClientSecretEncrypted: []byte("ciphertext"),
	}))
	require.NoError(t, repo.Delete(context.Background(), org, env))
	_, err := repo.Get(context.Background(), org, env)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))

	// Deleting again (simulating a retried teardown) must still succeed.
	require.NoError(t, repo.Delete(context.Background(), org, env))
}
