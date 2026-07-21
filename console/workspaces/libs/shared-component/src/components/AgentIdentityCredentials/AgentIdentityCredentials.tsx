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

import { useState } from "react";
import {
  useAgentIdentityBinding,
  useRegenerateAgentIdentitySecret,
} from "@agent-management-platform/api-client";

// Client IDs/secrets are opaque tokens — monospace makes them easier to
// visually scan and copy correctly. Shared by every place that displays one.
export const monospaceInputSx = { "& .MuiInputBase-input": { fontFamily: "monospace" } };

interface UseAgentIdentityCredentialsParams {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

export interface RevealedAgentIdentitySecret {
  clientId: string;
  clientSecret: string;
}

/**
 * Client ID/secret regenerate state for one agent+environment. The client
 * secret is never stored server-side, so the only way to see one is right
 * after a regenerate call — shared by the Configure Agent page's Manage
 * AgentID drawer and the MCP server's "Connect" panel, both of which surface
 * the same client-id-always-visible / secret-only-once-after-regenerate flow.
 */
export function useAgentIdentityCredentials({
  orgId, projectId, agentId, envId,
}: UseAgentIdentityCredentialsParams) {
  const { binding, provisioned, isLoading, isError, error } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });
  const { mutateAsync: regenerateSecret, isPending: isRegenerating } =
    useRegenerateAgentIdentitySecret();
  const [revealed, setRevealed] = useState<RevealedAgentIdentitySecret | null>(null);

  const regenerate = async () => {
    try {
      const resp = await regenerateSecret({
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: { environment: envId },
      });
      setRevealed({ clientId: resp.clientId, clientSecret: resp.clientSecret });
    } catch {
      // Error already surfaced via useRegenerateAgentIdentitySecret's snackbar.
    }
  };

  return {
    binding,
    provisioned,
    isLoading,
    isError,
    error,
    revealed,
    isRegenerating,
    regenerate,
  };
}
