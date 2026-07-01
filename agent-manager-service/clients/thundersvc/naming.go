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
	"crypto/sha256"
	"fmt"
	"strings"
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
func ThunderReleaseName(org, env string) string {
	org = slugify(org)
	env = slugify(env)
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
// Reachable only inside the cluster (internal K8s DNS).
func ThunderTokenURL(org, env string) string {
	return ThunderInternalURL(org, env) + "/oauth2/token"
}

// ThunderHost returns the wildcard-cert-friendly hostname under thunder.amp.localhost for the env-Thunder instance.
// Capped at 63 characters for the DNS label limit.
func ThunderHost(org, env string) string {
	org = slugify(org)
	env = slugify(env)
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
