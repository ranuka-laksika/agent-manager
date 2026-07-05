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
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestNewEnvThunderResolver_RejectsMissingConfig(t *testing.T) {
	_, err := NewEnvThunderResolver("http://openbao:8200", "", "secret")
	require.Error(t, err)
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

// TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments guards against a
// path.Join subtlety: it cleans its result, so a ".." segment silently escapes
// the "thunder-system-clients" prefix even though it contains no "/".
func TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments(t *testing.T) {
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(context.Context, string) (*vault.Secret, error) {
			t.Fatal("must not read OpenBao when a segment is invalid")
			return &vault.Secret{}, nil
		},
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", fakeResolveBaseURL)

	cases := []struct{ org, env string }{
		{"..", "staging"},
		{"acme", ".."},
		{".", "staging"},
		{"acme", ""},
		{"acme/evil", "staging"},
	}
	for _, tc := range cases {
		_, err := resolver.Resolve(context.Background(), tc.org, tc.env)
		require.Error(t, err)
	}
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

// TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight guards
// against a thundering-herd on cold cache: many concurrent first-time Resolve calls
// for the same org/env must share one OpenBao read and one base-URL probe, not each
// pay that cost independently.
func TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight(t *testing.T) {
	var readCalls, probeCalls int64
	reader := &fakeOpenBaoReader{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			atomic.AddInt64(&readCalls, 1)
			time.Sleep(20 * time.Millisecond) // widen the race window
			return &vault.Secret{
				Data: map[string]any{"data": map[string]any{"client-secret": "s3cr3t"}},
			}, nil
		},
	}
	probeFn := func(_ context.Context, _, _ string) (string, string, bool) {
		atomic.AddInt64(&probeCalls, 1)
		return "http://fake-thunder:8090", "", true
	}
	resolver := newEnvThunderResolverWithReader(reader, "secret", probeFn)

	const goroutines = 20
	clients := make([]ThunderClient, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			c, err := resolver.Resolve(context.Background(), "acme", "staging")
			errs[idx] = err
			clients[idx] = c
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}
	assert.EqualValues(t, 1, atomic.LoadInt64(&readCalls), "concurrent cache misses for the same key must share one OpenBao read")
	assert.EqualValues(t, 1, atomic.LoadInt64(&probeCalls), "concurrent cache misses for the same key must share one base-URL probe")
	for i := 1; i < goroutines; i++ {
		assert.Same(t, clients[0], clients[i], "all concurrent resolvers for the same key must get the identical client")
	}
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
			return nil, nil //nolint:nilnil // simulates OpenBao's real (nil, nil) response for a missing secret
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
