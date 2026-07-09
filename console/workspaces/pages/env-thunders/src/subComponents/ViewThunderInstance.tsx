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
import React, { type ReactNode, useCallback } from "react";
import {
  Alert,
  Box,
  Card,
  Chip,
  Divider,
  Grid,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  ListSubheader,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  CheckCircle,
  Folder,
  LayoutGrid,
  Shield,
  Users,
} from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  matchPath,
  useLocation,
  useParams,
  Link,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";
import { ThunderInstanceOverviewTab } from "./ThunderInstanceOverviewTab";
import { AgentsTab } from "./agentIdentity/AgentsTab";
import { GroupsPage } from "./agentIdentity/GroupsPage";
import { GroupCreatePage } from "./agentIdentity/GroupCreatePage";
import { GroupEditPage } from "./agentIdentity/GroupEditPage";
import { RolesPage } from "./agentIdentity/RolesPage";
import { RoleCreatePage } from "./agentIdentity/RoleCreatePage";
import { RoleEditPage } from "./agentIdentity/RoleEditPage";

interface NavTab {
  label: string;
  icon: ReactNode;
  href: string;
  wildPath?: string;
}

const ICON_SX_ACTIVE = { minWidth: 36, color: "primary.main" } as const;
const ICON_SX_INACTIVE = { minWidth: 36, color: "inherit" } as const;
const TEXT_SX_ACTIVE = { color: "text.secondary" } as const;
const TEXT_SX_INACTIVE = { color: "inherit" } as const;

interface NavItemProps {
  tab: NavTab;
  isActive: boolean;
}

function NavItem({ tab, isActive }: NavItemProps) {
  return (
    <Link to={tab.href} style={{ textDecoration: "none", color: "inherit" }}>
      <ListItemButton selected={isActive}>
        <ListItemIcon sx={isActive ? ICON_SX_ACTIVE : ICON_SX_INACTIVE}>{tab.icon}</ListItemIcon>
        <ListItemText primary={tab.label} sx={isActive ? TEXT_SX_ACTIVE : TEXT_SX_INACTIVE} />
      </ListItemButton>
    </Link>
  );
}

export const ViewThunderInstance: React.FC = () => {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const { pathname } = useLocation();
  const { pushSnackBar } = useSnackBar();

  const { data, isLoading, error } = useListThunderInstances({ orgName: orgId });
  const instance = data?.thunderInstances.find((i) => i.envName === envName);

  const handleCopy = useCallback(
    (value: string, label: string) => {
      void copyToClipboard(value).then((succeeded) => {
        pushSnackBar(
          succeeded
            ? { message: `${label} copied to clipboard`, type: "success" }
            : { message: `Failed to copy ${label}`, type: "error" },
        );
      });
    },
    [pushSnackBar],
  );

  const displayName = instance?.displayName || instance?.envName || envName || "";

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.thunderInstances.path,
    { orgId: orgId ?? "" },
  );

  const viewNode = absoluteRouteMap.children.org.children.thunderInstances.children.view;
  const overviewHref = generatePath(viewNode.path, { orgId: orgId ?? "", envName: envName ?? "" });
  const overviewTab: NavTab = { label: "Overview", icon: <LayoutGrid size={18} />, href: overviewHref };

  const identityManagementTabs: NavTab[] = [
    {
      label: "Agents",
      icon: <Users size={18} />,
      href: generatePath(viewNode.children.agents.path, { orgId: orgId ?? "", envName: envName ?? "" }),
      wildPath: viewNode.children.agents.wildPath,
    },
    {
      label: "Roles",
      icon: <Shield size={18} />,
      href: generatePath(viewNode.children.roles.path, { orgId: orgId ?? "", envName: envName ?? "" }),
      wildPath: viewNode.children.roles.wildPath,
    },
    {
      label: "Groups",
      icon: <Folder size={18} />,
      href: generatePath(viewNode.children.groups.path, { orgId: orgId ?? "", envName: envName ?? "" }),
      wildPath: viewNode.children.groups.wildPath,
    },
  ];

  const isOverviewActive = pathname === overviewHref;

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={backHref}
        backLabel="Back to Identity Providers"
        disableIcon
        isLoading={isLoading}
        titleTail={
          instance && !error ? (
            <Tooltip title="Thunder identity provider is active for this environment">
              <Chip
                icon={<CheckCircle size={14} />}
                label="Active"
                size="small"
                color="success"
                variant="outlined"
              />
            </Tooltip>
          ) : undefined
        }
      >
        {isLoading && (
          <Stack spacing={3}>
            <Grid container spacing={2}>
              {[0, 1, 2, 3].map((i) => (
                <Grid key={i} size={{ xs: 12, sm: 6, md: 3 }}>
                  <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                    <Stack spacing={0.5}>
                      <Skeleton variant="text" width="40%" height={14} />
                      <Skeleton variant="text" width="85%" height={20} />
                    </Stack>
                  </Card>
                </Grid>
              ))}
            </Grid>
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={140} height={24} />
                <Skeleton variant="rounded" height={80} />
              </Stack>
            </Card>
          </Stack>
        )}

        {!!error && (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            Failed to load identity provider. Please try again.
          </Alert>
        )}

        {!isLoading && !error && !instance && (
          <Alert severity="warning" icon={<AlertTriangle size={18} />}>
            Identity provider for environment &quot;{envName}&quot; was not found.
          </Alert>
        )}

        {instance && !error && (
          <Card
            variant="outlined"
            sx={{ display: "flex", overflow: "hidden", minHeight: "calc(70vh - 64px)" }}
          >
            <Box component="nav" sx={{ width: 200, flexShrink: 0, overflowY: "auto", p: 2 }}>
              <List>
                <NavItem tab={overviewTab} isActive={isOverviewActive} />
              </List>
              <List
                subheader={
                  <ListSubheader disableGutters disableSticky>
                    <Typography variant="overline">Identity Management</Typography>
                  </ListSubheader>
                }
              >
                {identityManagementTabs.map((tab) => (
                  <NavItem
                    key={tab.label}
                    tab={tab}
                    isActive={!!tab.wildPath && !!matchPath(tab.wildPath, pathname)}
                  />
                ))}
              </List>
            </Box>
            <Divider orientation="vertical" flexItem />
            <Box sx={{ flex: 1, minWidth: 0, overflowY: "auto", p: 3 }}>
              <Routes>
                <Route
                  index
                  element={<ThunderInstanceOverviewTab instance={instance} onCopy={handleCopy} />}
                />
                <Route path="agents" element={<AgentsTab />} />
                <Route path="groups" element={<GroupsPage />} />
                <Route path="groups/create" element={<GroupCreatePage />} />
                <Route path="groups/:groupId" element={<GroupEditPage />} />
                <Route path="roles" element={<RolesPage />} />
                <Route path="roles/create" element={<RoleCreatePage />} />
                <Route path="roles/:roleId" element={<RoleEditPage />} />
                <Route path="*" element={<Navigate to="." replace />} />
              </Routes>
            </Box>
          </Card>
        )}
      </PageLayout>
    </>
  );
};

export default ViewThunderInstance;
