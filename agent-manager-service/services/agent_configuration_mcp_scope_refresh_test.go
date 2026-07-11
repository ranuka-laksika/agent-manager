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

package services

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefreshTouchedMCPEnvironments_CallsInjectForEveryTouchedEnv(t *testing.T) {
	var called []string
	svc := &agentConfigurationService{
		agentIdentityInjection: &agentIdentityInjectorStub{
			InjectForEnvironmentFunc: func(_ context.Context, orgName, projectName, agentName, envName string) error {
				assert.Equal(t, "acme", orgName)
				assert.Equal(t, "proj1", projectName)
				assert.Equal(t, "my-agent", agentName)
				called = append(called, envName)
				return nil
			},
		},
		logger: discardLogger(),
	}

	svc.refreshTouchedMCPEnvironments(context.Background(), "acme", "proj1", "my-agent",
		map[string]struct{}{"dev": {}, "staging": {}, "production": {}})

	sort.Strings(called)
	assert.Equal(t, []string{"dev", "production", "staging"}, called)
}

func TestRefreshTouchedMCPEnvironments_EmptySet_NoCalls(t *testing.T) {
	svc := &agentConfigurationService{
		agentIdentityInjection: &agentIdentityInjectorStub{
			InjectForEnvironmentFunc: func(context.Context, string, string, string, string) error {
				t.Fatal("InjectForEnvironment must not be called when no environments were touched")
				return nil
			},
		},
		logger: discardLogger(),
	}

	svc.refreshTouchedMCPEnvironments(context.Background(), "acme", "proj1", "my-agent", map[string]struct{}{})
}

func TestRefreshTouchedMCPEnvironments_FailureIsSwallowed(t *testing.T) {
	svc := &agentConfigurationService{
		agentIdentityInjection: &agentIdentityInjectorStub{
			InjectForEnvironmentFunc: func(context.Context, string, string, string, string) error {
				return errors.New("release binding update failed")
			},
		},
		logger: discardLogger(),
	}

	// Must not panic and must not return anything the caller could mistake for
	// an error — refreshTouchedMCPEnvironments has no return value precisely
	// because a refresh failure must never turn a successful MCP config update
	// into an error response.
	assert.NotPanics(t, func() {
		svc.refreshTouchedMCPEnvironments(context.Background(), "acme", "proj1", "my-agent", map[string]struct{}{"dev": {}})
	})
}
