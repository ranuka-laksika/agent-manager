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
	"errors"
	"fmt"
	"path"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// ErrAgentSecretNotFound is returned when no AgentID credential exists at the
// given path.
var ErrAgentSecretNotFound = errors.New("agent thunder secret not found")

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/agent_secret_store_fake.go . AgentSecretStore:AgentSecretStoreMock

// AgentSecretStore stores and retrieves an AgentID's Thunder client credentials
// in OpenBao. This is deliberately a raw KV store, independent of the
// SecretManagementClient/SecretReference machinery used elsewhere in this
// service: that machinery exists to inject secrets into OpenChoreo workloads as
// env vars, which is never appropriate for an AgentID credential — it is
// managed by the platform itself (for internal agents) or shown once and
// otherwise untouched (for external agents), never auto-mounted into a pod.
type AgentSecretStore interface {
	// Store writes the client ID and secret for one agent/environment binding
	// and returns the path it was stored at.
	Store(ctx context.Context, orgName, projectName, envName, agentName, clientID, clientSecret string) (secretPath string, err error)

	// Get retrieves the client ID and secret stored at secretPath.
	Get(ctx context.Context, secretPath string) (clientID, clientSecret string, err error)

	// Delete permanently destroys the credential at secretPath (all versions),
	// not just a soft-delete of the latest one.
	Delete(ctx context.Context, secretPath string) error
}

// openBaoReadWriter is the narrow slice of the vault/OpenBao API this store
// needs — kept minimal so it can be faked in tests without a real OpenBao server.
type openBaoReadWriter interface {
	ReadWithContext(ctx context.Context, path string) (*vault.Secret, error)
	WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vault.Secret, error)
	DeleteWithContext(ctx context.Context, path string) (*vault.Secret, error)
}

type agentSecretStore struct {
	rw          openBaoReadWriter
	openBaoPath string
}

// validateOpenBaoConfig fails fast on a missing OPENBAO_* value instead of
// letting it silently build a client that only errors on first actual use.
func validateOpenBaoConfig(openBaoURL, openBaoToken, openBaoPath string) error {
	if openBaoURL == "" {
		return errors.New("openbao url is required")
	}
	if openBaoToken == "" {
		return errors.New("openbao token is required")
	}
	if openBaoPath == "" {
		return errors.New("openbao path is required")
	}
	return nil
}

// NewAgentSecretStore creates an AgentSecretStore backed by a real OpenBao
// server at openBaoURL, authenticating with openBaoToken.
func NewAgentSecretStore(openBaoURL, openBaoToken, openBaoPath string) (AgentSecretStore, error) {
	if err := validateOpenBaoConfig(openBaoURL, openBaoToken, openBaoPath); err != nil {
		return nil, err
	}
	cfg := vault.DefaultConfig()
	cfg.Address = openBaoURL
	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}
	client.SetToken(openBaoToken)
	return newAgentSecretStoreWithReadWriter(client.Logical(), openBaoPath), nil
}

func newAgentSecretStoreWithReadWriter(rw openBaoReadWriter, openBaoPath string) *agentSecretStore {
	return &agentSecretStore{rw: rw, openBaoPath: openBaoPath}
}

// key builds the OpenBao path for one agent/environment secret. Agent and
// project names are validated to a safe [a-z0-9-]+ charset before they ever
// reach this service, but environment name and org name are not (environment
// name has only a paper OpenAPI pattern, never enforced at runtime; org name
// arrives as a bare path parameter with no validation in this service at
// all) — so every segment is checked here rather than assuming upstream
// safety, mirroring secretmanagersvc's own sanitizeSegment guard.
//
// Also rejects "", ".", and ".." — path.Join would otherwise silently clean
// those out of the resulting path.
func (s *agentSecretStore) key(orgName, projectName, envName, agentName string) (string, error) {
	for _, seg := range []string{orgName, projectName, envName, agentName} {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "/") {
			return "", fmt.Errorf("agent secret path segment %q is empty or contains an invalid character", seg)
		}
	}
	return path.Join("agent-thunder-clients", orgName, projectName, envName, agentName), nil
}

func (s *agentSecretStore) Store(ctx context.Context, orgName, projectName, envName, agentName, clientID, clientSecret string) (string, error) {
	key, err := s.key(orgName, projectName, envName, agentName)
	if err != nil {
		return "", err
	}
	dataPath := path.Join(s.openBaoPath, "data", key)

	_, err = s.rw.WriteWithContext(ctx, dataPath, map[string]interface{}{
		"data": map[string]interface{}{
			"client_id":     clientID,
			"client_secret": clientSecret,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to store agent thunder secret at %s: %w", key, err)
	}
	return key, nil
}

func (s *agentSecretStore) Get(ctx context.Context, secretPath string) (clientID, clientSecret string, err error) {
	dataPath := path.Join(s.openBaoPath, "data", secretPath)

	secret, err := s.rw.ReadWithContext(ctx, dataPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read agent thunder secret at %s: %w", secretPath, err)
	}
	if secret == nil || secret.Data == nil {
		return "", "", ErrAgentSecretNotFound
	}
	dataMap, ok := secret.Data["data"].(map[string]any)
	if !ok {
		return "", "", ErrAgentSecretNotFound
	}
	clientID, _ = dataMap["client_id"].(string)
	clientSecret, _ = dataMap["client_secret"].(string)
	if clientID == "" && clientSecret == "" {
		return "", "", ErrAgentSecretNotFound
	}
	return clientID, clientSecret, nil
}

func (s *agentSecretStore) Delete(ctx context.Context, secretPath string) error {
	// Deleting the metadata path (rather than the data path) permanently destroys
	// every version of the secret — a soft-delete on the data path would leave it
	// recoverable, which defeats the point of a one-time external-agent claim.
	metaPath := path.Join(s.openBaoPath, "metadata", secretPath)
	_, err := s.rw.DeleteWithContext(ctx, metaPath)
	if err != nil {
		return fmt.Errorf("failed to delete agent thunder secret at %s: %w", secretPath, err)
	}
	return nil
}
