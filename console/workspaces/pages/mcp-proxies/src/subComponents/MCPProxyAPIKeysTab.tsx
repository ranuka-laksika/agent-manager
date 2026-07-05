/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  useCreateMCPProxyAPIKey,
  useListMCPProxyAPIKeys,
  useRevokeMCPProxyAPIKey,
} from "@agent-management-platform/api-client";
import {
  APIKeysManager,
  isApiKeyAuthEnabled,
  type CreateAPIKeyInput,
} from "@agent-management-platform/shared-component";
import type { MCPProxy } from "@agent-management-platform/types";
import { Alert, Skeleton } from "@wso2/oxygen-ui";

export type MCPProxyAPIKeysTabProps = {
  proxy: MCPProxy | null | undefined;
  orgName: string | undefined;
  isLoading?: boolean;
};

export function MCPProxyAPIKeysTab({
  proxy,
  orgName,
  isLoading = false,
}: MCPProxyAPIKeysTabProps) {
  const apiKeyEnabled = isApiKeyAuthEnabled(proxy?.security);
  const proxyId = proxy?.id;

  const {
    data,
    isLoading: isLoadingKeys,
    isError,
  } = useListMCPProxyAPIKeys({
    orgName: apiKeyEnabled ? orgName : undefined,
    proxyId: apiKeyEnabled && proxyId ? proxyId : "",
  });

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateMCPProxyAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } =
    useRevokeMCPProxyAPIKey();

  const handleCreate = async ({ displayName, expiresAt }: CreateAPIKeyInput) => {
    if (!orgName || !proxyId) return undefined;
    const randomSuffix = Math.random().toString(36).slice(2, 10);
    const name = `mcp-${proxyId}-${randomSuffix}`;
    const res = await createKey({
      params: { orgName, proxyId },
      body: { name, displayName, expiresAt },
    });
    return res.apiKey;
  };

  const handleRevoke = (keyName: string) => {
    if (!orgName || !proxyId) return;
    revokeKey({ orgName, proxyId, keyName });
  };

  if (isLoading) {
    return <Skeleton variant="rectangular" width="100%" height={200} />;
  }

  if (!apiKeyEnabled) {
    return (
      <Alert severity="info">
        Enable API key authentication from the Security tab to manage API keys.
      </Alert>
    );
  }

  return (
    <APIKeysManager
      keys={data?.keys}
      isLoading={isLoadingKeys}
      isError={isError}
      isCreating={isCreating}
      isRevoking={isRevoking}
      emptyDescription="Create an API key to authenticate requests to this MCP proxy."
      onCreate={handleCreate}
      onRevoke={handleRevoke}
    />
  );
}

export default MCPProxyAPIKeysTab;
