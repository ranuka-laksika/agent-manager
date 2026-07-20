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
)

// okReader returns a fixed clientID/secret for any (org, env) — the common case
// for tests that don't care about the read itself.
func okReader(clientID, secret string) ReadSystemClientFunc {
	return func(context.Context, string, string) (string, string, error) {
		return clientID, secret, nil
	}
}

// fakeResolveBaseURL stands in for real network probing — always reports a
// fake, reachable base URL with no dial override.
func fakeResolveBaseURL(_ context.Context, _, _ string) (string, string, bool) {
	return "http://fake-thunder:8090", "", true
}

func TestEnvThunderResolver_Resolve_Success(t *testing.T) {
	var gotOrg, gotEnv string
	read := func(_ context.Context, org, env string) (string, string, error) {
		gotOrg, gotEnv = org, env
		return "amp-system-client", "the-system-client-secret", nil
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "acme", gotOrg)
	assert.Equal(t, "staging", gotEnv)
}

// TestEnvThunderResolver_Resolve_UsesStoredClientID confirms the resolver uses
// the client ID returned by the reader (not only the well-known constant).
func TestEnvThunderResolver_Resolve_UsesStoredClientID(t *testing.T) {
	resolver := newEnvThunderResolverWithReader(okReader("custom-client-id", "s3cr3t"), fakeResolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	tc, ok := client.(*thunderClient)
	require.True(t, ok)
	assert.Equal(t, "custom-client-id", tc.clientID)
}

// TestEnvThunderResolver_Resolve_FallsBackToConstantClientID confirms an empty
// stored client ID (rows predating the client_id column) falls back to thunderSystemClientID.
func TestEnvThunderResolver_Resolve_FallsBackToConstantClientID(t *testing.T) {
	resolver := newEnvThunderResolverWithReader(okReader("", "s3cr3t"), fakeResolveBaseURL)

	client, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	tc, ok := client.(*thunderClient)
	require.True(t, ok)
	assert.Equal(t, thunderSystemClientID, tc.clientID)
}

// TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments guards org/env
// with a clear, fast error rather than letting a nonsensical pair reach the reader.
func TestEnvThunderResolver_Resolve_RejectsPathBreakingSegments(t *testing.T) {
	read := func(context.Context, string, string) (string, string, error) {
		t.Fatal("must not read the store when a segment is invalid")
		return "", "", nil // unreachable — t.Fatal above halts the test
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)

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
	read := func(context.Context, string, string) (string, string, error) {
		calls++
		return "amp-system-client", "s3cr3t", nil
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)

	c1, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	c2, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)

	assert.Same(t, c1, c2, "a resolved client for the same org/env must be cached, not rebuilt")
	assert.Equal(t, 1, calls, "the secret must only be fetched once per org/env")
}

// TestEnvThunderResolver_Resolve_ExpiresAfterTTL guards against a cached client
// outliving a secret rotation — without the TTL, a rotated secret would break until restart.
func TestEnvThunderResolver_Resolve_ExpiresAfterTTL(t *testing.T) {
	calls := 0
	read := func(context.Context, string, string) (string, string, error) {
		calls++
		return "amp-system-client", "s3cr3t", nil
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)
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
	assert.NotSame(t, c1, c3, "past TTL, must re-read the store and rebuild so a rotated secret takes effect")
	assert.Equal(t, 2, calls)
}

// TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight guards
// against a thundering-herd: concurrent first-time resolves must share one read/probe.
func TestEnvThunderResolver_Resolve_ConcurrentCacheMiss_DedupesViaSingleflight(t *testing.T) {
	var readCalls, probeCalls int64
	read := func(context.Context, string, string) (string, string, error) {
		atomic.AddInt64(&readCalls, 1)
		time.Sleep(20 * time.Millisecond) // widen the race window
		return "amp-system-client", "s3cr3t", nil
	}
	probeFn := func(_ context.Context, _, _ string) (string, string, bool) {
		atomic.AddInt64(&probeCalls, 1)
		return "http://fake-thunder:8090", "", true
	}
	resolver := newEnvThunderResolverWithReader(read, probeFn)

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
	assert.EqualValues(t, 1, atomic.LoadInt64(&readCalls), "concurrent cache misses for the same key must share one credential read")
	assert.EqualValues(t, 1, atomic.LoadInt64(&probeCalls), "concurrent cache misses for the same key must share one base-URL probe")
	for i := 1; i < goroutines; i++ {
		assert.Same(t, clients[0], clients[i], "all concurrent resolvers for the same key must get the identical client")
	}
}

func TestEnvThunderResolver_Resolve_DifferentEnvironmentsAreNotCachedTogether(t *testing.T) {
	resolver := newEnvThunderResolverWithReader(okReader("amp-system-client", "s3cr3t"), fakeResolveBaseURL)

	staging, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.NoError(t, err)
	prod, err := resolver.Resolve(context.Background(), "acme", "prod")
	require.NoError(t, err)

	assert.NotSame(t, staging, prod)
}

func TestEnvThunderResolver_Resolve_NotProvisioned_NoRow(t *testing.T) {
	read := func(context.Context, string, string) (string, string, error) {
		return "", "", ErrThunderNotProvisioned
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "no-such-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_NotProvisioned_EmptySecret(t *testing.T) {
	resolver := newEnvThunderResolverWithReader(okReader("amp-system-client", ""), fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "half-provisioned-env")
	assert.True(t, errors.Is(err, ErrThunderNotProvisioned))
}

func TestEnvThunderResolver_Resolve_ReadErrorPropagates(t *testing.T) {
	boom := errors.New("decrypt failed")
	read := func(context.Context, string, string) (string, string, error) {
		return "", "", boom
	}
	resolver := newEnvThunderResolverWithReader(read, fakeResolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "a real read error must not be mistaken for not-provisioned")
}

func TestEnvThunderResolver_Resolve_UsesResolvedBaseURLAndDialOverride(t *testing.T) {
	var gotOrg, gotEnv string
	resolveBaseURL := func(_ context.Context, org, env string) (string, string, bool) {
		gotOrg, gotEnv = org, env
		return "http://acme-staging.thunder.amp.localhost:8080", "host.docker.internal:8080", true
	}
	resolver := newEnvThunderResolverWithReader(okReader("amp-system-client", "s3cr3t"), resolveBaseURL)

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
	resolveBaseURL := func(_ context.Context, _, _ string) (string, string, bool) {
		return "", "", false
	}
	resolver := newEnvThunderResolverWithReader(okReader("amp-system-client", "s3cr3t"), resolveBaseURL)

	_, err := resolver.Resolve(context.Background(), "acme", "staging")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThunderUnreachable))
	assert.False(t, errors.Is(err, ErrThunderNotProvisioned), "unreachable-but-provisioned must not be treated as never-provisioned (that classifies as a permanent failure upstream, unreachable must be retried)")
}
