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
	"fmt"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// ResolveNamespace resolves the OpenChoreo namespace (organization) name.
// Trace ingestion, the observer's search-scope resolution, and env-Thunder
// naming are all keyed by this name (e.g. "default"), NOT by the OU id from
// the JWT — passing the raw OU id where a namespace is expected targets
// resources that don't exist. Single-namespace deployment: the first (only)
// organization is the one every OU maps to.
func ResolveNamespace(ctx context.Context, ocClient occlient.OpenChoreoClient) (string, error) {
	orgs, err := ocClient.ListOrganizations(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list organizations for namespace resolution: %w", err)
	}
	if len(orgs) == 0 {
		return "", fmt.Errorf("no organization found for namespace resolution")
	}
	return orgs[0].Namespace, nil
}

// ThunderOrgNamespace returns the org namespace/handle used to address an
// env-Thunder instance (see thundersvc.ThunderReleaseName) for AgentID
// provisioning and injection. Deliberately config-pinned
// (OPEN_CHOREO_DEFAULT_NAMESPACE, default "default"), NOT resolved dynamically:
// an env-Thunder's Helm release/namespace is fixed when the platform admin
// deploys it, so a dynamic lookup that followed an org rename would start
// addressing a Thunder that doesn't exist and break every provisioned agent.
// Both the provisioning and injection services call this so they can't disagree.
func ThunderOrgNamespace() string {
	return config.GetConfig().OpenChoreo.DefaultNamespace
}

// ResolveEnvThunderClient is the single place every caller resolves a
// ThunderClient: credential lookup is scoped by ouID (multi-tenant-safe),
// while addressing stays pinned to ThunderOrgNamespace(). Call this instead of
// resolver.Resolve directly, so the pairing can't drift between call sites.
func ResolveEnvThunderClient(ctx context.Context, resolver thundersvc.EnvThunderResolver, ouID, envName string) (thundersvc.ThunderClient, error) {
	return resolver.Resolve(ctx, ouID, ThunderOrgNamespace(), envName)
}

// ResolveEnvThunderIdentity is ResolveEnvThunderClient widened to the identity surface.
func ResolveEnvThunderIdentity(ctx context.Context, resolver thundersvc.EnvThunderResolver, ouID, envName string) (thundersvc.EnvIdentityClient, error) {
	return resolver.ResolveIdentity(ctx, ouID, ThunderOrgNamespace(), envName)
}
