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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// spyConfigService records the request passed to Create and stubs the LLM env var resolver.
// Only Create and BuildSystemManagedEnvVarsFromConfig are exercised; the embedded interface
// satisfies the rest (and panics if any other method is called).
type spyConfigService struct {
	AgentConfigurationService
	lastReq        models.CreateAgentModelConfigRequest
	systemEnvVars  []client.EnvVar
	systemEnvVarsE error
}

func (s *spyConfigService) Create(_ context.Context, _, _, _ string,
	req models.CreateAgentModelConfigRequest, _ string,
) (*models.AgentModelConfigResponse, error) {
	s.lastReq = req
	return &models.AgentModelConfigResponse{}, nil
}

func (s *spyConfigService) BuildSystemManagedEnvVarsFromConfig(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) {
	return s.systemEnvVars, s.systemEnvVarsE
}

func TestCreateAgentLLMConfigs_KeysUnderFirstEnv(t *testing.T) {
	spy := &spyConfigService{}
	s := &agentManagerService{agentConfigurationService: spy}

	req := &spec.CreateAgentRequest{
		Name:        "my-agent",
		ModelConfig: []spec.ModelConfigRequest{{ProviderName: "openai"}},
	}

	err := s.createAgentLLMConfigs(context.Background(), "org", "proj", "Development", req)
	require.NoError(t, err)

	require.Len(t, spy.lastReq.EnvMappings, 1, "exactly one env mapping")
	got, ok := spy.lastReq.EnvMappings["Development"]
	require.True(t, ok, "config must be keyed under firstEnv")
	require.Equal(t, "openai", got.ProviderName)
}

// TestMergeKindWorkloadLLMEnvVars_InjectsLLMEnvVars verifies that for a kind-sourced agent
// with an LLM configuration, the resolved system-managed LLM env vars are appended to the
// user-supplied env vars that get baked into the Workload CR. Regression test for the bug where
// LLM provider keys were written to the (unused) Component workflow params instead of the Workload.
func TestMergeKindWorkloadLLMEnvVars_InjectsLLMEnvVars(t *testing.T) {
	llmVars := []client.EnvVar{
		{Key: "OPENAI_BASE_URL", Value: "https://gw/openai"},
		{Key: "OPENAI_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{systemEnvVars: llmVars}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadLLMEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars, true)
	require.NoError(t, err)
	require.Equal(t, append(append([]client.EnvVar{}, userVars...), llmVars...), got,
		"user env vars must be preserved and LLM env vars appended")
}

// TestMergeKindWorkloadLLMEnvVars_NoModelConfig verifies that without an LLM configuration the
// resolver is not consulted and the user env vars pass through unchanged.
func TestMergeKindWorkloadLLMEnvVars_NoModelConfig(t *testing.T) {
	spy := &spyConfigService{systemEnvVarsE: errors.New("resolver must not be called")}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadLLMEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars, false)
	require.NoError(t, err)
	require.Equal(t, userVars, got)
}

// TestMergeKindWorkloadLLMEnvVars_ResolverError verifies the resolver error is propagated so the
// caller can roll back the partially-created agent rather than deploying without LLM keys.
func TestMergeKindWorkloadLLMEnvVars_ResolverError(t *testing.T) {
	spy := &spyConfigService{systemEnvVarsE: errors.New("boom")}
	s := &agentManagerService{agentConfigurationService: spy}

	_, err := s.mergeKindWorkloadLLMEnvVars(context.Background(), "my-agent", "org", "proj", "Development", nil, true)
	require.Error(t, err)
}
