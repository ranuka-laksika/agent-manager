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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
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

	// maxJWKSProbeBodyBytes caps how much of a probe response ThunderProbe reads
	// before validating it as JWKS — a real JWKS document is a few KB at most; this
	// just guards against an unrelated server on a fallback address streaming an
	// unbounded body.
	maxJWKSProbeBodyBytes = 64 * 1024
)

// ThunderReleaseName returns the Helm release name (and namespace) for an env-Thunder instance.
// Mirrors thunder_release_name() in deployments/scripts/thunder-naming.sh — must stay in sync.
// That file is the single source of truth for the bash side of this derivation (sourced by
// every script that provisions, removes, or wires the gateway to env-Thunder); this function
// is the one Go-side copy, since Go can't source bash.
//
// Format: amp-thunder-<org>-<env>, capped at 53 characters.
// If the natural name exceeds 53 chars, it is truncated to 46 characters (trailing "-" stripped)
// and a 6-char hex hash of "org/env" is appended for collision safety.
//
// Deliberately lowercases only — does NOT collapse consecutive hyphens like slugify() does.
// The bash scripts that actually provision Thunder use org/env raw, with no hyphen-collapsing.
// Slugifying here would let this function compute a different address than what was actually
// deployed for any org/env containing "--".
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
// Reachable from outside the cluster via the HTTPRoute that maps ThunderHost -> the
// Thunder service (locally through the k3d gateway; on a VM through the Caddy
// wildcard site — see deployments/vm/lib-vm.sh).
// Use this in developer-facing API responses (console copy-buttons, Identity page, etc.).
func ThunderExternalTokenURL(org, env string) string {
	return thunderExternalOrigin(org, env) + "/oauth2/token"
}

// ThunderExternalJWKSURL returns the public JWKS endpoint for the env-Thunder instance.
// Reachable from outside the cluster via the same route as ThunderExternalTokenURL.
// Use this in developer-facing API responses.
func ThunderExternalJWKSURL(org, env string) string {
	return thunderExternalOrigin(org, env) + "/oauth2/jwks"
}

// ThunderHost returns the wildcard-cert-friendly hostname
// "<org>-<env>.thunder.<config.ThunderHostBaseDomain>" for the env-Thunder instance,
// capped at 63 characters for the DNS label limit. ThunderHostBaseDomain defaults to
// "amp.localhost" (local dev, k3d's *.amp.localhost wildcard) and is overridden
// deployment-wide (never per call — see deployments/scripts/thunder-naming.sh's
// THUNDER_HOST_BASE_DOMAIN, which must be set to the identical value everywhere
// env-Thunder is provisioned on a given deployment, so this function's output always
// matches what was actually deployed).
//
// Deliberately lowercases only — see ThunderReleaseName for why slugify()'s hyphen-collapsing
// is not applied here (must match the un-collapsed bash implementation byte-for-byte).
func ThunderHost(org, env string) string {
	org = strings.ToLower(org)
	env = strings.ToLower(env)
	if org == "" || env == "" {
		panic("org and env names must be valid alphanumeric slugs and not empty")
	}
	baseDomain := config.GetConfig().ThunderHostBaseDomain
	label := fmt.Sprintf("%s-%s", org, env)
	if len(label) <= 63 {
		return fmt.Sprintf("%s.thunder.%s", strings.TrimSuffix(label, "-"), baseDomain)
	}
	hash := thunderSHA6(org + "/" + env)
	prefix := strings.TrimSuffix(label[:56], "-")
	return fmt.Sprintf("%s-%s.thunder.%s", prefix, hash, baseDomain)
}

// thunderExternalOrigin returns "<scheme>://<ThunderHost>[:8080]" — the externally
// reachable origin for an env-Thunder instance. Local dev (config.TLSConfig.EnableTLS
// == false, the default) reaches env-Thunder directly on the k3d gateway's plain-HTTP
// port 8080. VM/production deployments (TLS_ENABLED=true — the same flag
// deployments/vm/lib-vm.sh already sets for platform Thunder's own advertised URLs)
// front env-Thunder with Caddy on the standard HTTPS port instead, so no port is
// appended: deployments/scripts/thunder-naming.sh's thunder_issuer() must produce the
// SAME scheme/port shape when TLS_ENABLED=true, or the URL reported here won't match
// what env-Thunder itself self-configured as its issuer/publicUrl.
func thunderExternalOrigin(org, env string) string {
	if config.GetConfig().TLSConfig.EnableTLS {
		return fmt.Sprintf("https://%s", ThunderHost(org, env))
	}
	return fmt.Sprintf("http://%s:8080", ThunderHost(org, env))
}

// ThunderIssuerURL returns the public issuer URL for the env-Thunder instance.
// This is what Thunder stamps into the JWT iss claim.
func ThunderIssuerURL(org, env string) string {
	return thunderExternalOrigin(org, env)
}

// isValidJWKS reports whether body is a syntactically valid JWKS document (a JSON
// object with a non-empty "keys" array). An HTTP 200 alone is not sufficient evidence
// of a live Thunder instance — any unrelated server answering on a probed
// address/port (e.g. a stray dev server bound to the same fallback address) would
// otherwise be misreported as a reachable env-Thunder.
func isValidJWKS(body []byte) bool {
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false
	}
	return len(doc.Keys) > 0
}

// probeAttempt is one candidate (URL, Host-header override) pair ThunderProbe tries.
type probeAttempt struct {
	url           string
	resolveToHost string
}

// ThunderProbe checks whether an env-Thunder instance is reachable by performing a
// HTTP GET to its JWKS endpoint and validating the response is actually a JWKS
// document. Two network topologies are always tried — the cluster-internal URL
// (the real address in any Kubernetes deployment) and the public URL. Two more,
// host-header-resolved fallbacks targeting fixed local addresses
// (host.docker.internal / 127.0.0.1), are tried ONLY when config.IsLocalDevEnv is
// set: they exist purely to compensate for agent-manager-service running as a plain
// Docker container (docker-compose) rather than a Kubernetes pod in local dev, where
// neither of the two real probes can resolve. They must never run outside that
// context — in-cluster, the cluster-internal probe always succeeds when Thunder is
// actually reachable, and probing 127.0.0.1 there would hit agent-manager-service's
// own pod rather than Thunder, which is both pointless and a latent false-positive
// risk if anything ever answers on that port.
//
// All attempts run concurrently (rather than tried one after another) so latency is
// bounded by a single probe's timeout, not the sum of however many are tried.
// Callers treat a negative probe as "not provisioned" and skip the env.
//
// A 2-second per-probe timeout avoids blocking the ListThunderInstances hot path on
// unreachable instances while still being generous enough for a slow cold start.
func ThunderProbe(ctx context.Context, org, env string) bool {
	const probeTimeout = 2 * time.Second
	probe := func(ctx context.Context, url string, resolveToHost string) bool {
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
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSProbeBodyBytes))
		if err != nil {
			return false
		}
		return isValidJWKS(body)
	}

	attempts := []probeAttempt{
		{ThunderJWKSURL(org, env), ""},         // 1. cluster-internal (the real address in any k8s deployment)
		{ThunderExternalJWKSURL(org, env), ""}, // 2. public URL
	}
	if config.GetConfig().IsLocalDevEnv {
		host := ThunderHost(org, env)
		attempts = append(attempts,
			probeAttempt{"http://" + host + ":8080/oauth2/jwks", "host.docker.internal:8080"}, // 3. local dev: agent-manager-service as a Docker container
			probeAttempt{"http://" + host + ":8080/oauth2/jwks", "127.0.0.1:8080"},            // 4. local dev: host networking
		)
	}

	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan bool, len(attempts))
	for _, a := range attempts {
		go func(url, resolveToHost string) {
			results <- probe(probeCtx, url, resolveToHost)
		}(a.url, a.resolveToHost)
	}

	for range attempts {
		if <-results {
			return true
		}
	}
	return false
}

const maxAgentAppNameLen = 100

// AgentThunderAppName returns the OAuth app name to use in Thunder for a per-agent client.
// Format: amp-agent-<org>-<project>-<agent>, truncated to 100 chars, trailing hyphen stripped.
// The name mirrors amp-publisher-<org> but is fully scoped to project + agent to avoid collisions.
func AgentThunderAppName(org, project, agent string) string {
	org = slugify(org)
	project = slugify(project)
	agent = slugify(agent)
	if org == "" || project == "" || agent == "" {
		panic("org, project, and agent names must be valid alphanumeric slugs and not empty")
	}
	name := fmt.Sprintf("amp-agent-%s-%s-%s", org, project, agent)
	if len(name) <= maxAgentAppNameLen {
		return strings.TrimSuffix(name, "-")
	}
	return strings.TrimRight(name[:maxAgentAppNameLen], "-")
}

// thunderSHA6 returns the first 6 hex characters of the SHA-256 hash of s.
// Produces the same output as _sha6() in deployments/scripts/thunder-naming.sh.
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
