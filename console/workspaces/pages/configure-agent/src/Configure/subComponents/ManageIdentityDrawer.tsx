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

import { useId } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Form,
  FormControl,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  ExternalLink,
  RotateCcwKey,
  ShieldAlert,
  ShieldCheck,
  ShieldOff,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  TextInput,
} from "@agent-management-platform/views";
import {
  getErrorMessage,
  monospaceInputSx,
  RolesGroupsChips,
  useAgentIdentityCredentials,
  useAgentRolesAndGroups,
  useThunderInstanceForEnv,
} from "@agent-management-platform/shared-component";

/**
 * Shared loading/error/empty fallback for this drawer's several independent
 * queries (identity binding, Thunder instance, environment list) — each
 * needs the same three-way triage, just with different icons/copy.
 */
const QueryStateFallback: React.FC<{
  isLoading: boolean;
  isError: boolean;
  errorTitle: string;
  errorDescription: string;
  isEmptyValue: boolean;
  emptyIcon: React.ReactNode;
  emptyTitle: string;
  emptyDescription: string;
}> = ({
  isLoading, isError, errorTitle, errorDescription,
  isEmptyValue, emptyIcon, emptyTitle, emptyDescription,
}) => {
  if (isLoading) {
    return <Skeleton variant="rounded" height={120} />;
  }
  if (isError) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<AlertTriangle size={56} />}
          title={errorTitle}
          description={errorDescription}
          minHeight={160}
        />
      </ListingTable.Container>
    );
  }
  if (isEmptyValue) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={emptyIcon}
          title={emptyTitle}
          description={emptyDescription}
          minHeight={160}
        />
      </ListingTable.Container>
    );
  }
  return null;
};

interface AgentIdentitySectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Client ID/secret regenerate UI for one environment. The client secret is
 * never stored server-side, so the only way to see one is right after a
 * regenerate call — internal agents get it injected straight into the
 * workload instead, but the same regenerate action and client ID display
 * apply to both.
 */
const AgentIdentitySection: React.FC<AgentIdentitySectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const {
    binding, provisioned, isLoading, isError, error,
    revealed, isRegenerating, regenerate: handleRegenerate,
  } = useAgentIdentityCredentials({ orgId, projectId, agentId, envId });

  const {
    thunderInstance,
    isLoading: isLoadingThunderInstance,
    isError: isThunderInstanceError,
    error: thunderInstanceError,
  } = useThunderInstanceForEnv({ orgId, envId });

  const { roles, groups, isLoading: isLoadingRolesAndGroups } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  if (isLoading || isError || !binding) {
    return (
      <QueryStateFallback
        isLoading={isLoading}
        isError={isError}
        errorTitle="Failed to load agent identity"
        errorDescription={getErrorMessage(error)}
        isEmptyValue={!binding}
        emptyIcon={<ShieldOff size={56} />}
        emptyTitle="No agent identity"
        emptyDescription="This environment doesn't have an agent identity yet."
      />
    );
  }

  const isExternal = binding.provisioningType === "external";

  const idpHref = generatePath(
    absoluteRouteMap.children.org.children.thunderInstances.children.view.path,
    { orgId, envName: envId },
  );

  let body: React.ReactNode;
  if (revealed) {
    // Only shown once, right after regenerating — the backend never stores
    // the secret, so this is the only chance to see it.
    body = (
      <Stack spacing={1.5}>
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client ID"
          value={revealed.clientId}
          copyable
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client Secret"
          value={revealed.clientSecret}
          type="password"
          showPasswordToggle
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <Typography variant="body2" color="text.secondary">
          This secret will not be shown again — copy it now.
        </Typography>
      </Stack>
    );
  } else if (binding.status === "failed") {
    body = (
      <Typography variant="body2" color="text.secondary">
        Provisioning failed — check the identity settings for details.
      </Typography>
    );
  } else if (binding.clientId) {
    // The client ID itself isn't sensitive, unlike the secret, so it's always
    // safe to show. The secret field is a static placeholder here — it's not
    // a real masked value the user could reveal, just an indicator that a
    // secret exists; the real value only ever shows up above, right after a
    // regenerate.
    body = (
      <Stack spacing={1.5}>
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client ID"
          value={binding.clientId}
          copyable
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <TextInput
          slotProps={{ input: { readOnly: true } }}
          label="Client Secret"
          value="••••••••"
          fullWidth
          size="small"
          sx={monospaceInputSx}
        />
        <Typography variant="body2" color="text.secondary">
          This secret was already generated and can&apos;t be shown again — regenerate to get a
          new one.
        </Typography>
      </Stack>
    );
  } else {
    body = (
      <Typography variant="body2" color="text.secondary">
        Provisioning in progress…
      </Typography>
    );
  }

  return (
    <Stack spacing={3}>
      <Form.Section>
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Form.Subheader>Client Credentials</Form.Subheader>
          {provisioned && (
            <Button
              variant="text"
              size="small"
              onClick={() => void handleRegenerate()}
              disabled={isRegenerating}
              startIcon={
                isRegenerating ? <CircularProgress size={16} /> : <RotateCcwKey size={16} />
              }
            >
              {isRegenerating ? "Regenerating..." : "Regenerate Secret"}
            </Button>
          )}
        </Box>

        {body}

        {!isExternal && (
          <Typography variant="body2" color="text.secondary">
            This agent&apos;s client secret is injected directly into the workload —
            the values above are shown for reference, but you don&apos;t need to copy
            anything from here to configure the agent itself.
          </Typography>
        )}
      </Form.Section>

      <Form.Section>
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Form.Subheader>Roles &amp; Groups</Form.Subheader>
          <Button
            variant="text"
            size="small"
            component="a"
            href={idpHref}
            target="_blank"
            rel="noopener noreferrer"
            startIcon={<ExternalLink size={16} />}
          >
            Manage Permissions
          </Button>
        </Box>
        <RolesGroupsChips roles={roles} groups={groups} isLoading={isLoadingRolesAndGroups} />
      </Form.Section>

      <Form.Section>
        <Form.Subheader>OAuth2 Endpoints</Form.Subheader>

        {isLoadingThunderInstance || isThunderInstanceError || !thunderInstance ? (
          <QueryStateFallback
            isLoading={isLoadingThunderInstance}
            isError={isThunderInstanceError}
            errorTitle="Failed to load identity provider"
            errorDescription={getErrorMessage(thunderInstanceError)}
            isEmptyValue={!thunderInstance}
            emptyIcon={<ShieldAlert size={56} />}
            emptyTitle="No identity provider"
            emptyDescription="No identity provider found for this environment."
          />
        ) : (
          <Stack spacing={1.5}>
            <TextInput
              slotProps={{ input: { readOnly: true } }}
              label="Issuer URL"
              value={thunderInstance.issuerUrl}
              copyable
              fullWidth
              size="small"
              sx={monospaceInputSx}
            />
            <TextInput
              slotProps={{ input: { readOnly: true } }}
              label="Token Endpoint"
              value={thunderInstance.tokenUrl}
              copyable
              fullWidth
              size="small"
              sx={monospaceInputSx}
            />
            <TextInput
              slotProps={{ input: { readOnly: true } }}
              label="JWKS Endpoint"
              value={thunderInstance.jwksUrl}
              copyable
              fullWidth
              size="small"
              sx={monospaceInputSx}
            />
          </Stack>
        )}
      </Form.Section>

      {isExternal && thunderInstance && (
        <Alert severity="info">
          <Typography variant="body2">
            Configure your agent to request a JWT token from the identity provider
            endpoints above, using the client ID and secret shown earlier, so it can
            authenticate its requests.
          </Typography>
        </Alert>
      )}
    </Stack>
  );
};

export interface ManageIdentityDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
  projectId: string;
  agentId: string;
  envNames: string[];
  /** Loading/error state of the query that produced `envNames`, so the
   * drawer can tell "still fetching" or "failed to fetch" apart from a
   * genuinely empty environment list. */
  isEnvironmentsLoading: boolean;
  isEnvironmentsError: boolean;
  getEnvDisplayName: (name: string) => string;
  /** Controlled by the caller (URL search param) so a deep link from an
   * EnvironmentCard's "Manage AgentID" button can pre-select the environment
   * it was opened from. */
  selectedEnvName: string;
  onSelectedEnvNameChange: (envName: string) => void;
}

/**
 * Per-agent identity management, opened from the Configure Agent page's
 * "Manage AgentID" button — client ID/secret/regenerate, roles/groups, and
 * the OAuth2 endpoint details, one environment at a time via the selector
 * below. Overview cards keep their own roles/groups display too
 * (EnvAgentRolesGroupsSection); this is the fuller picture for one env.
 */
export const ManageIdentityDrawer: React.FC<ManageIdentityDrawerProps> = ({
  open, onClose, orgId, projectId, agentId, envNames,
  isEnvironmentsLoading, isEnvironmentsError, getEnvDisplayName,
  selectedEnvName, onSelectedEnvNameChange,
}) => {
  const envSelectLabelId = useId();
  const envName = envNames.includes(selectedEnvName) ? selectedEnvName : (envNames[0] ?? "");

  let content: React.ReactNode;
  if (isEnvironmentsLoading || isEnvironmentsError || !envName) {
    content = (
      <QueryStateFallback
        isLoading={isEnvironmentsLoading}
        isError={isEnvironmentsError}
        errorTitle="Failed to load environments"
        errorDescription="Something went wrong while loading this agent's environments. Please try again."
        isEmptyValue={!envName}
        emptyIcon={<ShieldOff size={56} />}
        emptyTitle="No environments"
        emptyDescription="This agent isn't deployed to any environment yet."
      />
    );
  } else {
    content = (
      <>
        <Stack direction="row" spacing={2} alignItems="center" justifyContent="flex-end">
          <Typography id={envSelectLabelId} variant="body2" color="text.secondary">
            Environment
          </Typography>
          <FormControl size="small" sx={{ minWidth: 260 }}>
            <Select
              labelId={envSelectLabelId}
              value={envName}
              onChange={(event) => onSelectedEnvNameChange(event.target.value as string)}
            >
              {envNames.map((name) => (
                <MenuItem key={name} value={name}>
                  {getEnvDisplayName(name)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Stack>

        <AgentIdentitySection
          key={envName}
          orgId={orgId}
          projectId={projectId}
          agentId={agentId}
          envId={envName}
        />
      </>
    );
  }

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={640}>
      <DrawerHeader icon={<ShieldCheck size={24} />} title="Manage AgentID" onClose={onClose} />
      <DrawerContent>
        <Stack spacing={2}>{content}</Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
};
