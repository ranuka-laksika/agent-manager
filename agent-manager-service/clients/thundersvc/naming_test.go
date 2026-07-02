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

import "testing"

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
