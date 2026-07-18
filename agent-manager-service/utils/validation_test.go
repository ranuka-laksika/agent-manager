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
	"errors"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestValidatePromoteAgentRequest_UseSourceExcludesInstrumentationVersion(t *testing.T) {
	useSource := true
	req := &spec.PromoteAgentRequest{
		SourceEnvironment:      "dev",
		TargetEnvironment:      "staging",
		UseConfigFromSourceEnv: &useSource,
	}
	req.InstrumentationVersion.Set(strPtrForTest("0.4.0"))

	err := ValidatePromoteAgentRequest(req)
	if err == nil {
		t.Fatal("expected error: instrumentationVersion is mutually exclusive with useConfigFromSourceEnv=true")
	}
	if !strings.Contains(err.Error(), "instrumentationVersion") {
		t.Errorf("error %q should mention instrumentationVersion", err)
	}
}

func TestValidatePromoteAgentRequest_InstrumentationVersionAllowedWithoutUseSource(t *testing.T) {
	req := &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	}
	req.InstrumentationVersion.Set(strPtrForTest("0.4.0"))

	if err := ValidatePromoteAgentRequest(req); err != nil {
		t.Errorf("instrumentationVersion should be allowed when useConfigFromSourceEnv is unset: %v", err)
	}
}

func strPtrForTest(s string) *string { return &s }

func TestValidateTemplateHandle(t *testing.T) {
	tests := []struct {
		name    string
		handle  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid handle with alphanumeric",
			handle:  "openai",
			wantErr: false,
		},
		{
			name:    "valid handle with hyphens",
			handle:  "azure-openai",
			wantErr: false,
		},
		{
			name:    "valid handle with underscores",
			handle:  "aws_bedrock",
			wantErr: false,
		},
		{
			name:    "valid handle with mixed characters",
			handle:  "my-template_v1",
			wantErr: false,
		},
		{
			name:    "valid handle with numbers",
			handle:  "template123",
			wantErr: false,
		},
		{
			name:    "empty handle",
			handle:  "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "handle too long",
			handle:  strings.Repeat("a", 256),
			wantErr: true,
			errMsg:  "must not exceed 255 characters",
		},
		{
			name:    "handle with spaces",
			handle:  "my template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with special characters",
			handle:  "template@123",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with dots",
			handle:  "my.template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with forward slash",
			handle:  "my/template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with backslash",
			handle:  "my\\template",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle with null byte",
			handle:  "template\x00",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "handle at max length",
			handle:  strings.Repeat("a", 255),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateHandle(tt.handle)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateTemplateHandle() expected error but got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateTemplateHandle() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateTemplateHandle() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateResourceRequestsWithinLimits(t *testing.T) {
	tests := []struct {
		name      string
		requested spec.ResourceConfig
		current   *spec.ResourceConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "requests-only update exceeding the current limits is rejected",
			requested: spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("500m"), Memory: strPtrForTest("512Mi")},
			},
			current: &spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
			},
			wantErr: true,
			errMsg:  "must be less than or equal to",
		},
		{
			name: "requests and limits raised together is allowed",
			requested: spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("500m"), Memory: strPtrForTest("512Mi")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("500m"), Memory: strPtrForTest("512Mi")},
			},
			current: &spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
			},
			wantErr: false,
		},
		{
			name: "limits-only update lowering below current requests is rejected",
			requested: spec.ResourceConfig{
				Limits: &spec.ResourceLimits{Cpu: strPtrForTest("50m"), Memory: strPtrForTest("128Mi")},
			},
			current: &spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("500m"), Memory: strPtrForTest("512Mi")},
			},
			wantErr: true,
			errMsg:  "must be less than or equal to",
		},
		{
			name: "cpu-only update leaves memory untouched and valid",
			requested: spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("200m")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("200m")},
			},
			current: &spec.ResourceConfig{
				Requests: &spec.ResourceRequests{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
				Limits:   &spec.ResourceLimits{Cpu: strPtrForTest("100m"), Memory: strPtrForTest("256Mi")},
			},
			wantErr: false,
		},
		{
			name:      "no current config and no limits in request skips the check",
			requested: spec.ResourceConfig{Requests: &spec.ResourceRequests{Cpu: strPtrForTest("500m"), Memory: strPtrForTest("512Mi")}},
			current:   nil,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceRequestsWithinLimits(tt.requested, tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ValidateResourceRequestsWithinLimits() expected error but got nil")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("ValidateResourceRequestsWithinLimits() error should wrap ErrInvalidInput, got %v", err)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateResourceRequestsWithinLimits() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateResourceRequestsWithinLimits() unexpected error = %v", err)
			}
		})
	}
}
