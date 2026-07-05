/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { useParams } from "react-router-dom";
import { Alert, Box, Skeleton } from "@wso2/oxygen-ui";
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateAgentAPIKey,
  useGetAgent,
  useListAgentDeployments,
  useListAgentAPIKeys,
  useRevokeAgentAPIKey,
} from "@agent-management-platform/api-client";
import {
  APIKeysManager,
  EnvironmentSelector,
  type CreateAPIKeyInput,
} from "@agent-management-platform/shared-component";
import { NoDataFound, PageLayout } from "@agent-management-platform/views";

export const SecurityComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId } = useParams();

  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const { data: deployments, isLoading: isLoadingDeployments } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });

  const securityEnabled = agent?.configurations?.enableApiKeySecurity ?? true;
  const currentDeployment = envId ? deployments?.[envId] : undefined;
  const hasActiveDeployment = currentDeployment?.status === "active";
  const shouldLoadKeys =
    !isLoadingAgent &&
    !isLoadingDeployments &&
    hasActiveDeployment &&
    securityEnabled &&
    !!envId;
  const {
    data: keys,
    isLoading: isLoadingKeys,
    isError,
  } = useListAgentAPIKeys({
    orgName: shouldLoadKeys ? orgId : undefined,
    projName: shouldLoadKeys ? projectId : undefined,
    agentName: shouldLoadKeys ? agentId : undefined,
    envId: shouldLoadKeys ? envId : undefined,
  });
  const isLoading =
    isLoadingAgent || isLoadingDeployments || (shouldLoadKeys && isLoadingKeys);

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateAgentAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } = useRevokeAgentAPIKey();

  const handleCreate = async ({ displayName, expiresAt }: CreateAPIKeyInput) => {
    const data = await createKey({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        envId,
      },
      body: { displayName, expiresAt },
    });
    return data.apiKey;
  };

  const handleRevoke = (keyName: string) => {
    revokeKey({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
      envId,
      keyName,
    });
  };

  if (!isLoading && !hasActiveDeployment) {
    return (
      <PageLayout title="API Keys" disableIcon actions={<EnvironmentSelector />}>
        <Box
          height="50vh"
          display="flex"
          justifyContent="center"
          alignItems="center"
        >
          <NoDataFound
            iconElement={Rocket}
            disableBackground
            message="Agent is not deployed"
            subtitle="Deploy your agent to manage API keys. You can deploy your agent by clicking the deploy button in the deploy tab."
          />
        </Box>
      </PageLayout>
    );
  }

  return (
    <PageLayout title="API Keys" disableIcon actions={<EnvironmentSelector />}>
      {isLoading ? (
        <Skeleton variant="rectangular" width="100%" height={200} />
      ) : !securityEnabled ? (
        <Alert severity="info">
          API Key Security is disabled for this agent. To manage API keys, enable
          it from the <strong>Deployment</strong> settings and redeploy.
        </Alert>
      ) : (
        <APIKeysManager
          keys={keys}
          isLoading={false}
          isError={isError}
          isCreating={isCreating}
          isRevoking={isRevoking}
          emptyDescription="Create an API key to authenticate requests to this agent."
          onCreate={handleCreate}
          onRevoke={handleRevoke}
        />
      )}
    </PageLayout>
  );
};
