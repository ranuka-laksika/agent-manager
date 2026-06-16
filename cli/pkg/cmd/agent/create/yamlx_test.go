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

package create

import (
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

// DecodeStrict must honour json tags (the generated types have no yaml tags),
// so camelCase YAML keys land in the right fields.
func TestDecodeStrict_JSONTags(t *testing.T) {
	yaml := `
name: my-agent
displayName: My Agent
provisioning:
  type: internal
  repository:
    url: https://github.com/example/repo
    branch: main
    appPath: /app
`
	var req amsvc.CreateAgentRequest
	if err := DecodeStrict([]byte(yaml), &req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.DisplayName != "My Agent" {
		t.Errorf("DisplayName = %q, want %q (yaml must map via json tags)", req.DisplayName, "My Agent")
	}
	if req.Provisioning.Repository == nil || req.Provisioning.Repository.AppPath != "/app" {
		t.Errorf("Repository = %+v, want appPath=/app", req.Provisioning.Repository)
	}
}

func TestDecodeStrict_UnknownFieldRejected(t *testing.T) {
	yaml := `
name: my-agent
displayNam: typo
`
	var req amsvc.CreateAgentRequest
	err := DecodeStrict([]byte(yaml), &req)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "displayNam") {
		t.Errorf("error %q should name the unknown field", err)
	}
}

func TestDecodeStrict_NestedUnknownFieldRejected(t *testing.T) {
	yaml := `
name: my-agent
provisioning:
  type: internal
  repository:
    url: https://github.com/example/repo
    branche: typo
`
	var req amsvc.CreateAgentRequest
	if err := DecodeStrict([]byte(yaml), &req); err == nil {
		t.Fatal("expected error for nested unknown field")
	}
}

func TestDecodeStrict_InvalidYAML(t *testing.T) {
	var req amsvc.CreateAgentRequest
	if err := DecodeStrict([]byte("a: [unclosed"), &req); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}
