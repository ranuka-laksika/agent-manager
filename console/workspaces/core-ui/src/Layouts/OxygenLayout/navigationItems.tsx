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

import { useMemo } from "react";
import {
  BarChart3 as AutoGraphOutlined,
  Binoculars as ObservabilityOutline,
  Settings2 as EvaluationOutline,
  Settings,
  Home,
  Wrench,
  FlaskConical,
  Workflow,
  Logs,
  Rocket,
  Code,
  MonitorCheck,
  BrainCircuit,
  Package,
  DoorClosedLocked,
  ServerCrash,
  Server,
  ShieldCheck,
} from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  matchPath,
  useLocation,
  useParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  globalConfig,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useGetAgent } from "@agent-management-platform/api-client";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import { thunderInstancesMetadata } from "@agent-management-platform/env-thunders/metadata";
import { useExternalNavItems } from "@agent-management-platform/views";
import type { NavigationItem, NavigationSection } from "./LeftNavigation";

// MCP logo inlined here so mcp-proxies package stays in its own async chunk.
const MCP_LOGO_MASK =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAALQAAAC0CAYAAAA9zQYyAAAAAXNSR0IArs4c6QAAAERlWElmTU0AKgAAAAgAAYdpAAQAAAABAAAAGgAAAAAAA6ABAAMAAAABAAEAAKACAAQAAAABAAAAtKADAAQAAAABAAAAtAAAAABW1ZZ5AAAPtElEQVR4Ae2dC/BuUxnGD45cIo5LicGHTgYRxshfOKYpNCSjMak0ZZTURKVG5ZJUU5mpIXJPRnTTVBppSuqENLnUyOQyueYyIXFCOuGo5zFWs32+y3rXu/Zee3/7WTPvf+9v77Xetdazft/6773W2vubN09BCkgBKSAFpIAUkAJSQApIASkgBaSAFJACUkAKSAEpIAWkgBSQAlJACkiB/imwXP+q3Moar4BSDWAbwlZ73pZh+wTsn7A7YQ/AFKYoIKCnCFTT6XXgdw/YItgusIWwFWGTwuM4eSPsStgVsMWwp2AKUqCIAvOR6ztgl8Kehv3XaY8g/TmwOZiCFGhMgVWQ00dh98C8EI9LfxV87w1TkAK1KrA/vN8NGwdi7uOXI68tYQpSIKsCvEa+BJYb2Bh/vJz5DGx5mIIUcCuwCB7uh8XAV2ecxSjDK921kYNeK3AQas+RhzpBtfjmdfsWvW4RVT5ZgSOR8lmYBbgm4v4DZXpdcq2UsJcKfBq1bgLO1Dw4xLdVL1tGlTYr0HaYw5fgPtSMM5EzHTRT6GtewvxFn4v/pyZ4vJn8K4xT3pwOXxO2GWwBLEe4Bk44M/lMDmfyMVsK5OiZeSlwNoyTImtMkGdjnHsvLMcs44kT8tGpnipAhfmBOgc6IMwN6DPgXMFBWgBSMcaMEFBzp9a2BzFPLSPuNe2QccBBZYFh+lZcadAW7aWMF8DGHWnhgABHMLtv2jCSwPJc0BzUNtaZ4K1AqwK7oeYnPFvYQWOQFUetMeQm8yYrexP3g2gHzWABXO46bvmIJ+4HXaGnFGH0+aAo4xp1pqOQ60rQpMIJi7p2YnxMNe";

const MCPLogo = ({ size = 20, className }: { size?: number | string; className?: string }) => (
  <span
    aria-hidden="true"
    className={className}
    style={{
      backgroundColor: "currentColor",
      color: "inherit",
      display: "inline-block",
      height: size,
      maskImage: `url(${MCP_LOGO_MASK})`,
      maskPosition: "center",
      maskRepeat: "no-repeat",
      maskSize: "contain",
      WebkitMaskImage: `url(${MCP_LOGO_MASK})`,
      WebkitMaskPosition: "center",
      WebkitMaskRepeat: "no-repeat",
      WebkitMaskSize: "contain",
      verticalAlign: "middle",
      width: size,
    }}
  />
);

/**
 * TODO: Use nav bar instead of navigate to the items.
 */

export function useNavigationItems(): Array<
  NavigationSection | NavigationItem
> {
  const { orgId, projectId, agentId, envId } = useParams();
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    agentName: agentId,
    orgName: orgId,
    projName: projectId,
  });
  const { environments, isLoading: isLoadingEnvironments } =
    usePipelineEnvironmentsState(orgId, projectId);

  const externalNavItems = useExternalNavItems();
  const { userInfo } = useAuthHooks();

  const navVisibility = useMemo(() => {
    const showAll = {
      resources: true,
      evaluation: true,
      infrastructure: true,
      identityUsers: true,
      identityRoles: true,
      identityGroups: true,
    };
    if (globalConfig.disableAuth || !globalConfig.rbacEnabled) return showAll;
    const scopeStr = userInfo?.scope;
    if (!scopeStr) {
      return {
        resources: false,
        evaluation: false,
        infrastructure: false,
        identityUsers: false,
        identityRoles: false,
        identityGroups: false,
      };
    }
    const s = new Set(scopeStr.split(" ").filter(Boolean));
    return {
      resources:
        s.has("amp:llm-provider:read") ||
        s.has("amp:llm-provider-template:read") ||
        s.has("amp:mcp-server:read") ||
        s.has("amp:llm-proxy:read"),
      evaluation: s.has("amp:evaluator:read"),
      infrastructure:
        s.has("amp:gateway:read") ||
        s.has("amp:deployment-pipeline:read") ||
        s.has("amp:environment:read"),
      identityUsers:
        s.has("amp:org:invite-member") || s.has("amp:org:remove-member"),
      identityRoles:
        s.has("amp:role:read") ||
        s.has("amp:role:create") ||
        s.has("amp:role:update") ||
        s.has("amp:role:delete"),
      identityGroups:
        s.has("amp:group:read") ||
        s.has("amp:group:create") ||
        s.has("amp:group:update") ||
        s.has("amp:group:delete"),
    };
  }, [userInfo?.scope]);

  const defaultEnv =
    envId ?? (environments.length > 0 ? environments[0]?.name : "");
  const { pathname } = useLocation();

  const llmProvidersOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).llmProviders;
  const mcpProxiesOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).mcpProxies;
  const agentsChildren = absoluteRouteMap.children.org.children.projects
    .children.agents.children as Record<
    string,
    { path: string; wildPath: string }
  >;
  const gatewaysOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).gateways;
  const settingsOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).settings;
  const deploymentPipelinesOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).deploymentPipelines;
  const environmentsOrgRoute = (
    absoluteRouteMap.children.org.children as unknown as Record<
      string,
      { path: string; wildPath: string }
    >
  ).environments;
  const evaluatorsOrgRoute = absoluteRouteMap.children.org.children.evaluators;

  if (isLoadingAgent || (isLoadingEnvironments && agentId)) {
    return [];
  }

  if (
    agent?.provisioning.type === "external" &&
    agentId &&
    projectId &&
    orgId
  ) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <Home size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
      {
        label: "Configure",
        type: "item",
        icon: <Settings size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.configure.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.configure.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        title: "Observability",
        type: "section",
        icon: <AutoGraphOutlined />,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
    ];
  }

  if (orgId && projectId && agentId && defaultEnv && agent?.kindName) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <Home size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Configure",
        type: "item",
        icon: <Settings size={20} />,
        isActive: !!matchPath(
          agentsChildren.configure?.wildPath ?? "",
          pathname,
        ),
        href: generatePath(agentsChildren.configure?.path ?? "", {
          orgId,
          projectId,
          agentId,
        }),
      },
      {
        label: "Deploy",
        type: "item",
        icon: <Rocket size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Try It",
        type: "item",
        icon: <FlaskConical size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
          { orgId, projectId, agentId, envId: defaultEnv },
        ),
      },
      ...(agent?.agentType?.type === "agent-api"
        ? [
            {
              title: "Security",
              type: "section" as const,
              icon: <ShieldCheck />,
              items: [
                {
                  label: "Credentials",
                  type: "item" as const,
                  icon: <ShieldCheck size={20} />,
                  isActive: !!matchPath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.wildPath,
                    pathname,
                  ),
                  href: generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.path,
                    { orgId, projectId, agentId, envId: defaultEnv },
                  ),
                },
              ],
            },
          ]
        : []),
      {
        title: "Observability",
        type: "section",
        icon: <ObservabilityOutline />,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "Runtime Logs",
            type: "item",
            icon: <Logs size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "System Metrics",
            type: "item",
            icon: <AutoGraphOutlined size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
    ];
  }
  if (orgId && projectId && agentId && defaultEnv && !agent?.kindName) {
    return [
      {
        label: "Overview",
        type: "item",
        icon: <Home size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Build",
        type: "item",
        icon: <Wrench size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.build.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.build.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Configure",
        type: "item",
        icon: <Settings size={20} />,
        isActive: !!matchPath(
          agentsChildren.configure?.wildPath ?? "",
          pathname,
        ),
        href: generatePath(agentsChildren.configure?.path ?? "", {
          orgId,
          projectId,
          agentId,
        }),
      },
      {
        label: "Deploy",
        type: "item",
        icon: <Rocket size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.deployment.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Publish",
        type: "item",
        icon: <Package size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.publish.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.publish.path,
          { orgId, projectId, agentId },
        ),
      },
      {
        label: "Try It",
        type: "item",
        icon: <FlaskConical size={20} />,
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.wildPath,
          pathname,
        ),
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
          { orgId, projectId, agentId, envId: defaultEnv },
        ),
      },
      ...(agent?.agentType?.type === "agent-api"
        ? [
            {
              title: "Security",
              type: "section" as const,
              icon: <ShieldCheck />,
              items: [
                {
                  label: "Credentials",
                  type: "item" as const,
                  icon: <ShieldCheck size={20} />,
                  isActive: !!matchPath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.wildPath,
                    pathname,
                  ),
                  href: generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.path,
                    { orgId, projectId, agentId, envId: defaultEnv },
                  ),
                },
              ],
            },
          ]
        : []),
      {
        title: "Observability",
        type: "section",
        icon: <ObservabilityOutline />,
        items: [
          {
            label: "Traces",
            type: "item",
            icon: <Workflow size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.traces
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "Runtime Logs",
            type: "item",
            icon: <Logs size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.logs.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
          {
            label: "System Metrics",
            type: "item",
            icon: <AutoGraphOutlined size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.observability.children.metrics
                .path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      {
        title: "Evaluation",
        type: "section",
        icon: <EvaluationOutline />,
        items: [
          {
            label: "Monitors",
            type: "item",
            icon: <MonitorCheck size={20} />,
            isActive: !!matchPath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor
                .wildPath,
              pathname,
            ),
            href: generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.environment.children.evaluation.children.monitor.path,
              { orgId, projectId, agentId, envId: defaultEnv },
            ),
          },
        ],
      },
      ...externalNavItems
        .filter((item) => item.level === "component")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId, projectId, agentId }),
        })),
    ];
  }
  if (orgId && projectId) {
    return [
      {
        label: "Agents",
        type: "item",
        icon: <Home size={20} />,
        href: generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          { orgId, projectId },
        ),
        isActive:
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.path,
            pathname,
          ) ||
          !!matchPath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .wildPath,
            pathname,
          ),
      },
    ];
  }
  if (orgId) {
    return [
      {
        label: "Projects",
        type: "item",
        icon: <Home size={20} />,
        href: generatePath(absoluteRouteMap.children.org.path, { orgId }),
        isActive: !!matchPath(absoluteRouteMap.children.org.path, pathname),
      },
      {
        label: "Agent Catalog",
        type: "item",
        icon: <Package size={20} />,
        href: generatePath(
          absoluteRouteMap.children.org.children.catalog.path,
          { orgId },
        ),
        isActive: !!matchPath(
          absoluteRouteMap.children.org.children.catalog.wildPath,
          pathname,
        ),
      },
      ...(navVisibility.identityUsers ||
      navVisibility.identityRoles ||
      navVisibility.identityGroups
        ? [
            {
              label: "Settings",
              type: "item" as const,
              icon: <Settings size={20} />,
              href: generatePath(settingsOrgRoute.path, { orgId }),
              isActive: !!matchPath(settingsOrgRoute.wildPath, pathname),
              pinBottom: true,
            },
          ]
        : []),
      ...(navVisibility.resources
        ? [
            {
              type: "section" as const,
              title: "Resources",
              icon: <Settings size={20} />,
              items: [
                {
                  label: "LLM Service Providers",
                  type: "item" as const,
                  icon: <BrainCircuit size={20} />,
                  href: generatePath(llmProvidersOrgRoute.path, { orgId }),
                  isActive: !!matchPath(
                    llmProvidersOrgRoute.wildPath,
                    pathname,
                  ),
                },
                {
                  label: "MCP Proxies",
                  type: "item" as const,
                  icon: <MCPLogo size={20} />,
                  href: generatePath(mcpProxiesOrgRoute.path, { orgId }),
                  isActive: !!matchPath(mcpProxiesOrgRoute.wildPath, pathname),
                },
              ],
            },
          ]
        : []),
      ...(navVisibility.evaluation
        ? [
            {
              title: "Evaluation",
              type: "section" as const,
              icon: <EvaluationOutline />,
              items: [
                {
                  label: "Evaluators",
                  type: "item" as const,
                  icon: <Code size={20} />,
                  isActive: !!matchPath(evaluatorsOrgRoute.wildPath, pathname),
                  href: generatePath(evaluatorsOrgRoute.path, { orgId }),
                },
              ],
            },
          ]
        : []),
      ...(navVisibility.infrastructure
        ? [
            {
              title: "Infrastructure",
              type: "section" as const,
              icon: <DoorClosedLocked />,
              items: [
                {
                  label: "Gateways",
                  type: "item" as const,
                  icon: <DoorClosedLocked size={20} />,
                  href: generatePath(gatewaysOrgRoute.path, { orgId }),
                  isActive: !!matchPath(gatewaysOrgRoute.wildPath, pathname),
                },
                {
                  label: "Deployment Pipelines",
                  type: "item" as const,
                  icon: <ServerCrash size={20} />,
                  href: generatePath(deploymentPipelinesOrgRoute.path, {
                    orgId,
                  }),
                  isActive: !!matchPath(
                    deploymentPipelinesOrgRoute.wildPath,
                    pathname,
                  ),
                },
                {
                  label: "Environments",
                  type: "item" as const,
                  icon: <Server size={20} />,
                  href: generatePath(environmentsOrgRoute.path, { orgId }),
                  isActive: !!matchPath(
                    environmentsOrgRoute.wildPath,
                    pathname,
                  ),
                },
                {
                  label: thunderInstancesMetadata.title,
                  type: "item" as const,
                  icon: <thunderInstancesMetadata.icon size={20} />,
                  href: generatePath(
                    absoluteRouteMap.children.org.children.thunderInstances
                      .path,
                    { orgId },
                  ),
                  isActive: !!matchPath(
                    absoluteRouteMap.children.org.children.thunderInstances
                      .wildPath,
                    pathname,
                  ),
                },
              ],
            },
          ]
        : []),
      ...externalNavItems
        .filter((item) => item.level === "org")
        .map((item) => ({
          label: item.title,
          type: "item" as const,
          icon: item.icon,
          isActive: !!matchPath(item.route, pathname),
          href: generatePath(item.route, { orgId }),
        })),
    ];
  }
  return [];
}
