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

package thundersvc

import (
	"context"
	"errors"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOpenBaoReader is a hand-written test double for the narrow openBaoReader
// interface (mirrors the Func-field mock style used elsewhere in this codebase).
type fakeOpenBaoReader struct {
	ReadWithContextFunc func(ctx context.Context, path string) (*vault.Secret, error)
}

func (f *fakeOpenBaoReader) ReadWithContext(ctx context.Context, path string) (*vault.Secret, error) {
	return f.ReadWithContextFunc(ctx, path)
}

// fakeResolveBaseURL stands in for real network probing in tests that don't care
// about base-URL resolution itself — it always reports a fake base URL reachable
// with no dial override.
func fakeResolveBaseURL(_ context.Context, _, _ string) (string, string, bool) {
	return "http://fake-thunder:8090", "", true
}

func TestEnvThunderResolver_Resolve_Success(t *testing.T) {
	var capturedPath string
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, p string) (*vault.Secret, error) {
			capturedPath = p
			return &vault.Secret{
				Data: map[string]any{
					"data": map[string]any{
						"client-secret": "the-system-client-secret",
					},
				},
			}, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "secret/data/thunder-system-clients/acme/staging", capturedPath)
}

func TestEnvThunderResolver_Resolve_Caches(t *testing.T) {
	calls := 0
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			calls++
			return &vault.Secret{
				Data: map[string]any{"data": map[string]any{"client-secret": "s3cr3t"}},
			}, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	c1, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	c2, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)

	assert.Same(t, c1, c2, "a resolved client for the same org/env must be cached, not rebuilt")
	assert.Equal(t, 1, calls, "the OpenBao secret must only be fetched once per org/env")
}

func TestEnvThunderResolver_Resolve_DifferentEnvironmentsAreNotCachedTogether(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return &vault.Secret{Data: map[string]any{"data": map[string]any{"client-secret": "s3cr3t"}}}, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	staging, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	prod, err := resolver.Resolve(context.Background(), "acme", "prod")
	require.NoError(t, err)

	assert.NotSame(t, staging, prod)
}

func TestEnvThunderResolver_Resolve_NotProvisioned_NilSecret(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return nil, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "no-such-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_NotProvisioned_MissingSecretKey(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return &vault.Secret{Data: map[string]any{"data": map[string]any{}}}, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "half-provisioned-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_OpenBaoErrorPropagates(t *testing.T) {
	boom := errors.New("connection refused")
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return nil, boom
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "a real OpenBao error must not be mistaken for not-provisioned")
}

func TestEnvThunderResolver_Resolve_UsesResolvedBaseURLAndDialOverride(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return &vault.Secret{Data: map[string]any{"data": map[string]any{"client-secret": "s3cr3t"}}}, nil
		},
	}
	var gotOrg, gotEnv string
	resolveBaseURL := func(_ context.Context, org, env string) (string, string, bool) {
		gotOrg, gotEnv = org, env
		return "http://acme-staging.thunder.amp.localhost:8080", "host.docker.internal:8080", true
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", resolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)

	assert.Equal(t, "acme", gotOrg)
	assert.Equal(t, "staging", gotEnv)

	tc, ok := client.(*thunderClient)
	require.True(t, ok)
	assert.Equal(t, "http://acme-staging.thunder.amp.localhost:8080", tc.baseURL)
	require.NotNil(t, tc.httpClient.Transport, "a non-empty dial override must install a custom transport")
}

func TestEnvThunderResolver_Resolve_ThunderUnreachable(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return &vault.Secret{Data: map[string]any{"data": map[string]any{"client-secret": "s3cr3t"}}}, nil
		},
	}
	resolveBaseURL := func(_ context.Context, _, _ string) (string, string, bool) {
		return "", "", false
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", resolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThunderUnreachable))
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "unreachable-but-provisioned must not be treated as never-provisioned (that classifies as a permanent failure upstream, unreachable must be retried)")
}
