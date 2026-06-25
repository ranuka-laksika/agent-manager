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

package cmdutil

import (
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func TestLowestEnvironment(t *testing.T) {
	cases := []struct {
		name  string
		paths []amsvc.PromotionPath
		want  string
	}{
		{
			name: "linear dev->staging->prod, dev is entry",
			paths: []amsvc.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "staging"}}},
				{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name:  "empty pipeline",
			paths: nil,
			want:  "",
		},
		{
			name: "single path dev->prod",
			paths: []amsvc.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name: "every source is also a target (cycle) -> empty",
			paths: []amsvc.PromotionPath{
				{SourceEnvironmentRef: "a", TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "b"}}},
				{SourceEnvironmentRef: "b", TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "a"}}},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LowestEnvironment(tc.paths)
			if got != tc.want {
				t.Errorf("LowestEnvironment = %q, want %q", got, tc.want)
			}
		})
	}
}
