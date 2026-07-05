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
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// thunderSystemClientID is the OAuth2 client ID created by the Thunder bootstrap job.
	// Every env-Thunder uses this same ID — each instance has its own isolated DB.
	thunderSystemClientID = "amp-system-client"

	// thunderSystemClientSecretKey is the key within the K8s Secret that holds the
	// system client's OAuth2 secret.
	thunderSystemClientSecretKey = "client-secret"

	thunderInternalPort = 8090
	maxReleaseNameLen   = 53
	truncatePrefixLen   = 46
)

// ThunderReleaseName returns the Helm release name (and namespace) for an env-Thunder instance.
// Mirrors thunder_release_name() in add-environment-thunder.sh — must stay in sync.
//
// Format: amp-thunder-<org>-<env>, capped at 53 characters.
// If the natural name exceeds 53 chars, it is truncated to 46 characters (trailing "-" stripped)
// and a 6-char hex hash of "org/env" is appended for collision safety.
//
// Deliberately lowercases only — does NOT collapse consecutive hyphens like slugify() does.
// The bash scripts that actually provision Thunder (add-environment.sh, add-environment-thunder.sh)
// use org/env raw, with no hyphen-collapsing. Slugifying here would let this function compute a
// different address than what was actually deployed for any org/env containing "--".
func ThunderReleaseName(org, env string) string {
	org = strings.ToLower(org)
	env = strings.ToLower(env)
	if org == "" || env == "" {
		panic("org and env names must be valid alphanumeric slugs and not empty")
	}
	full := fmt.Sprintf("amp-thunder-%s-%s", org, env)
	if len(full) <= maxReleaseNameLen {
		return strings.TrimSuffix(full, "-")
	}
	hash := thunderSHA6(org + "/" + env)
	prefix := strings.TrimSuffix(full[:truncatePrefixLen], "-")
	return fmt.Sprintf("%s-%s", prefix, hash)
}

// ThunderNamespace returns the Kubernetes namespace for an env-Thunder instance.
// The namespace always mirrors the release name.
func ThunderNamespace(org, env string) string {
	return ThunderReleaseName(org, env)
}

// ThunderInternalURL returns the cluster-internal HTTP URL for an env-Thunder's admin API.
// AMS uses this URL to authenticate and create per-agent OAuth clients.
//
// The Thunder Helm chart creates a K8s Service named "{{ .Release.Name }}-service", so the
// cluster-internal DNS is: http://<release>-service.<namespace>.svc.cluster.local:8090
func ThunderInternalURL(org, env string) string {
	release := ThunderReleaseName(org, env)
	return fmt.Sprintf("http://%s-service.%s.svc.cluster.local:%d", release, release, thunderInternalPort)
}

// ThunderJWKSURL returns the cluster-internal URL for fetching the env-Thunder's JWKS.
// Used by the API gateway's ThunderKeyManager to validate agent tokens.
// INTERNAL ONLY — not reachable outside the cluster; use ThunderExternalJWKSURL for developer-facing output.
func ThunderJWKSURL(org, env string) string {
	return ThunderInternalURL(org, env) + "/oauth2/jwks"
}

// ThunderSystemClientSecretName returns the Kubernetes Secret name that holds the
// system-client credentials for the given env-Thunder instance.
// The secret is created by add-environment-thunder.sh and lives in ThunderNamespace.
func ThunderSystemClientSecretName(org, env string) string {
	return ThunderReleaseName(org, env) + "-system-client"
}

// ThunderTokenURL returns the OAuth2 token endpoint for the env-Thunder instance.
// INTERNAL ONLY — uses cluster-internal K8s DNS (svc.cluster.local:8090); not reachable
// outside the cluster. Use ThunderExternalTokenURL for developer-facing output.
func ThunderTokenURL(org, env string) string {
	return ThunderInternalURL(org, env) + "/oauth2/token"
}

// ThunderExternalTokenURL returns the public OAuth2 token endpoint for the env-Thunder instance.
// Reachable from outside the cluster via the HTTPRoute that maps
// http://<org>-<env>.thunder.amp.localhost:8080 -> the Thunder service.
// Use this in developer-facing API responses (console copy-buttons, Identity page, etc.).
func ThunderExternalTokenURL(org, env string) string {
	return fmt.Sprintf("http://%s:8080/oauth2/token", ThunderHost(org, env))
}

// ThunderExternalJWKSURL returns the public JWKS endpoint for the env-Thunder instance.
// Reachable from outside the cluster via the same HTTPRoute as ThunderExternalTokenURL.
// Use this in developer-facing API responses.
func ThunderExternalJWKSURL(org, env string) string {
	return fmt.Sprintf("http://%s:8080/oauth2/jwks", ThunderHost(org, env))
}

// ThunderHost returns the wildcard-cert-friendly hostname under thunder.amp.localhost for the env-Thunder instance.
// Capped at 63 characters for the DNS label limit.
//
// Deliberately lowercases only — see ThunderReleaseName for why slugify()'s hyphen-collapsing
// is not applied here (must match the un-collapsed bash implementation byte-for-byte).
func ThunderHost(org, env string) string {
	org = strings.ToLower(org)
	env = strings.ToLower(env)
	if org == "" || env == "" {
		panic("org and env names must be valid alphanumeric slugs and not empty")
	}
	label := fmt.Sprintf("%s-%s", org, env)
	if len(label) <= 63 {
		return fmt.Sprintf("%s.thunder.amp.localhost", strings.TrimSuffix(label, "-"))
	}
	hash := thunderSHA6(org + "/" + env)
	prefix := strings.TrimSuffix(label[:56], "-")
	return fmt.Sprintf("%s-%s.thunder.amp.localhost", prefix, hash)
}

// ThunderIssuerURL returns the public issuer URL for the env-Thunder instance.
// This is what Thunder stamps into the JWT iss claim.
func ThunderIssuerURL(org, env string) string {
	return fmt.Sprintf("http://%s:8080", ThunderHost(org, env))
}

// thunderURLCandidate pairs a candidate base URL for an env-Thunder instance with
// the actual host:port a caller should connect to in order to reach it. resolveToHost
// is empty when the base URL's own host is directly dialable; when set, callers must
// dial resolveToHost while keeping the base URL's host as the HTTP Host header, so
// Kgateway's host-based routing still selects the right backend.
type thunderURLCandidate struct {
	baseURL       string
	resolveToHost string
}

// thunderBaseURLCandidates returns, in preference order, every base URL an env-Thunder
// instance might be reachable at: cluster-internal DNS (production/in-cluster callers),
// the public ingress hostname (development on the host machine), and two Docker-container
// fallbacks (Docker Desktop's host.docker.internal, then plain Linux host networking)
// for callers running inside a container that can't resolve *.thunder.amp.localhost itself.
func thunderBaseURLCandidates(org, env string) []thunderURLCandidate {
	host := ThunderHost(org, env)
	externalBaseURL := fmt.Sprintf("http://%s:8080", host)
	return []thunderURLCandidate{
		{baseURL: ThunderInternalURL(org, env)},
		{baseURL: externalBaseURL},
		{baseURL: externalBaseURL, resolveToHost: "host.docker.internal:8080"},
		{baseURL: externalBaseURL, resolveToHost: "127.0.0.1:8080"},
	}
}

// probeThunderURL reports whether a GET to url succeeds, optionally dialing
// resolveToHost instead of the URL's own host (see thunderURLCandidate).
func probeThunderURL(ctx context.Context, url, resolveToHost string) bool {
	const probeTimeout = 2 * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if resolveToHost != "" {
		req.Host = req.URL.Host
		req.URL.Host = resolveToHost
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// thunderURLProber checks whether a candidate base URL is reachable — its JWKS
// endpoint is used as the health check since it requires no authentication.
// Injectable so callers can test the candidate-selection cascade without real
// network access.
type thunderURLProber func(ctx context.Context, candidate thunderURLCandidate) bool

// defaultThunderURLProber is the real, network-probing implementation used outside tests.
func defaultThunderURLProber(ctx context.Context, candidate thunderURLCandidate) bool {
	return probeThunderURL(ctx, candidate.baseURL+"/oauth2/jwks", candidate.resolveToHost)
}

// resolveThunderBaseURL returns the first candidate base URL that prober reports as
// reachable, trying them in thunderBaseURLCandidates' preference order. ok is false
// if none respond.
func resolveThunderBaseURL(ctx context.Context, org, env string, prober thunderURLProber) (candidate thunderURLCandidate, ok bool) {
	for _, c := range thunderBaseURLCandidates(org, env) {
		if prober(ctx, c) {
			return c, true
		}
	}
	return thunderURLCandidate{}, false
}

// ResolveThunderBaseURL returns the first reachable base URL for an env-Thunder
// instance, trying cluster-internal DNS, the external ingress hostname, then
// Docker Desktop/Linux host-networking fallbacks — the same cascade ThunderProbe
// uses. Callers that build an HTTP client against the result must dial
// resolveToHost (when non-empty) instead of the base URL's own host, while still
// sending the base URL's host as the Host header.
func ResolveThunderBaseURL(ctx context.Context, org, env string) (baseURL, resolveToHost string, ok bool) {
	c, ok := resolveThunderBaseURL(ctx, org, env, defaultThunderURLProber)
	return c.baseURL, c.resolveToHost, ok
}

// ThunderProbe checks whether an env-Thunder instance is reachable by trying the
// same candidate cascade as ResolveThunderBaseURL. Callers treat a negative probe
// as "not provisioned" and skip the env.
func ThunderProbe(ctx context.Context, org, env string) bool {
	_, ok := resolveThunderBaseURL(ctx, org, env, defaultThunderURLProber)
	return ok
}

const maxAgentAppNameLen = 100

// AgentThunderAppName returns the OAuth app name to use in Thunder for a per-agent client.
// Format: amp-agent-<org>-<env>-<project>-<agent>, truncated to 100 chars, trailing
// hyphen stripped. The name mirrors amp-publisher-<org> but is fully scoped to
// env + project + agent to avoid collisions.
//
// env is included even though each env already has its own physically separate
// Thunder instance (so it isn't needed for uniqueness there): without it, every
// env-Thunder's agent list looks identical — e.g. "amp-agent-default-default-x" in
// both the "stage" and "testing" instances — with nothing in the name itself
// showing which environment you're looking at from inside Thunder's own console.
func AgentThunderAppName(org, env, project, agent string) string {
	org = slugify(org)
	env = slugify(env)
	project = slugify(project)
	agent = slugify(agent)
	if org == "" || env == "" || project == "" || agent == "" {
		panic("org, env, project, and agent names must be valid alphanumeric slugs and not empty")
	}
	name := fmt.Sprintf("amp-agent-%s-%s-%s-%s", org, env, project, agent)
	if len(name) <= maxAgentAppNameLen {
		return strings.TrimSuffix(name, "-")
	}
	return strings.TrimRight(name[:maxAgentAppNameLen], "-")
}

// thunderSHA6 returns the first 6 hex characters of the SHA-256 hash of s.
// Produces the same output as _sha6() in add-environment-thunder.sh.
func thunderSHA6(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:])[:6]
}

// slugify converts string to lowercase, replaces invalid characters (spaces, underscores)
// with hyphens, merges consecutive hyphens, and trims leading/trailing hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else if r == '-' || r == '_' || r == ' ' {
			sb.WriteRune('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return strings.Trim(res, "-")
}
