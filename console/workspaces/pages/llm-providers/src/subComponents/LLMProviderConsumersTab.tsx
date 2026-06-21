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

import React from "react";
import { useListLLMProviderConsumers } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type LLMProviderConsumerItem,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate } from "react-router-dom";
import { ArrowRight } from "@wso2/oxygen-ui-icons-react";

interface LLMProviderConsumersTabProps {
  orgName: string | undefined;
  providerId: string | undefined;
}

function consumerLink(
  orgId: string,
  consumer: LLMProviderConsumerItem,
): string | undefined {
  if (consumer.consumerType === "agent") {
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.agents.children
        .configure.path,
      {
        orgId,
        projectId: consumer.projectName,
        agentId: consumer.consumerName,
      },
    );
  }
  // Monitors require envId which we don't have
  return undefined;
}

export const LLMProviderConsumersTab: React.FC<
  LLMProviderConsumersTabProps
> = ({ orgName, providerId }) => {
  const navigate = useNavigate();

  const { data, isLoading, error } = useListLLMProviderConsumers({
    orgName,
    providerId,
  });

  if (isLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ mt: 2 }}>
        Failed to load consumers.
      </Alert>
    );
  }

  const consumers = data?.consumers ?? [];

  if (consumers.length === 0) {
    return (
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          py: 8,
          gap: 1,
          color: "text.secondary",
        }}
      >
        <Typography variant="body1">No consumers yet</Typography>
        <Typography variant="body2">
          Agents and monitors that use an LLM proxy pointing to this provider
          will appear here.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ pt: 2 }}>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Project</TableCell>
              <TableCell sx={{ width: 140 }} />
            </TableRow>
          </TableHead>
          <TableBody>
            {consumers.map((consumer, idx) => {
              const href = orgName
                ? consumerLink(orgName, consumer)
                : undefined;
              return (
                <TableRow
                  key={idx}
                  hover
                  sx={{
                    "& .row-action": { visibility: "hidden" },
                    "&:hover .row-action": { visibility: "visible" },
                  }}
                >
                  <TableCell>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {consumer.consumerName}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    {consumer.consumerType === "agent" ? (
                      <Chip
                        label="Agent"
                        size="small"
                        variant="outlined"
                        color="primary"
                      />
                    ) : (
                      <Chip
                        label="Monitor"
                        size="small"
                        variant="outlined"
                        color="default"
                      />
                    )}
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" color="text.secondary">
                      {consumer.projectName}
                    </Typography>
                  </TableCell>
                  <TableCell align="right" padding="none" sx={{ pr: 1.5 }}>
                    {href && (
                      <Button
                        className="row-action"
                        variant="outlined"
                        size="small"
                        endIcon={<ArrowRight size={14} />}
                        onClick={() => navigate(href)}
                        aria-label={`Go to ${consumer.consumerType} ${consumer.consumerName}`}
                        sx={{ minWidth: 120 }}
                      >
                        {`Go to ${consumer.consumerType}`}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default LLMProviderConsumersTab;
