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

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ChangeEvent,
} from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useCreateMCPProxy,
  useFetchMCPProxyServerInfo,
  useListGateways,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MCPProxy,
  type MCPServerInfoFetchResponse,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Form,
  FormControl,
  FormLabel,
  IconButton,
  InputAdornment,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Eye,
  EyeOff,
  HelpCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useSnackBar } from "@agent-management-platform/views";
import {
  normalizeVersion,
  validateEndpointUrl,
} from "@agent-management-platform/shared-component";
import { MCPCapabilitiesView } from "../components/MCPCapabilitiesView";
import { MCP_SPEC_VERSION } from "../constants";

interface AddMCPProxyFormProps {
  onCancel: () => void;
}

type FormStep = "endpoint" | "details";

export function AddMCPProxyForm({ onCancel }: AddMCPProxyFormProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const fetchServerInfo = useFetchMCPProxyServerInfo();
  const createMCPProxy = useCreateMCPProxy();
  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId ?? "" },
    { limit: 500 },
  );
  const { pushSnackBar } = useSnackBar();
  const [step, setStep] = useState<FormStep>("endpoint");
  const [endpointUrl, setEndpointUrl] = useState("");
  const [authHeader, setAuthHeader] = useState("");
  const [authValue, setAuthValue] = useState("");
  const [showAuthValue, setShowAuthValue] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(true);
  const [fetchedInfo, setFetchedInfo] =
    useState<MCPServerInfoFetchResponse | null>(null);
  const [proxyName, setProxyName] = useState("");
  const [proxyVersion, setProxyVersion] = useState("");
  const [proxyDescription, setProxyDescription] = useState("");
  const [proxyContext, setProxyContext] = useState("");
  const [proxyTarget, setProxyTarget] = useState("");
  const [selectedGatewayIds, setSelectedGatewayIds] = useState<string[]>([]);
  const [urlError, setUrlError] = useState<string | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);

  const gateways = useMemo(
    () =>
      gatewaysData?.gateways.filter((gateway) => gateway.status === "ACTIVE") ??
      [],
    [gatewaysData?.gateways],
  );
  const trimmedUrl = endpointUrl.trim();
  const isFetched = Boolean(fetchedInfo);
  const isFetching = fetchServerInfo.isPending;
  const isCreating = createMCPProxy.isPending;
  const canFetch = Boolean(trimmedUrl) && !isFetching;

  useEffect(() => {
    setSelectedGatewayIds((current) => {
      const activeGatewayIds = new Set(gateways.map((gateway) => gateway.uuid));
      const retained = current.filter((gatewayId) =>
        activeGatewayIds.has(gatewayId),
      );
      if (retained.length > 0) return retained;
      return gateways[0]?.uuid ? [gateways[0].uuid] : [];
    });
  }, [gateways]);

  const resetFetchedInfo = useCallback(() => {
    setFetchedInfo(null);
    setStep("endpoint");
  }, []);

  const handleEndpointChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      setEndpointUrl(event.target.value);
      resetFetchedInfo();
      setUrlError(null);
    },
    [resetFetchedInfo],
  );

  const performFetch = useCallback(
    async (url: string) => {
      if (!orgId) {
        setUrlError("Organization is required to fetch MCP server info.");
        return;
      }

      const urlValidationError = validateEndpointUrl(url, {
        requiredMessage: "Enter a valid MCP Proxy endpoint URL.",
        invalidMessage: "Enter a valid MCP Proxy endpoint URL.",
        protocolMessage: "Enter a valid MCP Proxy endpoint URL.",
      });
      if (urlValidationError) {
        setUrlError(urlValidationError);
        return;
      }

      const header = authHeader.trim();
      const value = authValue.trim();
      const hasHeader = Boolean(header);
      const hasValue = Boolean(value);
      if (hasHeader !== hasValue) {
        setAuthError("Enter both an authentication header and value.");
        return;
      }
      setAuthError(null);

      const body =
        hasHeader && hasValue
          ? {
              url,
              auth: {
                type: "api-key" as const,
                header,
                value,
              },
            }
          : { url };

      try {
        const result = await fetchServerInfo.mutateAsync({
          params: { orgName: orgId },
          body,
        });
        setFetchedInfo(result);
        const nextName =
          getServerInfoValue(result.serverInfo, "name") || "mcp-proxy";
        const nextVersion =
          normalizeVersion(extractVersionFromUrl(url)) ||
          normalizeVersion(getServerInfoValue(result.serverInfo, "version")) ||
          "v1.0";
        setProxyName(nextName);
        setProxyVersion(nextVersion);
        setProxyDescription("");
        setProxyContext(`/default/${nextName}`);
        setProxyTarget(url);
      } catch (err: unknown) {
        resetFetchedInfo();
        if (
          typeof err === "object" &&
          err !== null &&
          (err as { code?: string }).code === "UNAUTHORIZED"
        ) {
          setAdvancedOpen(true);
          setAuthError(
            "This server requires authentication. Enter the credentials above.",
          );
        } else {
          const message =
            err instanceof Error
              ? err.message
              : "Failed to fetch MCP server info. Please check the URL and try again.";
          pushSnackBar({ message, type: "error" });
        }
      }
    },
    [
      authHeader,
      authValue,
      fetchServerInfo,
      orgId,
      pushSnackBar,
      resetFetchedInfo,
    ],
  );

  const handlePrimaryAction = useCallback(async () => {
    if (isFetched) {
      setStep("details");
      return;
    }
    await performFetch(trimmedUrl);
  }, [isFetched, performFetch, trimmedUrl]);

  const handleNameChange = useCallback(
    (value: string) => {
      const previousContext = proxyName ? `/default/${proxyName}` : "";
      setProxyName(value);
      if (!proxyContext || proxyContext === previousContext) {
        setProxyContext(value ? `/default/${value}` : "");
      }
    },
    [proxyContext, proxyName],
  );

  const handleCreate = useCallback(async () => {
    if (!orgId || !fetchedInfo) return;

    const header = authHeader.trim();
    const value = authValue.trim();
    const hasAuth = Boolean(header) && Boolean(value);
    const target = proxyTarget.trim();
    const name = proxyName.trim();
    const body: MCPProxy = {
      id: toHandle(name),
      name,
      version: proxyVersion.trim(),
      description: proxyDescription.trim() || undefined,
      context: proxyContext.trim() || undefined,
      mcpSpecVersion: MCP_SPEC_VERSION,
      gateways: selectedGatewayIds.length > 0 ? selectedGatewayIds : undefined,
      upstream: {
        main: {
          url: target,
          auth: hasAuth
            ? {
                type: "api-key",
                header,
                value,
              }
            : undefined,
        },
      },
      capabilities: {
        tools: fetchedInfo.tools,
        resources: fetchedInfo.resources,
        prompts: fetchedInfo.prompts,
      },
      security: {
        enabled: true,
        apiKey: {
          enabled: true,
          key: "X-API-Key",
          in: "header",
        },
      },
    };

    await createMCPProxy.mutateAsync({
      params: { orgName: orgId },
      body,
    });
    navigate(
      generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
        orgId,
      }),
    );
  }, [
    authHeader,
    authValue,
    createMCPProxy,
    fetchedInfo,
    navigate,
    orgId,
    proxyContext,
    proxyDescription,
    proxyName,
    proxyTarget,
    proxyVersion,
    selectedGatewayIds,
  ]);

  const preview = useMemo(() => {
    if (!fetchedInfo) return null;

    const serverName = getServerInfoValue(fetchedInfo.serverInfo, "name");
    const serverVersion = getServerInfoValue(fetchedInfo.serverInfo, "version");

    return (
      <Form.Section>
        <Stack spacing={2}>
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography variant="h6" fontWeight={600}>
              {serverName || "MCP Proxy"}
            </Typography>
            {serverVersion ? (
              <Chip
                label={
                  serverVersion.startsWith("v")
                    ? serverVersion
                    : `v${serverVersion}`
                }
                size="small"
                variant="outlined"
              />
            ) : null}
          </Stack>

          <MCPCapabilitiesView
            tools={fetchedInfo.tools}
            resources={fetchedInfo.resources}
            prompts={fetchedInfo.prompts}
          />
        </Stack>
      </Form.Section>
    );
  }, [fetchedInfo]);

  if (step === "details") {
    return (
      <Stack spacing={3} sx={{ maxWidth: 920 }}>
        <Form.Section>
          <Form.Stack spacing={2}>
            <Form.Stack
              direction={{ xs: "column", md: "row" }}
              spacing={2}
              useFlexGap
            >
              <FormControl sx={{ flex: 1 }}>
                <FormLabel required>Name</FormLabel>
                <TextField
                  fullWidth
                  value={proxyName}
                  onChange={(event) => handleNameChange(event.target.value)}
                />
              </FormControl>
              <FormControl sx={{ width: { xs: "100%", md: 300 } }}>
                <FormLabel required>Version</FormLabel>
                <TextField
                  fullWidth
                  value={proxyVersion}
                  onChange={(event) => setProxyVersion(event.target.value)}
                />
              </FormControl>
            </Form.Stack>

            <FormControl fullWidth>
              <FormLabel>Description</FormLabel>
              <TextField
                fullWidth
                multiline
                minRows={3}
                value={proxyDescription}
                onChange={(event) => setProxyDescription(event.target.value)}
                placeholder="Primary MCP Proxy"
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel>Context</FormLabel>
              <TextField
                fullWidth
                value={proxyContext}
                onChange={(event) => setProxyContext(event.target.value)}
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel required>Target</FormLabel>
              <TextField
                fullWidth
                value={proxyTarget}
                onChange={(event) => setProxyTarget(event.target.value)}
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel required>Gateway</FormLabel>
              {isLoadingGateways ? (
                <Skeleton variant="rounded" height={40} sx={{ mt: 0.5 }} />
              ) : (
                <Autocomplete
                  multiple
                  options={gateways}
                  size="small"
                  value={gateways.filter((gateway) =>
                    selectedGatewayIds.includes(gateway.uuid),
                  )}
                  onChange={(_, selectedGateways) => {
                    setSelectedGatewayIds(
                      selectedGateways.map((gateway) => gateway.uuid),
                    );
                  }}
                  getOptionLabel={(option) =>
                    option.displayName || option.name || option.uuid
                  }
                  renderInput={(params) => (
                    <TextField {...params} placeholder="Select gateway(s)" />
                  )}
                  sx={{ mt: 0.5 }}
                />
              )}
              {!isLoadingGateways && gateways.length === 0 ? (
                <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
                  No active gateways available.
                </Typography>
              ) : null}
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Stack direction="row" spacing={1}>
          <Button variant="outlined" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="contained"
            disabled={
              !proxyName.trim() ||
              !proxyVersion.trim() ||
              !proxyTarget.trim() ||
              selectedGatewayIds.length === 0 ||
              isLoadingGateways ||
              isCreating
            }
            onClick={handleCreate}
            startIcon={
              isCreating ? (
                <CircularProgress size={16} color="inherit" />
              ) : undefined
            }
          >
            {isCreating ? "Creating" : "Create"}
          </Button>
        </Stack>
      </Stack>
    );
  }

  return (
    <Stack spacing={3}>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            lg: isFetched
              ? "minmax(0, 1fr) minmax(0, 1fr)"
              : "minmax(0, 1fr)",
          },
          gap: 3,
          alignItems: "start",
        }}
      >
        <Stack spacing={3}>
          <Form.Section>
            <Form.Header>Endpoint Details</Form.Header>
            <Form.Stack spacing={2}>
              <FormControl fullWidth error={Boolean(urlError)}>
                <FormLabel required>MCP Proxy Endpoint URL</FormLabel>
                <TextField
                  fullWidth
                  value={endpointUrl}
                  onChange={handleEndpointChange}
                  placeholder="Enter URL of your MCP Proxy"
                  error={Boolean(urlError)}
                  helperText={urlError}
                />
              </FormControl>

              <Accordion
                expanded={advancedOpen}
                onChange={(_, expanded) => setAdvancedOpen(expanded)}
                disableGutters
                variant="outlined"
              >
                <AccordionSummary expandIcon={<ChevronDown size={18} />}>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Typography variant="subtitle1" fontWeight={600}>
                      Advanced Configurations
                    </Typography>
                    <Tooltip title="Configure optional request headers for the MCP proxy endpoint.">
                      <HelpCircle size={16} />
                    </Tooltip>
                  </Stack>
                </AccordionSummary>
                <AccordionDetails>
                  <Form.Stack spacing={2}>
                    <Typography variant="subtitle2" fontWeight={600}>
                      Configure Authentication Header
                    </Typography>
                    <Form.Stack
                      direction={{ xs: "column", md: "row" }}
                      spacing={2}
                      useFlexGap
                    >
                      <FormControl sx={{ flex: 1 }} error={Boolean(authError)}>
                        <FormLabel>Header</FormLabel>
                        <TextField
                          fullWidth
                          value={authHeader}
                          onChange={(event) => {
                            setAuthHeader(event.target.value);
                            resetFetchedInfo();
                            setAuthError(null);
                          }}
                          placeholder="Header"
                          error={Boolean(authError)}
                        />
                      </FormControl>
                      <FormControl sx={{ flex: 1 }} error={Boolean(authError)}>
                        <FormLabel>Value</FormLabel>
                        <TextField
                          fullWidth
                          value={authValue}
                          onChange={(event) => {
                            setAuthValue(event.target.value);
                            resetFetchedInfo();
                            setAuthError(null);
                          }}
                          placeholder="Value"
                          error={Boolean(authError)}
                          helperText={authError}
                          type={showAuthValue ? "text" : "password"}
                          slotProps={{
                            input: {
                              endAdornment: (
                                <InputAdornment position="end">
                                  <IconButton
                                    aria-label={
                                      showAuthValue
                                        ? "Hide header value"
                                        : "Show header value"
                                    }
                                    onClick={() =>
                                      setShowAuthValue((prev) => !prev)
                                    }
                                    edge="end"
                                  >
                                    {showAuthValue ? (
                                      <EyeOff size={18} />
                                    ) : (
                                      <Eye size={18} />
                                    )}
                                  </IconButton>
                                </InputAdornment>
                              ),
                            },
                          }}
                        />
                      </FormControl>
                    </Form.Stack>
                  </Form.Stack>
                </AccordionDetails>
              </Accordion>
            </Form.Stack>
          </Form.Section>

          <Stack direction="row" spacing={1}>
            <Button variant="outlined" onClick={onCancel}>
              Cancel
            </Button>
            <Button
              variant="contained"
              disabled={!canFetch && !isFetched}
              onClick={handlePrimaryAction}
              startIcon={
                isFetching ? (
                  <CircularProgress size={16} color="inherit" />
                ) : undefined
              }
            >
              {isFetched
                ? "Next"
                : isFetching
                  ? "Fetching"
                  : "Fetch Server Info"}
            </Button>
          </Stack>
        </Stack>

        <Collapse in={isFetched} timeout="auto" unmountOnExit>
          {preview}
        </Collapse>
      </Box>
    </Stack>
  );
}

function getServerInfoValue(
  serverInfo: Record<string, unknown> | undefined,
  key: string,
): string | undefined {
  const value = serverInfo?.[key];
  return typeof value === "string" ? value : undefined;
}

function extractVersionFromUrl(value: string): string | undefined {
  try {
    const segments = new URL(value).pathname.split("/").filter(Boolean);
    return [...segments]
      .reverse()
      .find((segment) => /^v?\d+(\.\d+)*$/i.test(segment));
  } catch {
    return undefined;
  }
}

function toHandle(value: string): string {
  const handle = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return handle || "mcp-proxy";
}
