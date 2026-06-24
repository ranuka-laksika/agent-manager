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

// UNIT tests for repositoryService. No `//go:build integration` tag, so these
// run in the fast unit tier (`make test-unit`) and must NOT touch the network.
//
// repositoryService's public methods (ListBranches/ListCommits/GetLatestCommit)
// ultimately call a real git provider (HTTP to GitHub). We therefore cannot
// drive their happy paths in a unit test. What we CAN exercise without any
// network is everything that runs BEFORE the provider call:
//
//   - getGitProviderConfigWithCredentials: a pure helper (nil/invalid/valid creds).
//   - credential resolution: when the request carries a secretRef + orgName the
//     service calls GitCredentialsService.GetGitCredentials; we assert that a
//     fetch error and an invalid-credentials result are propagated verbatim,
//     before any provider/network work happens.
//   - provider construction: an unsupported ProviderType makes NewProvider fail
//     locally (no network), so we can assert that error surfaces.
//
// GitCredentialsService has no generated mock, so we hand-write a func-field stub
// (gitCredsStub) following the same pattern the generated moq mocks use: an
// unset func field would panic, making an unexpected code path fail loudly.
//
// Parts that genuinely need a live git remote (the successful branch/commit
// listing and SHA transformation, and GetLatestCommit end-to-end) are left to
// integration-level tests and are noted inline below.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/gitprovider"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// gitCredsStub is a hand-written stub for the in-package GitCredentialsService
// interface (no moq mock exists for it). A nil GetGitCredentialsFunc panics, so
// a test that reaches this method unexpectedly fails loudly.
type gitCredsStub struct {
	GetGitCredentialsFunc func(ctx context.Context, orgName, secretRef string) (*GitCredentials, error)
}

func (s *gitCredsStub) GetGitCredentials(ctx context.Context, orgName, secretRef string) (*GitCredentials, error) {
	return s.GetGitCredentialsFunc(ctx, orgName, secretRef)
}

// newRepoService wires repositoryService with the credential stub and a discard
// logger (discardLogger lives in evaluator_manager_unit_test.go in this package).
func newRepoService(creds GitCredentialsService) RepositoryService {
	return NewRepositoryService(creds, discardLogger())
}

// branchReq builds a ListBranchesRequest carrying a secretRef + orgName, which is
// what triggers the credential-resolution branch.
func branchReq(secretRef, orgName string) spec.ListBranchesRequest {
	return spec.ListBranchesRequest{
		Owner:      "acme",
		Repository: "widgets",
		SecretRef:  strPtr(secretRef),
		OrgName:    strPtr(orgName),
	}
}

// commitReq mirrors branchReq for ListCommits.
func commitReq(secretRef, orgName string) spec.ListCommitsRequest {
	return spec.ListCommitsRequest{
		Owner:     "acme",
		Repo:      "widgets",
		SecretRef: strPtr(secretRef),
		OrgName:   strPtr(orgName),
	}
}

// -----------------------------------------------------------------------------
// getGitProviderConfigWithCredentials — pure helper, no dependencies. Covers the
// validation gate that decides whether supplied credentials are usable.
// -----------------------------------------------------------------------------

func TestRepositoryService_getGitProviderConfigWithCredentials(t *testing.T) {
	t.Run("nil credentials -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(nil)
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("non basic-auth type -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "ssh", Password: "pw"})
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("basic-auth with empty password -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "basic-auth", Password: ""})
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("valid basic-auth -> token wired into provider config", func(t *testing.T) {
		cfg, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "basic-auth", Password: "ghp_secret"})
		require.NoError(t, err)
		assert.Equal(t, "ghp_secret", cfg.Token)
	})
}

// -----------------------------------------------------------------------------
// ListBranches — pre-network branches: credential fetch failure, invalid creds,
// and unsupported provider type. The successful listing path requires a live
// git remote and is covered by integration tests.
// -----------------------------------------------------------------------------

func TestRepositoryService_ListBranches(t *testing.T) {
	t.Run("propagates credential-fetch error", func(t *testing.T) {
		boom := errors.New("openbao unreachable")
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return nil, boom
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListBranches(context.Background(), branchReq("git-secret", "acme"), gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		// A real fetch error must not be masked as "not found".
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("propagates invalid-credentials error before any provider work", func(t *testing.T) {
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				// Wrong type makes getGitProviderConfigWithCredentials reject it.
				return &GitCredentials{Type: "ssh"}, nil
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListBranches(context.Background(), branchReq("git-secret", "acme"), gitprovider.ProviderGitHub, 10, 0)

		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("unsupported provider type fails locally without network", func(t *testing.T) {
		// No secretRef/orgName => credential branch is skipped (stub func left nil
		// to assert it is never called). NewProvider rejects the unknown type
		// before any HTTP call is made.
		svc := newRepoService(&gitCredsStub{})

		req := spec.ListBranchesRequest{Owner: "acme", Repository: "widgets"}
		_, err := svc.ListBranches(context.Background(), req, gitprovider.ProviderType("bitbucket"), 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported git provider")
	})
}

// -----------------------------------------------------------------------------
// ListCommits — same pre-network branches as ListBranches. The commit-to-spec
// transformation (short SHA truncation, author avatar mapping) needs a live
// remote to feed it data and is covered by integration tests.
// -----------------------------------------------------------------------------

func TestRepositoryService_ListCommits(t *testing.T) {
	t.Run("propagates credential-fetch error", func(t *testing.T) {
		boom := errors.New("openbao unreachable")
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return nil, boom
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListCommits(context.Background(), commitReq("git-secret", "acme"), gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		// A real fetch error must not be masked as "not found".
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("propagates invalid-credentials error before any provider work", func(t *testing.T) {
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return &GitCredentials{Type: "basic-auth", Password: ""}, nil
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListCommits(context.Background(), commitReq("git-secret", "acme"), gitprovider.ProviderGitHub, 10, 0)

		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("unsupported provider type fails locally without network", func(t *testing.T) {
		svc := newRepoService(&gitCredsStub{})

		req := spec.ListCommitsRequest{Owner: "acme", Repo: "widgets"}
		_, err := svc.ListCommits(context.Background(), req, gitprovider.ProviderType("gitlab"), 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported git provider")
	})
}

// NOTE: GetLatestCommit is intentionally not unit-tested. It hard-codes
// gitprovider.ProviderGitHub and immediately calls provider.ListCommits, which
// performs a real HTTP request to GitHub — there is no pre-network branch to
// exercise without a live remote (or an injectable provider). Its behaviour
// (latest-SHA extraction and the empty-result -> gitprovider.ErrNotFound mapping)
// belongs in integration tests.
