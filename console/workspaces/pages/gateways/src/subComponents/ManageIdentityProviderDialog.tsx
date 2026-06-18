/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormLabel,
  IconButton,
  MenuItem,
  Select,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, Eye, EyeOff, KeyRound, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useListEnvironments,
  useListGateways,
} from "@agent-management-platform/api-client";
import {
  getGatewayVersion,
  getRawScriptUrl,
  LOCAL_GATEWAY_EXTENSION_CHART_REF,
} from "@agent-management-platform/shared-component";
import type { IdentityProvider } from "@agent-management-platform/types";

const SCRIPT_NAME = "manage-identity-provider.sh";
const TOKEN_MASK = "•••••••••••••••";

/**
 * Quickstart runs against a local k3d cluster from a repo clone (local helm
 * chart, no version pin); Kubernetes targets a real cluster with the OCI chart
 * pinned to the released gateway version.
 */
type ScriptTarget = "quickstart" | "kubernetes";

export type ManageIdentityProviderMode = "upsert" | "delete";

interface ManageIdentityProviderDialogProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
  mode: ManageIdentityProviderMode;
  /** For delete, the provider row to remove (name + gateway + environment prefilled). */
  provider?: IdentityProvider | null;
}

interface ScriptInputs {
  orgId: string;
  envName: string;
  gatewayId: string;
  name: string;
  issuer: string;
  jwksUri: string;
  skipTlsVerify: boolean;
  mode: ManageIdentityProviderMode;
  token: string;
}

// Env vars shared by both targets, minus the leading indentation.
function actionEnvLines(i: ScriptInputs): string[] {
  return i.mode === "delete"
    ? ["ACTION=delete \\"]
    : [
        `IDP_ISSUER=${i.issuer || "<issuer>"} \\`,
        `IDP_JWKS_URI=${i.jwksUri || "<jwks-uri>"} \\`,
        ...(i.skipTlsVerify ? ["IDP_SKIP_TLS_VERIFY=true \\"] : []),
      ];
}

function buildScript(i: ScriptInputs, target: ScriptTarget): string {
  if (target === "quickstart") {
    // Repo clone + local helm chart: run the local script, no CHART_VERSION.
    return [
      `ORG_NAME=${i.orgId || "<org>"} \\`,
      `ENV_NAME=${i.envName || "<env-name>"} \\`,
      `GATEWAY_ID=${i.gatewayId || "<gateway-id>"} \\`,
      `AGENT_MANAGER_TOKEN=${i.token} \\`,
      `CHART_REF=${LOCAL_GATEWAY_EXTENSION_CHART_REF} \\`,
      `IDP_NAME=${i.name || "<identity-provider-name>"} \\`,
      ...actionEnvLines(i),
      `bash deployments/scripts/${SCRIPT_NAME}`,
    ].join("\n");
  }

  // Kubernetes: curl the pinned-tag script, OCI chart at the released version.
  return [
    `curl -fsSL ${getRawScriptUrl(SCRIPT_NAME)} \\`,
    `  | ORG_NAME=${i.orgId || "<org>"} \\`,
    `    ENV_NAME=${i.envName || "<env-name>"} \\`,
    `    GATEWAY_ID=${i.gatewayId || "<gateway-id>"} \\`,
    `    AGENT_MANAGER_TOKEN=${i.token} \\`,
    `    CHART_VERSION=${getGatewayVersion()} \\`,
    `    IDP_NAME=${i.name || "<identity-provider-name>"} \\`,
    ...actionEnvLines(i).map((line) => `    ${line}`),
    "    bash",
  ].join("\n");
}

export function ManageIdentityProviderDialog({
  open,
  onClose,
  orgId,
  mode,
  provider,
}: ManageIdentityProviderDialogProps) {
  const isDelete = mode === "delete";
  const { getToken } = useAuthHooks();

  const [envName, setEnvName] = useState("");
  const [gatewayId, setGatewayId] = useState("");
  const [name, setName] = useState("");
  const [issuer, setIssuer] = useState("");
  const [jwksUri, setJwksUri] = useState("");
  const [skipTlsVerify, setSkipTlsVerify] = useState(false);

  const [tab, setTab] = useState(0);
  const target: ScriptTarget = tab === 0 ? "quickstart" : "kubernetes";

  const [showToken, setShowToken] = useState(false);
  const [resolvedToken, setResolvedToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { data: environments } = useListEnvironments({ orgName: orgId });
  // Gateways belonging to the selected environment populate the gateway picker.
  const { data: gatewaysResp } = useListGateways(
    { orgName: orgId },
    envName ? { environment: envName } : undefined,
  );
  const gateways = useMemo(() => gatewaysResp?.gateways ?? [], [gatewaysResp]);

  // Reset form when the dialog opens, seeding from the provider for delete.
  useEffect(() => {
    if (!open) return;
    setShowToken(false);
    setResolvedToken(null);
    setCopied(false);
    if (isDelete && provider) {
      setEnvName(provider.environmentName ?? "");
      setGatewayId(provider.gatewayId ?? "");
      setName(provider.name);
      setIssuer(provider.issuer ?? "");
      setJwksUri(provider.jwksUri ?? "");
      setSkipTlsVerify(provider.skipTlsVerify ?? false);
    } else {
      setEnvName("");
      setGatewayId("");
      setName("");
      setIssuer("");
      setJwksUri("");
      setSkipTlsVerify(false);
    }
  }, [open, isDelete, provider]);

  // Clear the gateway selection when the environment changes (upsert flow).
  useEffect(() => {
    if (isDelete) return;
    setGatewayId("");
  }, [envName, isDelete]);

  const handleToggleToken = useCallback(async () => {
    if (showToken) {
      setShowToken(false);
      setResolvedToken(null);
      return;
    }
    try {
      const token = await getToken();
      setResolvedToken(token);
      setShowToken(true);
    } catch {
      // silently fail
    }
  }, [showToken, getToken]);

  const scriptInputs = useCallback(
    (token: string): ScriptInputs => ({
      orgId,
      envName,
      gatewayId,
      name,
      issuer,
      jwksUri,
      skipTlsVerify,
      mode,
      token,
    }),
    [orgId, envName, gatewayId, name, issuer, jwksUri, skipTlsVerify, mode],
  );

  const handleCopy = useCallback(async () => {
    try {
      const token = resolvedToken ?? (await getToken());
      await navigator.clipboard.writeText(buildScript(scriptInputs(token), target));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  }, [resolvedToken, getToken, scriptInputs, target]);

  const displayScript = useMemo(
    () =>
      buildScript(
        scriptInputs(showToken && resolvedToken ? resolvedToken : TOKEN_MASK),
        target,
      ),
    [scriptInputs, showToken, resolvedToken, target],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={isDelete ? <Trash size={24} /> : <KeyRound size={24} />}
        title={isDelete ? "Remove Identity Provider" : "Add Identity Provider"}
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            Identity providers are owned by the gateway. This command patches the gateway
            configuration and syncs Agent Manager. Run it in a terminal with{" "}
            <code>kubectl</code>, <code>helm</code>, and <code>jq</code> configured against your
            cluster.
          </Typography>

          <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
            <Tabs value={tab} onChange={(_, v) => setTab(v as number)}>
              <Tab label="Quick Start" />
              <Tab label="Kubernetes" />
            </Tabs>
          </Box>

          <Stack spacing={2}>
            <FormControl fullWidth disabled={isDelete}>
              <FormLabel required>Environment</FormLabel>
              <Select
                size="small"
                value={envName}
                displayEmpty
                onChange={(e) => setEnvName(e.target.value as string)}
                renderValue={(v) => (v ? (v as string) : "Select an environment")}
              >
                {(environments ?? []).map((env) => (
                  <MenuItem key={env.name} value={env.name}>
                    {env.displayName || env.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl fullWidth disabled={isDelete || !envName}>
              <FormLabel required>Gateway</FormLabel>
              <Select
                size="small"
                value={gatewayId}
                displayEmpty
                onChange={(e) => setGatewayId(e.target.value as string)}
                renderValue={(v) => {
                  if (!v) return "Select a gateway";
                  const gw = gateways.find((g) => g.uuid === v);
                  return gw?.displayName || gw?.name || (v as string);
                }}
              >
                {gateways.map((gw) => (
                  <MenuItem key={gw.uuid} value={gw.uuid}>
                    {gw.displayName || gw.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControl fullWidth disabled={isDelete}>
              <FormLabel required>Name</FormLabel>
              <TextField
                size="small"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g., AcmeIdP"
                disabled={isDelete}
              />
            </FormControl>

            {!isDelete && (
              <>
                <FormControl fullWidth>
                  <FormLabel required>Issuer</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    value={issuer}
                    onChange={(e) => setIssuer(e.target.value)}
                    placeholder="https://idp.example.com"
                  />
                </FormControl>

                <FormControl fullWidth>
                  <FormLabel required>JWKS URI</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    value={jwksUri}
                    onChange={(e) => setJwksUri(e.target.value)}
                    placeholder="https://idp.example.com/oauth2/jwks"
                  />
                </FormControl>

                <FormControlLabel
                  control={
                    <Checkbox
                      checked={skipTlsVerify}
                      onChange={(e) => setSkipTlsVerify(e.target.checked)}
                    />
                  }
                  label="Skip TLS verification when fetching JWKS (trusted internal only)"
                />
              </>
            )}
          </Stack>

          <Stack spacing={1}>
            <Typography variant="body2">
              {target === "quickstart"
                ? "Run from the root of your repo clone:"
                : "Run in a terminal configured against your cluster:"}
            </Typography>
            <Box
              sx={{
                position: "relative",
                bgcolor: "grey.900",
                borderRadius: 1,
                p: 2,
                pr: 8,
                fontFamily: "monospace",
                fontSize: "0.8125rem",
                color: "grey.100",
                whiteSpace: "pre",
                overflowX: "auto",
              }}
            >
              <Box sx={{ position: "absolute", top: 6, right: 6, display: "flex", gap: 0.5 }}>
                <Tooltip title={showToken ? "Hide token" : "Show token"}>
                  <IconButton size="small" onClick={handleToggleToken} sx={{ color: "grey.400" }}>
                    {showToken ? <EyeOff size={16} /> : <Eye size={16} />}
                  </IconButton>
                </Tooltip>
                <Tooltip title={copied ? "Copied!" : "Copy"}>
                  <IconButton
                    size="small"
                    onClick={handleCopy}
                    sx={{ color: copied ? "success.light" : "grey.400" }}
                  >
                    <Copy size={16} />
                  </IconButton>
                </Tooltip>
              </Box>
              {displayScript}
            </Box>
            <Typography variant="caption" color="text.secondary">
              Your access token will be substituted when you copy.
            </Typography>
          </Stack>

          <Alert severity="info">
            Once the script completes, the list reflects the change. The script is idempotent —
            safe to re-run.
          </Alert>

          <Box display="flex" justifyContent="flex-end">
            <Button variant="outlined" color="inherit" onClick={onClose}>
              Close
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
