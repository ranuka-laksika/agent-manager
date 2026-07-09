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

import { useMemo } from "react";
import type { AgentIdentityAgentResponse } from "@agent-management-platform/types";

/**
 * Agents without a Thunder binding yet can't be added as a group member or
 * role assignee, and can't be looked up by Thunder agent ID, so both the
 * picker options and the lookup map are restricted to bound agents.
 */
export function useAgentLookup(agents: AgentIdentityAgentResponse[]) {
  const boundAgents = useMemo(() => agents.filter((a) => !!a.thunderAgentId), [agents]);
  const agentsByThunderId = useMemo(
    () => new Map(boundAgents.map((a) => [a.thunderAgentId as string, a])),
    [boundAgents],
  );
  const displayName = (thunderAgentId: string) =>
    agentsByThunderId.get(thunderAgentId)?.agentName ?? thunderAgentId;

  return { agents: boundAgents, agentsByThunderId, displayName };
}
