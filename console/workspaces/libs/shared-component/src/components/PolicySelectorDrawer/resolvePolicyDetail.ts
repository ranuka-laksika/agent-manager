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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { GuardrailDefinition } from "@agent-management-platform/api-client";
import {
  parsePolicyYaml,
  normalizeRootSchema,
  type PolicyDefinition,
} from "../../utils/policyParameterEditor";

/**
 * True for gateway-manifest-sourced policies (see useLLMPoliciesCatalog), which already
 * carry their parameter schema inline. Hub-sourced policies (MCP, and any other future
 * non-gateway source) never set `parameters`, so this is naturally always false there.
 */
export function hasInlinePolicyDefinition(policy: GuardrailDefinition): boolean {
  return policy.parameters !== undefined;
}

export type ResolvedPolicyDetail =
  | { policyDefinition: PolicyDefinition; parseError: null }
  | { policyDefinition: null; parseError: string | null };

/**
 * Derives the policy's configuration schema either from its inline `parameters` (no
 * network round-trip) or by parsing a fetched YAML definition — mirroring whichever
 * source `useGuardrailPolicyDefinition` was actually asked to fetch.
 */
export function resolvePolicyDetail(
  policy: GuardrailDefinition,
  yamlText: string | undefined,
): ResolvedPolicyDetail {
  if (hasInlinePolicyDefinition(policy)) {
    return {
      policyDefinition: {
        name: policy.name,
        version: policy.version,
        description: policy.description ?? "",
        parameters: normalizeRootSchema(policy.parameters),
        systemParameters: normalizeRootSchema(policy.systemParameters),
      },
      parseError: null,
    };
  }
  if (!yamlText) return { policyDefinition: null, parseError: null };
  try {
    return { policyDefinition: parsePolicyYaml(yamlText), parseError: null };
  } catch {
    return {
      policyDefinition: null,
      parseError: "Failed to parse policy definition.",
    };
  }
}
