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

import (
	"context"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOpenBaoReadWriter struct {
	ReadWithContextFunc   func(ctx context.Context, path string) (*vault.Secret, error)
	WriteWithContextFunc  func(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error)
	DeleteWithContextFunc func(ctx context.Context, path string) (*vault.Secret, error)
}

func (f *fakeOpenBaoReadWriter) ReadWithContext(ctx context.Context, path string) (*vault.Secret, error) {
	return f.ReadWithContextFunc(ctx, path)
}
func (f *fakeOpenBaoReadWriter) WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error) {
	return f.WriteWithContextFunc(ctx, path, data)
}
func (f *fakeOpenBaoReadWriter) DeleteWithContext(ctx context.Context, path string) (*vault.Secret, error) {
	return f.DeleteWithContextFunc(ctx, path)
}

func TestAgentSecretStore_StoreAndGet(t *testing.T) {
	var writtenPath string
	var writtenData map[string]interface{}
	rw := &fakeOpenBaoReadWriter{
		WriteWithContextFunc: func(_ context.Context, p string, data map[string]interface{}) (*vault.Secret, error) {
			writtenPath = p
			writtenData = data
			return &vault.Secret{}, nil
		},
	}
	store := newAgentSecretStoreWithReadWriter(rw, "secret")

	secretPath, err := store.Store(context.Background(), "acme", "proj1", "staging", "my-agent", "client-abc", "secret-xyz")
	require.NoError(t, err)
	assert.Equal(t, "agent-thunder-clients/acme/proj1/staging/my-agent", secretPath)
	assert.Equal(t, "secret/data/agent-thunder-clients/acme/proj1/staging/my-agent", writtenPath)

	inner, ok := writtenData["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "client-abc", inner["client_id"])
	assert.Equal(t, "secret-xyz", inner["client_secret"])
}

func TestAgentSecretStore_Get_Success(t *testing.T) {
	rw := &fakeOpenBaoReadWriter{
		ReadWithContextFunc: func(_ context.Context, p string) (*vault.Secret, error) {
			assert.Equal(t, "secret/data/agent-thunder-clients/acme/proj1/staging/my-agent", p)
			return &vault.Secret{
				Data: map[string]any{
					"data": map[string]any{
						"client_id":     "client-abc",
						"client_secret": "secret-xyz",
					},
				},
			}, nil
		},
	}
	store := newAgentSecretStoreWithReadWriter(rw, "secret")

	clientID, clientSecret, err := store.Get(context.Background(), "agent-thunder-clients/acme/proj1/staging/my-agent")
	require.NoError(t, err)
	assert.Equal(t, "client-abc", clientID)
	assert.Equal(t, "secret-xyz", clientSecret)
}

func TestAgentSecretStore_Get_NotFound(t *testing.T) {
	rw := &fakeOpenBaoReadWriter{
		ReadWithContextFunc: func(_ context.Context, _ string) (*vault.Secret, error) {
			return nil, nil
		},
	}
	store := newAgentSecretStoreWithReadWriter(rw, "secret")

	_, _, err := store.Get(context.Background(), "agent-thunder-clients/acme/proj1/staging/my-agent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentSecretNotFound)
}

func TestAgentSecretStore_Delete(t *testing.T) {
	var deletedPath string
	rw := &fakeOpenBaoReadWriter{
		DeleteWithContextFunc: func(_ context.Context, p string) (*vault.Secret, error) {
			deletedPath = p
			return nil, nil
		},
	}
	store := newAgentSecretStoreWithReadWriter(rw, "secret")

	err := store.Delete(context.Background(), "agent-thunder-clients/acme/proj1/staging/my-agent")
	require.NoError(t, err)
	assert.Equal(t, "secret/metadata/agent-thunder-clients/acme/proj1/staging/my-agent", deletedPath,
		"delete must target the metadata path so it permanently destroys all versions, not just soft-delete the latest")
}
