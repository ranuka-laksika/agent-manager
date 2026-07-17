/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useEffect, useState } from "react";
import { Alert, Box, Button, CircularProgress, Divider, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { AlertTriangle, RotateCcwKey } from "@wso2/oxygen-ui-icons-react";
import {
  useProvisionAgentIdentity,
  useRegenerateAgentIdentitySecret,
} from "@agent-management-platform/api-client";
import { TextInput } from "@agent-management-platform/views";
import { RolesGroupsChips, useAgentRolesAndGroups } from "./EnvAgentRolesGroupsSection";
import { useAgentIdentityBinding } from "./useAgentIdentityBinding";

interface EnvAgentIdentitySectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

// Client IDs/secrets are opaque tokens — monospace makes them easier to
// visually scan and copy correctly.
const monospaceInputSx = { "& .MuiInputBase-input": { fontFamily: "monospace" } };

/**
 * Shared "which environment is regenerating" tracking for the
 * Internal/ExternalAgentOverview pages — each renders one
 * RegenerateAgentIdentityButton per environment, all backed by the same
 * mutation, so the in-flight environment has to be tracked separately from
 * the mutation's own isPending (which is shared across every button).
 * onSuccess lets external agents capture+display the new secret; internal
 * agents omit it since the value is already injected into the workload.
 */
export function useRegenerateAgentIdentity(
  orgId: string | undefined,
  projectId: string | undefined,
  agentId: string | undefined,
) {
  const [regeneratingEnv, setRegeneratingEnv] = useState<string | null>(null);
  const { mutateAsync: regenerateSecret } = useRegenerateAgentIdentitySecret();

  const regenerate = async (
    envName: string,
    onSuccess?: (secret: { clientId: string; clientSecret: string }) => void,
  ) => {
    if (!orgId || !projectId || !agentId) return;
    setRegeneratingEnv(envName);
    try {
      const resp = await regenerateSecret({
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: { environment: envName },
      });
      onSuccess?.({ clientId: resp.clientId, clientSecret: resp.clientSecret });
    } catch {
      // Error already surfaced via useRegenerateAgentIdentitySecret's snackbar.
    } finally {
      setRegeneratingEnv((current) => (current === envName ? null : current));
    }
  };

  return { regeneratingEnv, regenerate };
}

/**
 * Warning alert revealing a just-issued client ID/secret pair, with copy
 * buttons. Shared by the external-agent claim flow (EnvAgentIdentitySection
 * below) and the internal-agent regenerate flow (InternalAgentOverview).
 */
export const SecretRevealAlert: React.FC<{
  clientId: string;
  clientSecret: string;
  message: string;
}> = ({ clientId, clientSecret, message }) => (
  <Alert severity="warning" icon={<AlertTriangle size={18} />} sx={{ width: "100%" }}>
    <Typography variant="body2" fontWeight={600}>
      {message}
    </Typography>
    <Stack spacing={1} mt={1} width="100%">
      <TextInput
        slotProps={{ input: { readOnly: true } }}
        label="Client ID"
        value={clientId}
        copyable
        fullWidth
        sx={monospaceInputSx}
      />
      <TextInput
        slotProps={{ input: { readOnly: true } }}
        label="Client Secret"
        value={clientSecret}
        copyable
        fullWidth
        sx={monospaceInputSx}
      />
    </Stack>
  </Alert>
);

/**
 * Header-level "Regenerate ID" button for an EnvironmentCard's `actions`
 * slot. Rotating is only meaningful once a binding has actually completed
 * provisioning, so this hides itself until then — the mutation call and the
 * resulting secret (if the caller chooses to display one) stay owned by the
 * page, since what happens with the new value differs between internal
 * (never shown — already injected into the workload) and external (shown,
 * since there's no other way for the operator to get it).
 */
export const RegenerateAgentIdentityButton: React.FC<
  EnvAgentIdentitySectionProps & {
    isRegenerating: boolean;
    onRegenerate: () => void;
    label?: string;
  }
> = ({ orgId, projectId, agentId, envId, isRegenerating, onRegenerate, label = "Regenerate ID" }) => {
  const { provisioned } = useAgentIdentityBinding({ orgId, projectId, agentId, envId });

  if (!provisioned) return null;

  return (
    <Button
      variant="text"
      size="small"
      startIcon={<RotateCcwKey size={16} />}
      onClick={onRegenerate}
      disabled={isRegenerating}
    >
      {isRegenerating ? "Generating..." : label}
    </Button>
  );
};

/**
 * Client ID (or claim/pending state) on the left, Roles & Groups chips on
 * the right. Shared by both branches below that have a completed binding to
 * show — only the left-hand content differs between them.
 */
const IdentityRow: React.FC<{
  left: React.ReactNode;
  rolesAndGroups: React.ComponentProps<typeof RolesGroupsChips>;
}> = ({ left, rolesAndGroups }) => (
  <Stack direction={{ xs: "column", md: "row" }} spacing={3}>
    <Box flex={1}>{left}</Box>
    <Box flex={1}>
      <RolesGroupsChips {...rolesAndGroups} />
    </Box>
  </Stack>
);

/**
 * Per-environment "Agent Identity" section, rendered above the other
 * sections inside an EnvironmentCard for externally hosted agents. Shows the
 * AgentID's client ID once provisioning completes — the client secret itself
 * is never stored, so getting one is always the header's "Generate Secret"
 * action (see RegenerateAgentIdentityButton), not anything this section does.
 */
export const EnvAgentIdentitySection: React.FC<EnvAgentIdentitySectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const { binding, provisioned, isLoading } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });

  const { mutate: provisionIdentity } = useProvisionAgentIdentity();
  const [hasRequestedProvision, setHasRequestedProvision] = useState(false);

  // Automatically provision AgentID for this environment if no binding
  // exists yet (e.g. environment added to project pipeline after agent creation).
  useEffect(() => {
    if (isLoading || binding || hasRequestedProvision) return;
    setHasRequestedProvision(true);
    provisionIdentity({
      params: { orgName: orgId, projName: projectId, agentName: agentId },
      query: { environment: envId },
    });
  }, [
    isLoading, binding, hasRequestedProvision, provisionIdentity, orgId, projectId, agentId, envId,
  ]);

  const rolesAndGroups = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  if (isLoading) {
    return <Skeleton variant="rounded" height={56} sx={{ mt: 2 }} />;
  }

  // Nothing to show for a platform-hosted agent — its credentials are
  // injected straight into its pod, never surfaced through this UI.
  if (binding && binding.provisioningType !== "external") {
    return null;
  }

  return (
    <>
      <Divider sx={{ mb: 1 }} />
      <Typography
        variant="caption"
        color="text.secondary"
        fontWeight={600}
        sx={{ textTransform: "uppercase", letterSpacing: "0.05em" }}
      >
        Agent Identity
      </Typography>

      <Box sx={{ mt: 1 }}>
        {!binding ? (
          <Stack direction="row" alignItems="center" gap={1}>
            <CircularProgress size={14} />
            <Typography variant="body2" color="text.secondary">
              Provisioning identity for this environment…
            </Typography>
          </Stack>
        ) : binding.status === "failed" ? (
          <Typography variant="body2" color="text.secondary">
            Provisioning failed — check the identity settings for details.
          </Typography>
        ) : provisioned ? (
          // The client ID itself isn't sensitive, unlike the secret, so it's
          // always safe to show once provisioning has completed.
          binding.clientId && (
            <IdentityRow
              rolesAndGroups={rolesAndGroups}
              left={
                <TextInput
                  slotProps={{ input: { readOnly: true } }}
                  label="Client ID"
                  value={binding.clientId}
                  copyable
                  fullWidth
                  sx={monospaceInputSx}
                />
              }
            />
          )
        ) : (
          // Polled by useGetAgentIdentity while status is pending/in_progress,
          // so this updates on its own once provisioning settles.
          <Stack direction="row" alignItems="center" gap={1}>
            <CircularProgress size={14} />
            <Typography variant="body2" color="text.secondary">
              Provisioning in progress…
            </Typography>
          </Stack>
        )}
      </Box>
    </>
  );
};
