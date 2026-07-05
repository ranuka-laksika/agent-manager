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
	"strings"
	"testing"
	"time"
)

// These cases lock in that ThunderReleaseName/ThunderHost do NOT collapse consecutive
// hyphens, matching the bash implementations in add-environment.sh and
// add-environment-thunder.sh exactly. Both scripts validate ENV_NAME against
// ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ (which permits internal "--") and use org/env raw —
// they never slugify/collapse. If this Go code collapsed "--" to "-" (as slugify() does),
// it would compute a different release name / hostname than what actually gets deployed
// whenever an org or env name contains a double hyphen, causing AMS's own admin-API calls
// to Thunder (ThunderInternalURL/ThunderTokenURL, used for per-agent client provisioning)
// to target an address that doesn't exist.
func TestThunderReleaseName_NoHyphenCollapsing(t *testing.T) {
	got := ThunderReleaseName("my--org", "env")
	want := "amp-thunder-my--org-env"
	if got != want {
		t.Errorf("ThunderReleaseName must not collapse consecutive hyphens: want %q, got %q", want, got)
	}
}

func TestThunderHost_NoHyphenCollapsing(t *testing.T) {
	got := ThunderHost("my--org", "env")
	want := "my--org-env.thunder.amp.localhost"
	if got != want {
		t.Errorf("ThunderHost must not collapse consecutive hyphens: want %q, got %q", want, got)
	}
}

func TestThunderReleaseName_Lowercases(t *testing.T) {
	got := ThunderReleaseName("MyOrg", "Staging")
	want := "amp-thunder-myorg-staging"
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestThunderReleaseName_BasicCases(t *testing.T) {
	cases := []struct {
		org, env, want string
	}{
		{"default", "default", "amp-thunder-default-default"},
		{"my-org", "staging", "amp-thunder-my-org-staging"},
	}
	for _, tc := range cases {
		got := ThunderReleaseName(tc.org, tc.env)
		if got != tc.want {
			t.Errorf("ThunderReleaseName(%q, %q): want %q, got %q", tc.org, tc.env, tc.want, got)
		}
	}
}

func TestThunderHost_BasicCases(t *testing.T) {
	cases := []struct {
		org, env, want string
	}{
		{"default", "default", "default-default.thunder.amp.localhost"},
		{"my-org", "staging", "my-org-staging.thunder.amp.localhost"},
	}
	for _, tc := range cases {
		got := ThunderHost(tc.org, tc.env)
		if got != tc.want {
			t.Errorf("ThunderHost(%q, %q): want %q, got %q", tc.org, tc.env, tc.want, got)
		}
	}
}

// TestAgentThunderAppName_IncludesEnv locks in that the Thunder app name embeds
// the environment name — without it, every env-Thunder's own agent list looks
// identical (e.g. "amp-agent-default-default-x" in both "stage" and "testing"),
// with nothing in the name itself showing which environment an operator browsing
// Thunder's console directly is actually looking at.
func TestAgentThunderAppName_IncludesEnv(t *testing.T) {
	cases := []struct {
		org, env, project, agent, want string
	}{
		{"default", "stage", "default", "my-agent", "amp-agent-default-stage-default-my-agent"},
		{"acme", "production", "proj1", "agent-1", "amp-agent-acme-production-proj1-agent-1"},
	}
	for _, tc := range cases {
		got := AgentThunderAppName(tc.org, tc.env, tc.project, tc.agent)
		if got != tc.want {
			t.Errorf("AgentThunderAppName(%q, %q, %q, %q): want %q, got %q", tc.org, tc.env, tc.project, tc.agent, tc.want, got)
		}
	}
}

func TestAgentThunderAppName_DifferentEnvsProduceDifferentNames(t *testing.T) {
	stage := AgentThunderAppName("default", "stage", "default", "my-agent")
	testing_ := AgentThunderAppName("default", "testing", "default", "my-agent")
	if stage == testing_ {
		t.Errorf("expected different environments to produce different app names, both were %q", stage)
	}
}

func TestAgentThunderAppName_TruncatesAt100Chars(t *testing.T) {
	got := AgentThunderAppName(
		strings.Repeat("o", 40), strings.Repeat("e", 40), strings.Repeat("p", 40), strings.Repeat("a", 40))
	if len(got) > 100 {
		t.Errorf("expected app name truncated to 100 chars, got %d chars: %q", len(got), got)
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("expected no trailing hyphen after truncation, got %q", got)
	}
}

// These lock in resolveThunderBaseURL's candidate cascade — the mechanism that lets
// AMS reach env-Thunder both when running in-cluster (production) and when running
// via docker-compose outside the cluster (local dev), where *.svc.cluster.local
// cannot be resolved at all. A fake prober stands in for real network probing so
// the cascade order and short-circuiting are tested deterministically.

func TestResolveThunderBaseURL_PrefersClusterInternal(t *testing.T) {
	var probed []thunderURLCandidate
	prober := func(_ context.Context, c thunderURLCandidate) bool {
		probed = append(probed, c)
		return c.baseURL == ThunderInternalURL("acme", "staging")
	}

	got, ok := resolveThunderBaseURL(context.Background(), "acme", "staging", prober)
	if !ok {
		t.Fatal("expected ok=true when the cluster-internal candidate is reachable")
	}
	if got.baseURL != ThunderInternalURL("acme", "staging") {
		t.Errorf("want cluster-internal base URL, got %q", got.baseURL)
	}
	if got.resolveToHost != "" {
		t.Errorf("cluster-internal candidate must not set resolveToHost, got %q", got.resolveToHost)
	}
	if len(probed) != 1 {
		t.Errorf("must stop at the first reachable candidate, probed %d", len(probed))
	}
}

func TestResolveThunderBaseURL_FallsBackToExternalIngress(t *testing.T) {
	externalBaseURL := "http://acme-staging.thunder.amp.localhost:8080"
	prober := func(_ context.Context, c thunderURLCandidate) bool {
		return c.baseURL == externalBaseURL && c.resolveToHost == ""
	}

	got, ok := resolveThunderBaseURL(context.Background(), "acme", "staging", prober)
	if !ok {
		t.Fatal("expected ok=true when the external ingress candidate is reachable")
	}
	if got.baseURL != externalBaseURL || got.resolveToHost != "" {
		t.Errorf("want external ingress candidate with no dial override, got %+v", got)
	}
}

func TestResolveThunderBaseURL_FallsBackToDockerDesktop(t *testing.T) {
	prober := func(_ context.Context, c thunderURLCandidate) bool {
		return c.resolveToHost == "host.docker.internal:8080"
	}

	got, ok := resolveThunderBaseURL(context.Background(), "acme", "staging", prober)
	if !ok {
		t.Fatal("expected ok=true when only the host.docker.internal candidate is reachable")
	}
	if got.resolveToHost != "host.docker.internal:8080" {
		t.Errorf("want host.docker.internal dial override, got %+v", got)
	}
}

func TestResolveThunderBaseURL_FallsBackToLoopback(t *testing.T) {
	prober := func(_ context.Context, c thunderURLCandidate) bool {
		return c.resolveToHost == "127.0.0.1:8080"
	}

	got, ok := resolveThunderBaseURL(context.Background(), "acme", "staging", prober)
	if !ok {
		t.Fatal("expected ok=true when only the 127.0.0.1 candidate is reachable")
	}
	if got.resolveToHost != "127.0.0.1:8080" {
		t.Errorf("want 127.0.0.1 dial override, got %+v", got)
	}
}

func TestResolveThunderBaseURL_AllUnreachable(t *testing.T) {
	prober := func(_ context.Context, _ thunderURLCandidate) bool { return false }

	_, ok := resolveThunderBaseURL(context.Background(), "acme", "staging", prober)
	if ok {
		t.Error("expected ok=false when no candidate is reachable")
	}
}

func TestResolveThunderBaseURL_PublicWrapperUsesRealCascadeShape(t *testing.T) {
	// ResolveThunderBaseURL can't be probed against real network in a unit test,
	// but it must at least be wired to the real candidate cascade for a
	// definitely-unreachable org/env, rather than e.g. always returning ok=true.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, ok := ResolveThunderBaseURL(ctx, "nonexistent-org-xyz", "nonexistent-env-xyz")
	if ok {
		t.Error("expected ok=false for an org/env with no env-Thunder deployed anywhere reachable")
	}
}
