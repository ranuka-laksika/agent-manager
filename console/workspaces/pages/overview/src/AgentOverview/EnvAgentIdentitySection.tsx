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

import { useState } from "react";
import { Box, Button, CircularProgress, Divider, Skeleton, Typography } from "@wso2/oxygen-ui";
import { Eye, ExternalLink, RotateCcwKey } from "@wso2/oxygen-ui-icons-react";
import { generatePath } from "react-router-dom";
import {
  useClaimAgentIdentitySecret,
  useRegenerateAgentIdentitySecret,
} from "@agent-management-platform/api-client";
import { CodeBlock } from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useAgentIdentityBinding } from "./useAgentIdentityBinding";

interface EnvAgentIdentitySectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

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
 * Header-level "Regenerate ID" button for an EnvironmentCard's `actions`
 * slot. Rotating is only meaningful once a binding has actually completed
 * provisioning, so this hides itself until then — the mutation call and the
 * resulting secret (if the caller chooses to display one) stay owned by the
 * page, since what happens with the new value differs between internal
 * (never shown — already injected into the workload) and external (shown,
 * since there's no other way for the operator to get it).
 */
export const RegenerateAgentIdentityButton: React.FC<
  EnvAgentIdentitySectionProps & { isRegenerating: boolean; onRegenerate: () => void }
> = ({ orgId, projectId, agentId, envId, isRegenerating, onRegenerate }) => {
  const { provisioned } = useAgentIdentityBinding({ orgId, projectId, agentId, envId });

  if (!provisioned) return null;

  return (
    <Button
      variant="text"
      size="small"
      startIcon={isRegenerating ? <CircularProgress size={16} /> : <RotateCcwKey size={16} />}
      onClick={onRegenerate}
      disabled={isRegenerating}
    >
      {isRegenerating ? "Regenerating..." : "Regenerate ID"}
    </Button>
  );
};

/**
 * Client ID/secret claim + regenerate UI, rendered as its own section below
 * the instrumentation guide in the "Setup Agent" drawer (InstrumentationDrawer)
 * for externally hosted agents — roles/groups stay on the EnvironmentCard
 * itself via EnvAgentRolesGroupsSection. Lets the user claim the AgentID's
 * one-time client secret — the backend deletes its stored copy the moment
 * it's returned, so this is the only chance to see it. Also links out to the
 * environment's Thunder instance, since that's where the client ID actually
 * gets registered with the IDP (token/JWKS endpoints live there too).
 */
export const EnvAgentIdentitySection: React.FC<EnvAgentIdentitySectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const { binding, provisioned, isLoading } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });

  const { mutateAsync: claimSecret, isPending: isClaiming } = useClaimAgentIdentitySecret();
  const { regeneratingEnv, regenerate } = useRegenerateAgentIdentity(orgId, projectId, agentId);

  // Claiming and regenerating both end up showing the same "here's the secret,
  // it won't be shown again" reveal — one slot (with a flag for which message
  // applies) instead of two, since only the most recent reveal this session is
  // ever displayed.
  const [revealed, setRevealed] = useState<
    { clientId: string; clientSecret: string; isRegenerated: boolean } | null
  >(null);

  const alreadyClaimed = provisioned && !binding?.hasUnclaimedSecret;
  const hasNoIdentityToShow = alreadyClaimed && !binding?.clientId;

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

  const handleRegenerate = () =>
    regenerate(envId, (secret) => setRevealed({ ...secret, isRegenerated: true }));

  if (isLoading) {
    return <Skeleton variant="rounded" height={56} />;
  }

  // Nothing to show for a binding that isn't provisioned yet, or a
  // platform-hosted agent (which has no claim flow — see GetAgentCredentials).
  if (!binding || binding.provisioningType !== "external") {
    return null;
  }

  // Formatted as one code block (two lines) rather than two separate
  // fields — matches how the "Set environment variables" SetupStep displays
  // a client ID/secret pair as a single copyable snippet. Always rendered
  // (with placeholders standing in for values we don't have yet/anymore),
  // the same way TokenGenerationStep always shows its CodeBlock with an
  // "ey***" placeholder before a real token exists — one consistent shape
  // instead of the code block appearing/disappearing across states.
  const displayClientId = revealed?.clientId ?? binding.clientId ?? "pending...";
  const displaySecret = revealed?.clientSecret ?? "••••••••";
  const code = `AGENT_CLIENT_ID="${displayClientId}"\nAGENT_CLIENT_SECRET="${displaySecret}"`;

  let description: string;
  if (revealed?.isRegenerated) {
    description = "This is the new secret after regenerating — copy it now, it won't be shown again.";
  } else if (revealed) {
    description = "This secret will not be shown again — copy it now.";
  } else if (binding.status === "failed") {
    description = "Provisioning failed — check the identity settings for details.";
  } else if (binding.hasUnclaimedSecret) {
    description = "This agent's identity secret hasn't been claimed yet. Reveal it to view the value.";
  } else if (hasNoIdentityToShow) {
    description = "No identity credentials to show for this environment.";
  } else if (alreadyClaimed) {
    description = "This secret was already claimed and can't be shown again — regenerate to get a new one.";
  } else {
    description = "Provisioning in progress…";
  }

  const idpHref = generatePath(
    absoluteRouteMap.children.org.children.thunderInstances.children.view.path,
    { orgId, envName: envId },
  );

  return (
    <>
      <Divider sx={{ my: 2 }} />

      <Box display="flex" gap={1} flexDirection="column">
        <Box display="flex" alignItems="center" justifyContent="space-between">
          <Typography variant="h5">Agent Identity Credentials</Typography>

          <Box display="flex" gap={1} alignItems="center">
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
            {(alreadyClaimed || binding.hasUnclaimedSecret) && (
              <RegenerateAgentIdentityButton
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                isRegenerating={regeneratingEnv === envId}
                onRegenerate={() => void handleRegenerate()}
              />
            )}
          </Box>
        </Box>

        <Box display="flex" flexDirection="column" gap={1}>
          <CodeBlock code={code} fieldId="agent-identity-secret" />

          <Typography variant="body2" color="textSecondary">
            {description}
          </Typography>
        </Box>

        <Box display="flex" flexDirection="column" gap={0.5} sx={{ mt: 1 }}>
          <Typography variant="body2" color="textSecondary">
            Register this client ID with your agent&apos;s Identity Provider to enable
            authentication. Visit the IDP page for the token and JWKS endpoint details.
          </Typography>
          <Button
            variant="text"
            size="small"
            component="a"
            href={idpHref}
            target="_blank"
            rel="noopener noreferrer"
            startIcon={<ExternalLink size={16} />}
            sx={{ alignSelf: "flex-start" }}
          >
            Configure Identity Provider
          </Button>
        </Box>
      </Box>
    </>
  );
};
