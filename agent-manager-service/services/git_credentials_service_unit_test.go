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

// Unit tests for gitCredentialsService. Like agent_kind_service_unit_test.go
// (the reference), this file carries NO `//go:build integration` tag, so it runs
// in the fast unit tier with the OpenChoreo dependency mocked.
//
// gitCredentialsService.GetGitCredentials runs two collaborators in sequence:
//
//  1. ocClient.GetSecretReference  (mocked via clientmocks.OpenChoreoClientMock)
//  2. vaultClient.Logical().ReadWithContext (a concrete *vault.Client, NOT an
//     interface, so it cannot be mocked here)
//
// We therefore drive every branch the service owns that is reachable BEFORE the
// vault read: the empty-secretRef gate, OpenChoreo error propagation, the
// nil/empty SecretReferenceInfo gate, and the empty-KV-path gate. We also cover
// the pure helper convertToKV2ReadPath directly and the constructor happy path.
// The post-vault parsing branches require a live/fake Vault and belong to the
// integration tier; they are noted as out of scope below.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// testConfig returns a minimal config that lets NewGitCredentialsService succeed.
// The service only reads cfg.WorkflowPlaneOpenBao.{URL,Token}.
func gitTestConfig() config.Config {
	return config.Config{
		WorkflowPlaneOpenBao: config.OpenBaoConfig{
			URL:   "http://localhost:8200",
			Token: "test-token",
		},
	}
}

// newGitCredsService builds the service under test with the supplied OpenChoreo mock.
func newGitCredsService(t *testing.T, oc *clientmocks.OpenChoreoClientMock) GitCredentialsService {
	t.Helper()
	svc, err := NewGitCredentialsService(oc, gitTestConfig())
	require.NoError(t, err)
	require.NotNil(t, svc)
	return svc
}

// -----------------------------------------------------------------------------
// NewGitCredentialsService — constructor wiring.
// -----------------------------------------------------------------------------

func TestNewGitCredentialsService(t *testing.T) {
	t.Run("succeeds with a valid workflow-plane OpenBao config", func(t *testing.T) {
		svc, err := NewGitCredentialsService(&clientmocks.OpenChoreoClientMock{}, gitTestConfig())
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

// -----------------------------------------------------------------------------
// GetGitCredentials — validation gates and OpenChoreo error mapping. These are
// the branches the service owns up to (but not including) the Vault read.
// -----------------------------------------------------------------------------

func TestGitCredentialsService_GetGitCredentials_Gates(t *testing.T) {
	const org = "acme"

	t.Run("rejects an empty secretRef before touching any collaborator", func(t *testing.T) {
		// GetSecretReferenceFunc is left nil: it MUST NOT be called. An
		// unconfigured moq method panics, so reaching it would fail the test.
		svc := newGitCredsService(t, &clientmocks.OpenChoreoClientMock{})

		creds, err := svc.GetGitCredentials(context.Background(), org, "")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "secretRef is required")
	})

	t.Run("propagates the OpenChoreo GetSecretReference error", func(t *testing.T) {
		boom := errors.New("openchoreo unreachable")
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*occlient.SecretReferenceInfo, error) {
				return nil, boom
			},
		}
		svc := newGitCredsService(t, oc)

		creds, err := svc.GetGitCredentials(context.Background(), org, "git-secret")

		require.Error(t, err)
		assert.Nil(t, creds)
		// Underlying error must surface (wrapped), not be swallowed.
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to get secret reference")
	})

	t.Run("fails when the secret reference is nil", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*occlient.SecretReferenceInfo, error) {
				//nolint:nilnil // intentionally exercising the (nil, nil) input the service must handle
				return nil, nil
			},
		}
		svc := newGitCredsService(t, oc)

		creds, err := svc.GetGitCredentials(context.Background(), org, "git-secret")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "secret reference not found or has no data")
	})

	t.Run("fails when the secret reference has no data sources", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*occlient.SecretReferenceInfo, error) {
				return &occlient.SecretReferenceInfo{Name: "git-secret", Data: nil}, nil
			},
		}
		svc := newGitCredsService(t, oc)

		creds, err := svc.GetGitCredentials(context.Background(), org, "git-secret")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "secret reference not found or has no data")
	})

	t.Run("fails when the first data source has an empty KV path", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*occlient.SecretReferenceInfo, error) {
				return &occlient.SecretReferenceInfo{
					Name: "git-secret",
					Data: []occlient.SecretDataSourceInfo{
						{RemoteRef: occlient.RemoteRefInfo{Key: ""}},
					},
				}, nil
			},
		}
		svc := newGitCredsService(t, oc)

		creds, err := svc.GetGitCredentials(context.Background(), org, "git-secret")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "secret reference has no KV path")
	})
}

// -----------------------------------------------------------------------------
// convertToKV2ReadPath — pure helper, no dependencies. Verifies the KV v1 -> v2
// read-path rewrite that determines where the Vault read happens.
// -----------------------------------------------------------------------------

func TestConvertToKV2ReadPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "inserts /data/ after the mount for a nested path",
			in:   "secret/default/git/private",
			want: "secret/data/default/git/private",
		},
		{
			name: "inserts /data/ for a single-segment path",
			in:   "secret/private",
			want: "secret/data/private",
		},
		{
			name: "returns a path with no separator unchanged",
			in:   "secret",
			want: "secret",
		},
		{
			name: "returns empty input unchanged",
			in:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, convertToKV2ReadPath(tc.in))
		})
	}
}
