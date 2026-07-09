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

package utils

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestConvertToConfigurations_SurfacesInstrumentationVersion(t *testing.T) {
	version := "0.3.0"
	got := convertToConfigurations(&models.Configurations{
		InstrumentationVersion: &version,
	})
	if got == nil {
		t.Fatal("expected non-nil configurations")
	}
	// The read path must surface the pinned version, otherwise the deploy/promote
	// UI falls back to the platform default and shows a version different from
	// what is actually deployed.
	if !got.InstrumentationVersion.IsSet() {
		t.Fatal("InstrumentationVersion should be set on the response")
	}
	if v := got.InstrumentationVersion.Get(); v == nil || *v != "0.3.0" {
		t.Errorf("InstrumentationVersion = %v, want 0.3.0", v)
	}
}

func TestConvertToConfigurations_UnpinnedIsOmitted(t *testing.T) {
	got := convertToConfigurations(&models.Configurations{})
	if got == nil {
		t.Fatal("expected non-nil configurations")
	}
	if got.InstrumentationVersion.IsSet() {
		t.Errorf("InstrumentationVersion should be unset when the agent has no pin")
	}
}
