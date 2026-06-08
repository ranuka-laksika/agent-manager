/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import {
  useGetProject,
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap, type Environment, type PromotionPath } from "@agent-management-platform/types";
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  Skeleton,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowRight, Edit, GitBranch } from "@wso2/oxygen-ui-icons-react";
import { useMemo } from "react";
import { generatePath, Link, useParams } from "react-router-dom";

const CARD_SX = {
  minWidth: 300,
  flexGrow: 1,
  "&.MuiCard-root": { backgroundColor: "background.paper" },
};

function buildPromotionChain(paths: PromotionPath[]): string[] {
  if (!paths.length) return [];

  const allTargets = new Set(paths.flatMap((p) => p.targetEnvironmentRefs.map((t) => t.name)));
  const adjacency = new Map<string, string[]>(
    paths.map((p) => [p.sourceEnvironmentRef, p.targetEnvironmentRefs.map((t) => t.name)])
  );

  const roots = [...new Set(paths.map((p) => p.sourceEnvironmentRef))].filter(
    (s) => !allTargets.has(s)
  );

  const chain: string[] = [];
  const visited = new Set<string>();
  let current: string | undefined = roots[0];

  while (current && !visited.has(current)) {
    chain.push(current);
    visited.add(current);
    current = (adjacency.get(current) ?? [])[0];
  }

  allTargets.forEach((t) => {
    if (!visited.has(t)) chain.push(t);
  });

  return chain;
}

export function DeploymentPipelineCard() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  const { data: project, isLoading: isLoadingProject } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });

  const { data: pipelinesData, isLoading: isLoadingPipelines } = useListDeploymentPipelines(
    { orgName: orgId },
  );

  const { data: environments } = useListEnvironments({ orgName: orgId });

  const pipeline = useMemo(
    () => pipelinesData?.deploymentPipelines?.find((p) => p.name === project?.deploymentPipeline),
    [pipelinesData, project?.deploymentPipeline]
  );

  const envMap = useMemo(
    () => new Map<string, Environment>(environments?.map((e) => [e.name, e]) ?? []),
    [environments]
  );

  const promotionChain = useMemo(
    () => buildPromotionChain(pipeline?.promotionPaths ?? []),
    [pipeline]
  );

  const isLoading = isLoadingProject || isLoadingPipelines;
  const hasPipeline = !!project?.deploymentPipeline;

  if (isLoading) {
    return (
      <Card variant="outlined" sx={CARD_SX}>
        <CardContent>
          <Box display="flex" flexDirection="column" gap={1.5}>
            <Box display="flex" alignItems="center" gap={1}>
              <Skeleton variant="circular" width={18} height={18} />
              <Skeleton variant="text" width="60%" height={28} />
            </Box>
            <Skeleton variant="text" width="85%" height={16} />
            <Divider />
            <Skeleton variant="text" width="45%" height={12} />
            <Box display="flex" alignItems="center" gap={1}>
              <Skeleton variant="rounded" width={64} height={24} sx={{ borderRadius: 4 }} />
              <Skeleton variant="text" width={14} height={14} />
              <Skeleton variant="rounded" width={72} height={24} sx={{ borderRadius: 4 }} />
              <Skeleton variant="text" width={14} height={14} />
              <Skeleton variant="rounded" width={80} height={24} sx={{ borderRadius: 4 }} />
            </Box>
          </Box>
        </CardContent>
      </Card>
    );
  }

  if (!hasPipeline || !pipeline) {
    return (
      <Card variant="outlined" sx={CARD_SX}>
        <CardContent>
          <Box display="flex" flexDirection="column" gap={1.5}>
            <Typography variant="h6">Deployment Environments</Typography>
            <Divider />
            <Typography variant="body2" color="text.secondary" sx={{ fontStyle: "italic" }}>
              No deployment pipeline configured for this project.
            </Typography>
          </Box>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card variant="outlined" sx={CARD_SX}>
      <CardContent>
        <Box display="flex" flexDirection="column" gap={1.5}>
          <Typography variant="h6">Project Environments</Typography>
          <Divider />
          <Box display="flex" flexDirection="column" gap={0.5}>
            <Box display="flex" alignItems="center" gap={0.75}>
              <GitBranch size={14} style={{ opacity: 0.6, flexShrink: 0 }} />
              <Typography variant="body2" fontWeight={500}>
                {pipeline.displayName || pipeline.name}
              </Typography>
            </Box>
            {pipeline.description && (
              <Typography variant="body2" color="text.disabled" sx={{ pl: 2.75 }}>
                {pipeline.description}
              </Typography>
            )}
          </Box>

          <Divider />

          <Box display="flex" flexWrap="wrap" alignItems="center" gap={0.75}>
            {promotionChain.length > 0 ? (
              promotionChain.map((envName, index) => {
                const env = envMap.get(envName);
                const label = env?.displayName || envName;
                const isProd = env?.isProduction ?? false;
                return (
                  <Box key={envName} display="flex" alignItems="center" gap={0.75}>
                    <Tooltip title={isProd ? "Production" : ""} disableHoverListener={!isProd}>
                      <Chip
                        label={label}
                        size="small"
                        color={isProd ? "primary" : "default"}
                        variant="outlined"
                      />
                    </Tooltip>
                    {index < promotionChain.length - 1 && (
                      <ArrowRight size={14} style={{ opacity: 0.45, flexShrink: 0 }} />
                    )}
                  </Box>
                );
              })
            ) : (
              <Typography variant="body2" color="text.disabled" sx={{ fontStyle: "italic" }}>
                No promotion paths defined.
              </Typography>
            )}
            <Tooltip title="Edit Promotion Chain">
              <Button
                size="small"
                startIcon={<Edit size={14} />}
                component={Link}
                to={generatePath(absoluteRouteMap.children.org.children.deploymentPipelines.path, { orgId }) + `?edit=${pipeline.name}`}
              >
            
                Edit
              </Button>
            </Tooltip>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}
