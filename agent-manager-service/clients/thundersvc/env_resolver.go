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
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

// envThunderClientCacheTTL bounds how long a resolved ThunderClient is reused
// before its system-client secret is re-read from the secret store. Without
// this, a rotated env-Thunder secret (e.g. a re-bootstrap) would break
// provisioning for that environment until the AMS process restarts.
const envThunderClientCacheTTL = 15 * time.Minute

// ErrThunderNotProvisioned is returned when no env-Thunder system-client secret
// exists for the given organization/environment — i.e. add-environment-thunder.sh
// has not been run for it yet.
var ErrThunderNotProvisioned = errors.New("env-thunder not provisioned for this environment")

// ErrThunderUnreachable is returned when an env-Thunder's system-client secret
// exists (it has been provisioned) but no candidate base URL responds — unlike
// ErrThunderNotProvisioned, this is expected to be transient (e.g. a cold-starting
// pod, or a momentary network blip) and should be retried, not treated as a
// permanent failure.
var ErrThunderUnreachable = errors.New("env-thunder is provisioned but not reachable")

// EnvThunderResolver resolves a ready-to-use ThunderClient for a specific
// environment's Thunder instance, given only an organization and environment name.
// Used by both agent ownership models identically — every AgentID is provisioned
// against a specific environment's Thunder instance (see the AgentID architecture
// doc, Section 3).
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/env_thunder_resolver_fake.go . EnvThunderResolver:EnvThunderResolverMock
type EnvThunderResolver interface {
	Resolve(ctx context.Context, orgName, envName string) (ThunderClient, error)
	// ResolveIdentity returns the same resolved client widened to identity
	// operations (the concrete client implements both interfaces).
	ResolveIdentity(ctx context.Context, orgName, envName string) (EnvIdentityClient, error)
}

// EnvIdentityClient is the env-Thunder surface the agent-identity passthrough
// needs: full identity management plus default-OU lookup.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/env_identity_client_mock.go . EnvIdentityClient:EnvIdentityClientMock
type EnvIdentityClient interface {
	IdentityClient
	GetDefaultOUID(ctx context.Context) (string, error)
}

// envThunderSecretLocation returns the deterministic, backend-agnostic
// location of one environment's env-Thunder system-client secret: an
// org-level secret (no project/agent) keyed by a composite entity name so it
// round-trips unambiguously through SecretLocation.KVPath()/ParseKVPath() —
// unlike a bare {org, env, entity} triple, which would collide in shape with
// the {org, entity, key} triple already used elsewhere (e.g.
// monitorCompositeSecretLocation). add-environment-thunder.sh's
// write_to_openbao() writes to this exact path.
func envThunderSecretLocation(orgName, envName string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    orgName,
		EntityName: "thunder-system-client-" + envName,
	}
}

// envThunderResolver reads the env-Thunder system-client secret written by
// add-environment-thunder.sh's write_to_openbao(), via the same pluggable
// secretmanagersvc.SecretManagementClient every other secret-backed service in
// this codebase uses — this is infrastructure bootstrap state, not a
// user-facing org/project secret, but it lives in the same swappable backend.
// The client ID is not stored there: every env-Thunder uses the same well-known
// system client ID created by the Thunder bootstrap job (thunderSystemClientID).
// resolveBaseURLFunc picks a reachable base URL (and, if needed, a dial-override
// address) for an env-Thunder instance. Matches ResolveThunderBaseURL's signature —
// injectable so tests don't depend on real network probing.
type resolveBaseURLFunc func(ctx context.Context, org, env string) (baseURL, resolveToHost string, ok bool)

type envThunderResolver struct {
	secretMgmtClient secretmanagersvc.SecretManagementClient
	resolveBaseURL   resolveBaseURLFunc
	ttl              time.Duration
	now              func() time.Time

	mu    sync.RWMutex
	cache map[string]cachedThunderClient // keyed by "org/env"
	sfg   singleflight.Group             // dedupes concurrent cache-miss resolves per key
}

type cachedThunderClient struct {
	client   ThunderClient
	cachedAt time.Time
}

// NewEnvThunderResolver creates an EnvThunderResolver backed by the given
// secret management client — the same shared, deployment-pluggable client
// used for AgentID/LLM/MCP/publisher secrets, so swapping the backend (e.g.
// away from OpenBao) needs no change here.
func NewEnvThunderResolver(secretMgmtClient secretmanagersvc.SecretManagementClient) EnvThunderResolver {
	return newEnvThunderResolverWithSecretClient(secretMgmtClient, ResolveThunderBaseURL)
}

// newEnvThunderResolverWithSecretClient builds a resolver against an injected
// secret client — the real shared one, or a fake in tests — and an injected
// base-URL resolver, the real network-probing ResolveThunderBaseURL, or a fake.
func newEnvThunderResolverWithSecretClient(secretMgmtClient secretmanagersvc.SecretManagementClient, resolveBaseURL resolveBaseURLFunc) *envThunderResolver {
	return &envThunderResolver{
		secretMgmtClient: secretMgmtClient,
		resolveBaseURL:   resolveBaseURL,
		ttl:              envThunderClientCacheTTL,
		now:              time.Now,
		cache:            make(map[string]cachedThunderClient),
	}
}

// Resolve returns a ThunderClient authenticated against the given environment's
// Thunder instance. Resolved clients are cached per (org, env) for
// envThunderClientCacheTTL: the underlying ThunderClient already caches its
// own system token, so caching the client itself avoids re-reading the secret
// store on every call, while the TTL still picks up a rotated system-client
// secret without requiring a process restart.
func (r *envThunderResolver) Resolve(ctx context.Context, orgName, envName string) (ThunderClient, error) {
	// Reject path-breaking segments before they ever reach SecretLocation.KVPath().
	// envName is embedded inside a composite entity name (never a bare path
	// segment on its own), but orgName is always the KV path's first segment,
	// so it's still worth failing fast and explicitly here rather than relying
	// on the secret backend to reject it.
	for _, seg := range []string{orgName, envName} {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "/") {
			return nil, fmt.Errorf("invalid org or environment name segment %q", seg)
		}
	}

	cacheKey := orgName + "/" + envName

	r.mu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && r.now().Sub(entry.cachedAt) < r.ttl {
		r.mu.RUnlock()
		return entry.client, nil
	}
	r.mu.RUnlock()

	// Singleflight so concurrent first-time resolves for the same key share one
	// secret-store read and base-URL probe instead of each paying the cost independently.
	result, err, _ := r.sfg.Do(cacheKey, func() (any, error) {
		r.mu.RLock()
		if entry, ok := r.cache[cacheKey]; ok && r.now().Sub(entry.cachedAt) < r.ttl {
			r.mu.RUnlock()
			return entry.client, nil
		}
		r.mu.RUnlock()

		kvPath, err := envThunderSecretLocation(orgName, envName).KVPath()
		if err != nil {
			return nil, fmt.Errorf("build env-thunder secret location for %s/%s: %w", orgName, envName, err)
		}
		data, err := r.secretMgmtClient.GetSecretWithValue(ctx, kvPath)
		if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
			return nil, ErrThunderNotProvisioned
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read env-thunder system-client secret for %s/%s: %w", orgName, envName, err)
		}
		clientSecret := data[thunderSystemClientSecretKey]
		if clientSecret == "" {
			return nil, ErrThunderNotProvisioned
		}

		baseURL, resolveToHost, ok := r.resolveBaseURL(ctx, orgName, envName)
		if !ok {
			return nil, fmt.Errorf("%w: %s/%s", ErrThunderUnreachable, orgName, envName)
		}
		client := newThunderClientWithDialOverride(baseURL, thunderSystemClientID, clientSecret, resolveToHost)

		r.mu.Lock()
		r.cache[cacheKey] = cachedThunderClient{client: client, cachedAt: r.now()}
		r.mu.Unlock()

		return client, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(ThunderClient), nil
}

// ResolveIdentity resolves the env-Thunder client and widens it to the identity
// surface. The concrete client returned by Resolve implements both interfaces.
func (r *envThunderResolver) ResolveIdentity(ctx context.Context, orgName, envName string) (EnvIdentityClient, error) {
	c, err := r.Resolve(ctx, orgName, envName)
	if err != nil {
		return nil, err
	}
	ic, ok := c.(EnvIdentityClient)
	if !ok {
		return nil, fmt.Errorf("resolved thunder client for %s/%s does not support identity operations", orgName, envName)
	}
	return ic, nil
}
