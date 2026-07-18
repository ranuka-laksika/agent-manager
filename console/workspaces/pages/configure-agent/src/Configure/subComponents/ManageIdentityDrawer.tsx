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

import { useId, useState } from "react";
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
  Eye,
  ExternalLink,
  Fingerprint,
  RotateCcwKey,
  ShieldAlert,
  ShieldCheck,
  ShieldOff,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath } from "react-router-dom";
import {
  useAgentIdentityBinding,
  useClaimAgentIdentitySecret,
  useListThunderInstances,
  useRegenerateAgentIdentitySecret,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  TextInput,
} from "@agent-management-platform/views";
import {
  RolesGroupsChips,
  useAgentRolesAndGroups,
} from "@agent-management-platform/shared-component";

// Client IDs/secrets are opaque tokens — monospace makes them easier to
// visually scan and copy correctly.
const monospaceInputSx = { "& .MuiInputBase-input": { fontFamily: "monospace" } };

type Secret = { clientId: string; clientSecret: string; isRegenerated: boolean };

interface AgentIdentitySectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Client ID/secret claim + regenerate UI for one environment. Internal agents
 * never get a claim flow (the secret is injected straight into the workload,
 * never surfaced here) so they only see the regenerate action; external
 * agents additionally get to reveal/claim and see the client ID/secret.
 */
const AgentIdentitySection: React.FC<AgentIdentitySectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const { binding, provisioned, isLoading } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });

  const { mutateAsync: claimSecret, isPending: isClaiming } = useClaimAgentIdentitySecret();
  const { mutateAsync: regenerateSecret, isPending: isRegenerating } =
    useRegenerateAgentIdentitySecret();
  const [revealed, setRevealed] = useState<Secret | null>(null);

  const { data: thunderInstancesData, isLoading: isLoadingThunderInstance } =
    useListThunderInstances({ orgName: orgId });
  const thunderInstance = thunderInstancesData?.thunderInstances.find(
    (instance) => instance.envName === envId,
  );

  const { roles, groups, isLoading: isLoadingRolesAndGroups } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  const handleClaim = async () => {
    try {
      const resp = await claimSecret({
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        query: { environment: envId },
      });
      setRevealed({
        clientId: resp.clientId, clientSecret: resp.clientSecret, isRegenerated: false,
      });
    } catch {
      // Error already surfaced via useClaimAgentIdentitySecret's snackbar.
    }
  };

  const handleRegenerate = async () => {
    try {
      const resp = await regenerateSecret({
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: { environment: envId },
      });
      setRevealed({
        clientId: resp.clientId, clientSecret: resp.clientSecret, isRegenerated: true,
      });
    } catch {
      // Error already surfaced via useRegenerateAgentIdentitySecret's snackbar.
    }
  };

  if (isLoading) {
    return <Skeleton variant="rounded" height={120} />;
  }

  if (!binding) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<ShieldOff size={56} />}
          title="No agent identity"
          description="This environment doesn't have an agent identity yet."
          minHeight={160}
        />
      </ListingTable.Container>
    );
  }

  const isExternal = binding.provisioningType === "external";
  const alreadyClaimed = provisioned && !binding.hasUnclaimedSecret;

  const idpHref = generatePath(
    absoluteRouteMap.children.org.children.thunderInstances.children.view.path,
    { orgId, envName: envId },
  );

  let body: React.ReactNode;
  if (revealed) {
    // Only shown once, right after claiming/regenerating — the backend
    // deletes its stored copy the moment it's returned, so this is the only
    // chance to see the secret.
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
          {revealed.isRegenerated
            ? "This is the new secret after regenerating — copy it now, it won't be shown again."
            : "This secret will not be shown again — copy it now."}
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
    // reveal/regenerate.
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
        {alreadyClaimed && (
          <TextInput
            slotProps={{ input: { readOnly: true } }}
            label="Client Secret"
            value="••••••••"
            fullWidth
            size="small"
            sx={monospaceInputSx}
          />
        )}
        <Typography variant="body2" color="text.secondary">
          {binding.hasUnclaimedSecret
            ? "This agent's identity secret hasn't been claimed yet. Reveal it to view the value."
            : "This secret was already claimed and can't be shown again — regenerate to get a new one."}
        </Typography>
      </Stack>
    );
  } else if (alreadyClaimed) {
    // binding.clientId is guaranteed falsy here — the "binding.clientId"
    // branch above already claimed the truthy case.
    body = (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<Fingerprint size={56} />}
          title="No credentials to show"
          description="No identity credentials to show for this environment."
          minHeight={160}
        />
      </ListingTable.Container>
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
          <Stack direction="row" spacing={1}>
            {!revealed && binding.hasUnclaimedSecret && (
              <Button
                variant="text"
                size="small"
                onClick={() => void handleClaim()}
                disabled={isClaiming}
                startIcon={isClaiming ? <CircularProgress size={16} /> : <Eye size={16} />}
              >
                {isClaiming ? "Claiming..." : "Reveal Secret"}
              </Button>
            )}
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
          </Stack>
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

        {isLoadingThunderInstance ? (
          <Skeleton variant="rounded" height={120} />
        ) : !thunderInstance ? (
          <ListingTable.Container>
            <ListingTable.EmptyState
              illustration={<ShieldAlert size={56} />}
              title="No identity provider"
              description="No identity provider found for this environment."
              minHeight={160}
            />
          </ListingTable.Container>
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
  open, onClose, orgId, projectId, agentId, envNames, getEnvDisplayName,
  selectedEnvName, onSelectedEnvNameChange,
}) => {
  const envSelectLabelId = useId();
  const envName = envNames.includes(selectedEnvName) ? selectedEnvName : (envNames[0] ?? "");

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={640}>
      <DrawerHeader icon={<ShieldCheck size={24} />} title="Manage AgentID" onClose={onClose} />
      <DrawerContent>
        <Stack spacing={2}>
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

          {envName && (
            <AgentIdentitySection
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              envId={envName}
            />
          )}
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
};
