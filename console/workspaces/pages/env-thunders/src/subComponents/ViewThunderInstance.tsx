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
import React, { useCallback, useState } from "react";
import {
  Alert,
  Box,
  Card,
  Divider,
  Grid,
  Skeleton,
  Stack,
  Tab,
  Tabs,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Folder, Shield, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";
import { ThunderInstanceComingSoonTab } from "./ThunderInstanceComingSoonTab";
import { ThunderInstanceOverviewTab } from "./ThunderInstanceOverviewTab";

const TABS = ["Overview", "Users", "Roles", "Groups"] as const;

export const ViewThunderInstance: React.FC = () => {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const [tabIndex, setTabIndex] = useState(0);
  const { pushSnackBar } = useSnackBar();

  const { data, isLoading, error } = useListThunderInstances({ orgName: orgId });
  const instance = data?.thunderInstances.find((i) => i.envName === envName);

  const handleCopy = useCallback((value: string, label: string) => {
    navigator.clipboard.writeText(value).then(() => {
      pushSnackBar({ message: `${label} copied to clipboard`, type: "success" });
    }).catch(() => {});
  }, [pushSnackBar]);

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
        backLabel="Back to Identity"
        disableIcon
        isLoading={isLoading}
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
          <Card variant="outlined">
            <Tabs value={tabIndex} onChange={(_, value: number) => setTabIndex(value)}>
              {TABS.map((tab) => (
                <Tab key={tab} label={tab} />
              ))}
            </Tabs>
            <Divider />
            <Box sx={{ p: 3 }}>
              {tabIndex === 0 && (
                <ThunderInstanceOverviewTab instance={instance} onCopy={handleCopy} />
              )}
              {tabIndex === 1 && (
                <ThunderInstanceComingSoonTab illustration={<Users size={48} />} title="Users" />
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
