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

package catalog_test

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/catalog"
)

func TestApplyProviderPrefix(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		providerPrefix string
		hasPrefix      bool
		want           string
	}{
		{
			// Regression for issue #951: a vendor namespace ("meta/") is part of
			// the model id and must be preserved, not truncated.
			name:           "vendor namespace preserved (NVIDIA via OpenAI template)",
			model:          "meta/llama-3.3-70b-instruct",
			providerPrefix: "openai",
			hasPrefix:      true,
			want:           "openai/meta/llama-3.3-70b-instruct",
		},
		{
			name:           "bare model gets provider prefix",
			model:          "gpt-4",
			providerPrefix: "openai",
			hasPrefix:      true,
			want:           "openai/gpt-4",
		},
		{
			// Unknown gateway template: no prefix is applied and the model
			// (including any vendor namespace) is passed through untouched.
			name:           "no prefix leaves model untouched",
			model:          "meta/llama-3.3-70b-instruct",
			providerPrefix: "",
			hasPrefix:      false,
			want:           "meta/llama-3.3-70b-instruct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := catalog.ApplyProviderPrefix(tt.model, tt.providerPrefix, tt.hasPrefix)
			if got != tt.want {
				t.Errorf("ApplyProviderPrefix(%q, %q, %v) = %q, want %q",
					tt.model, tt.providerPrefix, tt.hasPrefix, got, tt.want)
			}
		})
	}
}
