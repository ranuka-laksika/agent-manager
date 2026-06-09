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

import { useGetProject, useListDeploymentPipelines, useListEnvironments } from "@agent-management-platform/api-client";
import { FormControl, MenuItem, Select, Typography } from "@wso2/oxygen-ui";
import { useMemo } from "react";
import { useLocation, useNavigate, useParams } from "react-router-dom";

/**
 * Self-contained environment selector for env-scoped pages.
 * Reads envId from the URL, lists only the environments that belong to the
 * current project's deployment pipeline promotion chain, and navigates to the
 * same page with the new envId when the selection changes.
 * Renders nothing when there is only one qualifying environment or no envId.
 */
export function EnvironmentSelector() {
    const { orgId, projectId, envId } = useParams<{
        orgId: string; projectId: string; envId: string;
    }>();
    const { pathname } = useLocation();
    const navigate = useNavigate();

    const { data: environments } = useListEnvironments({ orgName: orgId });
    const { data: project } = useGetProject({ orgName: orgId, projName: projectId });
    const { data: pipelinesData } = useListDeploymentPipelines({ orgName: orgId });

    const pipelineEnvironments = useMemo(() => {
        if (!environments) return [];

        const paths = pipelinesData?.deploymentPipelines
            ?.find((p) => p.name === project?.deploymentPipeline)
            ?.promotionPaths ?? [];

        if (!paths.length) return environments;

        // Build adjacency and compute a topological order so branched promotion
        // graphs (a → b, a → c, b → d, c → d) stay correctly ordered instead of
        // following only the first outgoing edge of each node.
        const adjacency = new Map<string, string[]>();
        const allNodes = new Set<string>();
        const inDegree = new Map<string, number>();

        for (const p of paths) {
            const targets = p.targetEnvironmentRefs.map((t) => t.name).filter(Boolean);
            adjacency.set(p.sourceEnvironmentRef, targets);
            allNodes.add(p.sourceEnvironmentRef);
            inDegree.set(p.sourceEnvironmentRef, inDegree.get(p.sourceEnvironmentRef) ?? 0);
            for (const t of targets) {
                allNodes.add(t);
                inDegree.set(t, (inDegree.get(t) ?? 0) + 1);
            }
        }

        const chain: string[] = [];
        const queue = [...allNodes].filter((n) => (inDegree.get(n) ?? 0) === 0);
        while (queue.length > 0) {
            const node = queue.shift()!;
            chain.push(node);
            for (const neighbor of adjacency.get(node) ?? []) {
                const deg = (inDegree.get(neighbor) ?? 1) - 1;
                inDegree.set(neighbor, deg);
                if (deg === 0) queue.push(neighbor);
            }
        }

        // Fallback for cycles/invalid graphs: keep any node that didn't make it
        // into the topo order so we never silently drop environments.
        allNodes.forEach((n) => { if (!chain.includes(n)) chain.push(n); });

        return chain
            .map((name) => environments.find((e) => e.name === name))
            .filter(Boolean) as typeof environments;
    }, [environments, pipelinesData, project?.deploymentPipeline]);

    const selectedEnvironment = useMemo(
        () => pipelineEnvironments.find((env) => env.name === envId),
        [pipelineEnvironments, envId],
    );

    if (!envId || pipelineEnvironments.length <= 1) {
        return null;
    }

    return (
        <FormControl size="small" sx={{ minWidth: 160 }}>
            <Select
                value={envId}
                onChange={(e) => {
                    const newEnvId = e.target.value as string;
                    navigate(
                        pathname.replace(`/environment/${envId}`, `/environment/${newEnvId}`),
                    );
                }}
                renderValue={(value) => (
                    <Typography>
                        {selectedEnvironment?.displayName ?? value}
                        {" "}
                        Environment
                    </Typography>
                )}
            >
                {pipelineEnvironments.map((env) => (
                    <MenuItem key={env.name} value={env.name}>
                        {env.displayName ?? env.name}
                    </MenuItem>
                ))}
            </Select>
        </FormControl>
    );
}
