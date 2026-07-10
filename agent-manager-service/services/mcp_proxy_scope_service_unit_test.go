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
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// scopeTestProxy builds an endpoint-era proxy: capabilities live on the endpoint's
// MCPEndpointConfig (migration 032), not on the parent proxy config.
func scopeTestProxy(handle string, tools ...string) *models.MCPProxy {
	endpoint := models.MCPProxyEndpoint{UUID: uuid.New(), Handle: "primary"}
	if len(tools) > 0 {
		toolMaps := make([]map[string]interface{}, 0, len(tools))
		for _, tl := range tools {
			toolMaps = append(toolMaps, map[string]interface{}{"name": tl})
		}
		endpoint.Configuration = models.MCPEndpointConfig{
			Capabilities: &models.MCPProxyCapabilities{Tools: &toolMaps},
		}
	}
	return &models.MCPProxy{
		UUID:      uuid.New(),
		Artifact:  &models.Artifact{Handle: handle},
		Endpoints: []models.MCPProxyEndpoint{endpoint},
	}
}

func newScopeSvcForTest(scopeRepo repositories.MCPProxyScopeRepository, proxy *models.MCPProxy) MCPProxyScopeService {
	// Tests that focus on the create path don't all set GetFunc; provide a
	// default "not found" so the duplicate-check path can run.
	if mock, ok := scopeRepo.(*repomocks.MCPProxyScopeRepositoryMock); ok && mock.GetFunc == nil {
		mock.GetFunc = func(_ context.Context, _ uuid.UUID, _ string) (*models.MCPProxyScope, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}

	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(ctx context.Context, handle, orgUUID string) (*models.MCPProxy, error) {
			if proxy != nil && proxy.Artifact.Handle == handle {
				return proxy, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	return NewMCPProxyScopeService(scopeRepo, proxyRepo, nil, nil, nil, nil, slog.Default())
}

func TestMCPProxyScopeCreate_ValidatesAction(t *testing.T) {
	svc := newScopeSvcForTest(&repomocks.MCPProxyScopeRepositoryMock{}, scopeTestProxy("gh-proxy", "list_repos"))
	for _, action := range []string{"", "has space", "with:colon", "with/slash", strings.Repeat("a", 101)} {
		_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
			models.MCPProxyScopeInput{Action: action, Tools: []string{"list_repos"}})
		assert.ErrorIs(t, err, utils.ErrInvalidInput, "action %q", action)
	}
}

func TestMCPProxyScopeCreate_StrictToolValidationWhenCapabilitiesKnown(t *testing.T) {
	svc := newScopeSvcForTest(&repomocks.MCPProxyScopeRepositoryMock{}, scopeTestProxy("gh-proxy", "list_repos", "get_repo"))
	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		models.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos", "not_a_tool"}})
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "not_a_tool")
}

func TestMCPProxyScopeCreate_PermissiveWhenNoCapabilitiesStored(t *testing.T) {
	var created *models.MCPProxyScope
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		CreateFunc: func(ctx context.Context, s *models.MCPProxyScope) error { created = s; return nil },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy")) // zero tools stored
	res, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		models.MCPProxyScopeInput{Action: "read", Tools: []string{"anything_goes"}})
	assert.NoError(t, err)
	assert.Equal(t, "gh-proxy", res.ProxyHandle)
	assert.Equal(t, []string{"anything_goes"}, created.Tools)
}

func TestMCPProxyScopeCreate_DuplicateActionConflicts(t *testing.T) {
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		GetFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
			return &models.MCPProxyScope{Action: action}, nil // already exists
		},
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy", "list_repos"))
	_, err := svc.Create(context.Background(), "org-uuid", "org", "gh-proxy",
		models.MCPProxyScopeInput{Action: "read", Tools: []string{"list_repos"}})
	assert.ErrorIs(t, err, utils.ErrConflict)
}

func TestMCPProxyScopeCreate_UnknownProxy404(t *testing.T) {
	svc := newScopeSvcForTest(&repomocks.MCPProxyScopeRepositoryMock{}, nil)
	_, err := svc.Create(context.Background(), "org-uuid", "org", "ghost",
		models.MCPProxyScopeInput{Action: "read", Tools: []string{"t"}})
	assert.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
}

func TestMCPProxyScopeDelete_MissingIsNotFound(t *testing.T) {
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		DeleteFunc: func(ctx context.Context, proxyUUID uuid.UUID, action string) error { return gorm.ErrRecordNotFound },
	}
	svc := newScopeSvcForTest(scopeRepo, scopeTestProxy("gh-proxy"))
	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy", "read")
	assert.ErrorIs(t, err, utils.ErrScopeNotFound)
}
