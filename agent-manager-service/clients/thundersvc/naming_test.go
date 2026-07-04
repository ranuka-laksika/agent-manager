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
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// TestThunderHost_RespectsBaseDomainConfig locks in that a VM deployment's
// THUNDER_HOST_BASE_DOMAIN override flows straight through into every URL builder —
// this is what makes deployments/vm/lib-vm.sh setting the same value (both for
// add-environment-thunder.sh and this Go config) keep the reported URLs and the
// actually-deployed Thunder instance's own issuer in sync.
func TestThunderHost_RespectsBaseDomainConfig(t *testing.T) {
	orig := config.GetConfig().ThunderHostBaseDomain
	defer func() { config.GetConfig().ThunderHostBaseDomain = orig }()

	config.GetConfig().ThunderHostBaseDomain = "amp.203.0.113.10.sslip.io"
	got := ThunderHost("default", "staging")
	want := "default-staging.thunder.amp.203.0.113.10.sslip.io"
	if got != want {
		t.Errorf("ThunderHost with overridden base domain: want %q, got %q", want, got)
	}
}

// TestThunderExternalURLs_RespectTLSConfig locks in that TLS_ENABLED (the same flag
// deployments/vm/lib-vm.sh already sets for platform Thunder's own advertised URLs)
// switches env-Thunder's reported scheme AND drops the :8080 suffix — matching Caddy
// terminating on the standard HTTPS port on a VM, rather than the k3d gateway's
// plain-HTTP :8080 in local dev.
func TestThunderExternalURLs_RespectTLSConfig(t *testing.T) {
	origDomain := config.GetConfig().ThunderHostBaseDomain
	origTLS := config.GetConfig().TLSConfig.EnableTLS
	defer func() {
		config.GetConfig().ThunderHostBaseDomain = origDomain
		config.GetConfig().TLSConfig.EnableTLS = origTLS
	}()
	config.GetConfig().ThunderHostBaseDomain = "amp.203.0.113.10.sslip.io"

	config.GetConfig().TLSConfig.EnableTLS = false
	if got, want := ThunderIssuerURL("default", "staging"), "http://default-staging.thunder.amp.203.0.113.10.sslip.io:8080"; got != want {
		t.Errorf("ThunderIssuerURL (TLS off): want %q, got %q", want, got)
	}
	if got, want := ThunderExternalJWKSURL("default", "staging"), "http://default-staging.thunder.amp.203.0.113.10.sslip.io:8080/oauth2/jwks"; got != want {
		t.Errorf("ThunderExternalJWKSURL (TLS off): want %q, got %q", want, got)
	}

	config.GetConfig().TLSConfig.EnableTLS = true
	if got, want := ThunderIssuerURL("default", "staging"), "https://default-staging.thunder.amp.203.0.113.10.sslip.io"; got != want {
		t.Errorf("ThunderIssuerURL (TLS on): want %q, got %q", want, got)
	}
	if got, want := ThunderExternalTokenURL("default", "staging"), "https://default-staging.thunder.amp.203.0.113.10.sslip.io/oauth2/token"; got != want {
		t.Errorf("ThunderExternalTokenURL (TLS on): want %q, got %q", want, got)
	}
}

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

// isValidJWKS is ThunderProbe's correctness check on top of an HTTP 200 — these cases
// lock in that a bare 200 with an unrelated body (e.g. from a stray server on a
// probed fallback address) is NOT treated as a live env-Thunder.
func TestIsValidJWKS(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"real JWKS with one key", `{"keys":[{"kty":"RSA","kid":"abc","use":"sig"}]}`, true},
		{"real JWKS with multiple keys", `{"keys":[{"kty":"RSA"},{"kty":"EC"}]}`, true},
		{"empty keys array", `{"keys":[]}`, false},
		{"missing keys field", `{"foo":"bar"}`, false},
		{"not JSON", `<html>404 not found</html>`, false},
		{"empty body", ``, false},
		{"keys is not an array", `{"keys":"not-an-array"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidJWKS([]byte(tc.body))
			if got != tc.want {
				t.Errorf("isValidJWKS(%q): want %v, got %v", tc.body, tc.want, got)
			}
		})
	}
}
