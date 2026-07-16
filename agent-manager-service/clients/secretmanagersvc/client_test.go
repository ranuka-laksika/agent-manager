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

package secretmanagersvc

import "testing"

func TestSecretLocation_KVPath_RejectsPathTraversalSegments(t *testing.T) {
	fieldSetters := map[string]func(v string) SecretLocation{
		"OrgName": func(v string) SecretLocation { return SecretLocation{OrgName: v, EntityName: "entity"} },
		"ProjectName": func(v string) SecretLocation {
			return SecretLocation{OrgName: "org", EntityName: "entity", ProjectName: v}
		},
		"EnvironmentName": func(v string) SecretLocation {
			return SecretLocation{OrgName: "org", EntityName: "entity", EnvironmentName: v}
		},
		"AgentName": func(v string) SecretLocation {
			return SecretLocation{OrgName: "org", EntityName: "entity", AgentName: v}
		},
		"ConfigName": func(v string) SecretLocation {
			return SecretLocation{OrgName: "org", EntityName: "entity", ConfigName: v}
		},
		"EntityName": func(v string) SecretLocation { return SecretLocation{OrgName: "org", EntityName: v} },
		"SecretKey": func(v string) SecretLocation {
			return SecretLocation{OrgName: "org", EntityName: "entity", SecretKey: v}
		},
	}

	for _, traversal := range []string{"..", ".", " .. ", " . "} {
		for field, makeLocation := range fieldSetters {
			t.Run(field+"="+traversal, func(t *testing.T) {
				if _, err := makeLocation(traversal).KVPath(); err == nil {
					t.Fatalf("KVPath() with %s=%q: expected a rejection, got nil error", field, traversal)
				}
			})
		}
	}
}

func TestSecretLocation_KVPath_RejectsSlashInAnySegment(t *testing.T) {
	if _, err := (SecretLocation{OrgName: "org", EntityName: "a/b"}).KVPath(); err == nil {
		t.Fatal("KVPath() with a '/' embedded in EntityName: expected a rejection, got nil error")
	}
}

func TestSecretLocation_KVPath_BuildsExpectedPathForOrdinarySegments(t *testing.T) {
	loc := SecretLocation{
		OrgName: "acme", ProjectName: "proj1", EnvironmentName: "dev",
		AgentName: "agent1", EntityName: "agent1-agent-identity",
	}
	got, err := loc.KVPath()
	if err != nil {
		t.Fatalf("KVPath() unexpected error: %v", err)
	}
	want := "acme/proj1/dev/agent1/agent1-agent-identity"
	if got != want {
		t.Fatalf("KVPath() = %q, want %q", got, want)
	}
}
