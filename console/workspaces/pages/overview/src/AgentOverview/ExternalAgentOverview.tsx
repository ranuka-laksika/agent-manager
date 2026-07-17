/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { globalConfig, type Environment } from '@agent-management-platform/types';
import { Box, Button, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import { useEffect, useState } from "react";
import {
  useGetAgent,
  useListGateways,
} from "@agent-management-platform/api-client";
import { EnvironmentCard, usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import { InstrumentationDrawer } from "./InstrumentationDrawer";
import { NoDataFound } from "@agent-management-platform/views";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const [selectedEnvironmentId, setSelectedEnvironmentId] = useState<string>("");

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Show only the environments in the current project's deployment pipeline,
  // ordered by the promotion chain. isLoading covers environments + project + pipelines.
  const { environments: sortedEnvironmentList, isLoading: isEnvironmentsLoading } =
    usePipelineEnvironmentsState(orgId, projectId);

  // Per-env OTEL endpoint. The gateway mapped to the selected environment carries
  // the externally-reachable vhost; the OTEL RestApi is published at `<vhost>/otel`.
  // Falls back to globalConfig only when the gateway lookup hasn't resolved yet
  // (e.g. before an env is selected).
  const { data: envGatewayList } = useListGateways(
    { orgName: orgId ?? "" },
    { environment: selectedEnvironmentId },
  );
  const envGatewayVhost = envGatewayList?.gateways?.[0]?.vhost;
  const agentInstrumentationUrl = envGatewayVhost
    ? `${envGatewayVhost.replace(/\/$/, "")}/otel`
    : (globalConfig.instrumentationUrl || "http://default-default.gateway.localhost:19080/otel");

  const handleSetupAgent = (environmentId: string) => {
    setSelectedEnvironmentId(environmentId);
    setSearchParams({ setup: "true" });
  };

  useEffect(() => {
    if (!isEnvironmentsLoading && !selectedEnvironmentId) {
      setSelectedEnvironmentId(sortedEnvironmentList.length > 0 ? (sortedEnvironmentList[0].id ?? "") : "");
    }
  }, [sortedEnvironmentList, isEnvironmentsLoading, selectedEnvironmentId]);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {isEnvironmentsLoading ? (
          <Box display="flex" flexDirection="column" gap={2}>
            <Skeleton variant="rounded" height={100} />
            <Skeleton variant="rounded" height={100} />
          </Box>
        ) : sortedEnvironmentList.length === 0 ? (
          <NoDataFound
            message="No environments found"
            subtitle="Environments will appear here once they are created"
          />
        ) : (
          <Stack spacing={2}>
            {sortedEnvironmentList.map(
              (environment: Environment) =>
                environment && orgId && projectId && agentId && (
                  <EnvironmentCard
                    key={environment.name}
                    orgId={orgId}
                    projectId={projectId}
                    agentId={agentId}
                    environment={environment}
                    actions={
                      <Button
                        variant="text"
                        size="small"
                        startIcon={<Settings size={16} />}
                        onClick={() => handleSetupAgent(environment.id ?? "")}
                      >
                        Setup Agent
                      </Button>
                    }
                    bottomContent={
                      <>
                        <EnvAgentRolesGroupsSection
                          orgId={orgId}
                          projectId={projectId}
                          agentId={agentId}
                          envId={environment.name}
                        />
                        <EnvMonitorsSection
                          orgId={orgId}
                          projectId={projectId}
                          agentId={agentId}
                          envId={environment.name}
                        />
                        <EnvObservabilitySection
                          orgId={orgId}
                          projectId={projectId}
                          agentId={agentId}
                          envId={environment.name}
                          hideMetrics
                          external
                        />
                      </>
                    }
                  />
                )
            )}
          </Stack>
        )}
      </Box>
      <InstrumentationDrawer
        open={searchParams.get("setup") === "true" && selectedEnvironmentId !== ""}
        onClose={() => setSearchParams({})}
        agentId={agentId ?? ""}
        orgName={orgId ?? "default"}
        projName={projectId ?? "default"}
        agentName={agentId ?? ""}
        environment={
          sortedEnvironmentList?.find((env: Environment) => env.id === selectedEnvironmentId)?.name
        }
        instrumentationUrl={agentInstrumentationUrl}
        componentUid={agent?.uuid}
        environmentUid={selectedEnvironmentId}
      />
    </>
  );
};
