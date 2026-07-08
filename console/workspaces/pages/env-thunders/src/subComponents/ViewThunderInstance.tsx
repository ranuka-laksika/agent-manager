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
import React, { type ReactNode, useCallback, useState } from "react";
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
import { generatePath, useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";
import { ThunderInstanceComingSoonTab } from "./ThunderInstanceComingSoonTab";
import { ThunderInstanceOverviewTab } from "./ThunderInstanceOverviewTab";

interface NavTab {
  label: string;
  icon: ReactNode;
}

// Mirrors the left-nav layout used by the org Settings page
// (pages/settings/src/SettingsLayout.tsx) instead of a top-tab bar. Overview
// stays as a standalone item up top; Agents/Roles/Groups are grouped under a
// subheader since they're all agent-identity management concerns.
const OVERVIEW_TAB: NavTab = { label: "Overview", icon: <LayoutGrid size={18} /> };

const IDENTITY_MANAGEMENT_TABS: NavTab[] = [
  { label: "Agents", icon: <Users size={18} /> },
  { label: "Roles", icon: <Shield size={18} /> },
  { label: "Groups", icon: <Folder size={18} /> },
];

const ICON_SX_ACTIVE = { minWidth: 36, color: "primary.main" } as const;
const ICON_SX_INACTIVE = { minWidth: 36, color: "inherit" } as const;
const TEXT_SX_ACTIVE = { color: "text.secondary" } as const;
const TEXT_SX_INACTIVE = { color: "inherit" } as const;

interface NavItemProps {
  tab: NavTab;
  isActive: boolean;
  onClick: () => void;
}

function NavItem({ tab, isActive, onClick }: NavItemProps) {
  return (
    <ListItemButton selected={isActive} onClick={onClick}>
      <ListItemIcon sx={isActive ? ICON_SX_ACTIVE : ICON_SX_INACTIVE}>{tab.icon}</ListItemIcon>
      <ListItemText primary={tab.label} sx={isActive ? TEXT_SX_ACTIVE : TEXT_SX_INACTIVE} />
    </ListItemButton>
  );
}

export const ViewThunderInstance: React.FC = () => {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const [tabIndex, setTabIndex] = useState(0);
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
                <NavItem
                  tab={OVERVIEW_TAB}
                  isActive={tabIndex === 0}
                  onClick={() => setTabIndex(0)}
                />
              </List>
              <List
                subheader={
                  <ListSubheader disableGutters disableSticky>
                    <Typography variant="overline">Identity Management</Typography>
                  </ListSubheader>
                }
              >
                {IDENTITY_MANAGEMENT_TABS.map((tab, i) => {
                  const index = i + 1;
                  return (
                    <NavItem
                      key={tab.label}
                      tab={tab}
                      isActive={tabIndex === index}
                      onClick={() => setTabIndex(index)}
                    />
                  );
                })}
              </List>
            </Box>
            <Divider orientation="vertical" flexItem />
            <Box sx={{ flex: 1, minWidth: 0, overflowY: "auto", p: 3 }}>
              {tabIndex === 0 && (
                <ThunderInstanceOverviewTab instance={instance} onCopy={handleCopy} />
              )}
              {tabIndex === 1 && (
                <ThunderInstanceComingSoonTab illustration={<Users size={48} />} title="Agents" />
              )}
              {tabIndex === 2 && (
                <ThunderInstanceComingSoonTab illustration={<Shield size={48} />} title="Roles" />
              )}
              {tabIndex === 3 && (
                <ThunderInstanceComingSoonTab illustration={<Folder size={48} />} title="Groups" />
              )}
            </Box>
          </Card>
        )}
      </PageLayout>
    </>
  );
};

export default ViewThunderInstance;
