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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// fakeSecretManagementClient is a hand-written test double for
// secretmanagersvc.SecretManagementClient (mirrors the Func-field mock style
// used elsewhere in this codebase). Not the moq-generated
// clientmocks.SecretManagementClientMock: clientmocks imports this package
// (for EnvThunderResolverMock/EnvIdentityClientMock), so importing it back
// from an in-package thundersvc test would be an import cycle.
type fakeSecretManagementClient struct {
	GetSecretWithValueFunc func(ctx context.Context, kvPath string) (map[string]string, error)
}

func (f *fakeSecretManagementClient) CreateSecret(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
	panic("fakeSecretManagementClient.CreateSecret: not used by these tests")
}

func (f *fakeSecretManagementClient) PatchSecret(context.Context, secretmanagersvc.SecretLocation, map[string]string, []string) (string, error) {
	panic("fakeSecretManagementClient.PatchSecret: not used by these tests")
}

func (f *fakeSecretManagementClient) DeleteSecret(context.Context, secretmanagersvc.SecretLocation, string) error {
	panic("fakeSecretManagementClient.DeleteSecret: not used by these tests")
}

func (f *fakeSecretManagementClient) GetSecret(context.Context, string) (*secretmanagersvc.SecretInfo, error) {
	panic("fakeSecretManagementClient.GetSecret: not used by these tests")
}

func (f *fakeSecretManagementClient) GetSecretWithValue(ctx context.Context, kvPath string) (map[string]string, error) {
	return f.GetSecretWithValueFunc(ctx, kvPath)
}

// fakeResolveBaseURL stands in for real network probing in tests that don't care
// about base-URL resolution itself — it always reports a fake base URL reachable
// with no dial override.
func fakeResolveBaseURL(_ context.Context, _, _ string) (string, string, bool) {
	return "http://fake-thunder:8090", "", true
}

func TestEnvThunderResolver_Resolve_Success(t *testing.T) {
	var capturedPath string
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(_ context.Context, kvPath string) (map[string]string, error) {
			capturedPath = kvPath
			return map[string]string{"client-secret": "the-system-client-secret"}, nil
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "acme/thunder-system-client-staging", capturedPath)
}

// TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments guards the org
// segment (always the KV path's first, bare segment) and the env segment
// (embedded inside a composite entity name, but still explicitly rejected
// here for a clear, fast error instead of silently building a nonsensical
// location and letting the secret backend reject it).
func TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments(t *testing.T) {
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			t.Fatal("must not read the secret store when a segment is invalid")
			return nil, nil //nolint:nilnil // unreachable — t.Fatal above halts the test
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

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
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			calls++
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	c1, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	c2, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)

	assert.Same(t, c1, c2, "a resolved client for the same org/env must be cached, not rebuilt")
	assert.Equal(t, 1, calls, "the secret must only be fetched once per org/env")
}

// TestEnvThunderResolver_Resolve_ExpiresAfterTTL guards against a cached
// ThunderClient (and the system-client secret baked into it) surviving
// forever: if that secret is ever rotated (e.g. a re-bootstrap), every call
// against a never-expiring cache would keep authenticating with the stale
// secret until the AMS process restarts.
func TestEnvThunderResolver_Resolve_ExpiresAfterTTL(t *testing.T) {
	calls := 0
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			calls++
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)
	now := time.Now()
	resolver.now = func() time.Time { return now }
	resolver.ttl = time.Minute

	c1, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)

	now = now.Add(30 * time.Second)
	c2, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	assert.Same(t, c1, c2, "still within TTL, must not rebuild")
	assert.Equal(t, 1, calls)

	now = now.Add(time.Minute)
	c3, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	assert.NotSame(t, c1, c3, "past TTL, must re-read the secret store and rebuild so a rotated secret takes effect")
	assert.Equal(t, 2, calls)
}

// TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight guards
// against a thundering-herd on cold cache: many concurrent first-time Resolve calls
// for the same org/env must share one secret-store read and one base-URL probe, not
// each pay that cost independently.
func TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight(t *testing.T) {
	var readCalls, probeCalls int64
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			atomic.AddInt64(&readCalls, 1)
			time.Sleep(20 * time.Millisecond) // widen the race window
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	probeFn := func(_ context.Context, _, _ string) (string, string, bool) {
		atomic.AddInt64(&probeCalls, 1)
		return "http://fake-thunder:8090", "", true
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, probeFn)

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
	assert.EqualValues(t, 1, atomic.LoadInt64(&readCalls), "concurrent cache misses for the same key must share one secret-store read")
	assert.EqualValues(t, 1, atomic.LoadInt64(&probeCalls), "concurrent cache misses for the same key must share one base-URL probe")
	for i := 1; i < goroutines; i++ {
		assert.Same(t, clients[0], clients[i], "all concurrent resolvers for the same key must get the identical client")
	}
}

func TestEnvThunderResolver_Resolve_DifferentEnvironmentsAreNotCachedTogether(t *testing.T) {
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	staging, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	prod, err := resolver.Resolve(context.Background(), "acme", "prod")
	require.NoError(t, err)

	assert.NotSame(t, staging, prod)
}

func TestEnvThunderResolver_Resolve_NotProvisioned_SecretNotFound(t *testing.T) {
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return nil, secretmanagersvc.ErrSecretNotFound
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "no-such-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_NotProvisioned_MissingSecretKey(t *testing.T) {
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{}, nil
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "half-provisioned-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_SecretStoreErrorPropagates(t *testing.T) {
	boom := errors.New("connection refused")
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return nil, boom
		},
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "a real secret-store error must not be mistaken for not-provisioned")
}

func TestEnvThunderResolver_Resolve_UsesResolvedBaseURLAndDialOverride(t *testing.T) {
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	var gotOrg, gotEnv string
	resolveBaseURL := func(_ context.Context, org, env string) (string, string, bool) {
		gotOrg, gotEnv = org, env
		return "http://acme-staging.thunder.amp.localhost:8080", "host.docker.internal:8080", true
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, resolveBaseURL)

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
	secretClient := &fakeSecretManagementClient{
		GetSecretWithValueFunc: func(context.Context, string) (map[string]string, error) {
			return map[string]string{"client-secret": "s3cr3t"}, nil
		},
	}
	resolveBaseURL := func(_ context.Context, _, _ string) (string, string, bool) {
		return "", "", false
	}
	resolver := newEnvThunderResolverWithSecretClient(secretClient, resolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThunderUnreachable))
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "unreachable-but-provisioned must not be treated as never-provisioned (that classifies as a permanent failure upstream, unreachable must be retried)")
}
