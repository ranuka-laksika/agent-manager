/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { Cloud, RefreshCw, Server } from "@wso2/oxygen-ui-icons-react";
import { useSearchParams } from "react-router-dom";
import {
  getConfigureGatewayDisplayCommand,
  getGatewayEnvFile,
  getK8sGatewayHelmCommand,
  useCopyOnSuccess,
} from "@agent-management-platform/shared-component";
import { CommandField } from "./CommandField";

const TAB_PARAM = "reconfig-tab";
const VALID_TABS = [0, 1] as const;

interface GatewayReconfigureCardProps {
  registrationToken: string | null;
  hasJustRegeneratedToken: boolean;
  onReconfigure: () => void;
  isReconfiguring: boolean;
  onCopy: (text: string, label: string) => void;
}

export function GatewayReconfigureCard({
  registrationToken,
  hasJustRegeneratedToken,
  onReconfigure,
  isReconfiguring,
  onCopy,
}: GatewayReconfigureCardProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabFromUrl = searchParams.get(TAB_PARAM);
  const parsedTab = tabFromUrl != null ? parseInt(tabFromUrl, 10) : 0;
  const tabIndex = VALID_TABS.includes(parsedTab as (typeof VALID_TABS)[number]) ? parsedTab : 0;

  const handleTabChange = useCallback(
    (_: React.SyntheticEvent, newValue: number) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set(TAB_PARAM, String(newValue));
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const handleCopy = useCopyOnSuccess(onCopy);

  if (!registrationToken) {
    return (
      <Card variant="outlined">
        <CardContent sx={{ p: 3 }}>
          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 2 }}>
            <Typography variant="h6">Reconfigure Gateway</Typography>
          </Stack>
          <Stack spacing={1.5}>
            <Typography variant="body2">
              Generate a new registration token to reconnect or update your gateway configuration.
              This will revoke the existing token.
            </Typography>
            <Box>
              <Button
                variant="outlined"
                startIcon={<RefreshCw size={16} />}
                onClick={onReconfigure}
                disabled={isReconfiguring}
                color="error"
              >
                {isReconfiguring ? "Generating..." : "Reconfigure"}
              </Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>
    );
  }

  const vmCmd = getConfigureGatewayDisplayCommand(registrationToken);
  const k8sCmd = getK8sGatewayHelmCommand(registrationToken, "upgrade");

  const vmContent = (
    <Stack spacing={2}>
      {hasJustRegeneratedToken && (
        <Alert severity="success">
          New token generated. Update your gateway configuration using the command below,
          then restart the gateway.
        </Alert>
      )}
      <Typography variant="body2" color="text.secondary">
        Run this command to update {getGatewayEnvFile()} with the new token.
      </Typography>
      <CommandField
        value={vmCmd}
        multiline
        minRows={4}
        onCopy={() => handleCopy(vmCmd, "Configure command")}
        copyLabel="Configure command"
      />
    </Stack>
  );

  const k8sContent = (
    <Stack spacing={2}>
      {hasJustRegeneratedToken && (
        <Alert severity="success">
          New token generated. Run the helm upgrade command below to reconfigure your gateway.
        </Alert>
      )}
      <CommandField
        value={k8sCmd}
        multiline
        minRows={4}
        onCopy={() => handleCopy(k8sCmd, "Helm upgrade command")}
        copyLabel="Helm upgrade command"
      />
    </Stack>
  );

  return (
    <Card variant="outlined">
      <CardContent sx={{ p: 3 }}>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 2 }}>
          <Typography variant="h6">Reconfigure Gateway</Typography>
        </Stack>
        <Box sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}>
          <Tabs value={tabIndex} onChange={handleTabChange}>
            <Tab label="VM / Docker" icon={<Server size={18} />} iconPosition="start" />
            <Tab label="Kubernetes" icon={<Cloud size={18} />} iconPosition="start" />
          </Tabs>
        </Box>
        <Box hidden={tabIndex !== 0}>{tabIndex === 0 ? vmContent : null}</Box>
        <Box hidden={tabIndex !== 1}>{tabIndex === 1 ? k8sContent : null}</Box>
      </CardContent>
    </Card>
  );
}
