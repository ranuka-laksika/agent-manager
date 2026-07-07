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

import type {
  MCPEnvironmentConfig,
  MCPProxyCapabilities,
  MCPProxyPolicy,
  MCPServerInfoFetchResponse,
  UpstreamAuth,
} from "@agent-management-platform/types";
import type { EndpointDraft } from "./AddEndpointDialog";
import { ACL_POLICY_NAME, REWRITE_POLICY_NAME } from "../constants";

type CapabilityKind = "tool" | "resource" | "prompt";

// Default security applied to freshly configured environment blocks — mirrors the
// blueprint created by the MCP proxy creation form.
export const DEFAULT_ENDPOINT_SECURITY = {
  enabled: true,
  apiKey: {
    enabled: true,
    key: "X-API-Key",
    in: "header" as const,
  },
};

// Backend identifier of a capability entry, matching the resolution used by the
// Rewrite / Access Control tabs (resources key on uri, tools/prompts on name).
export function getCapabilityId(
  kind: CapabilityKind,
  raw: Record<string, unknown> | undefined,
): string | null {
  if (!raw) return null;
  const value =
    kind === "resource" ? (raw.uri ?? raw.name) : (raw.name ?? raw.uri);
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length ? trimmed : null;
}

function collectCapabilityIds(
  kind: CapabilityKind,
  entries: Record<string, unknown>[] | undefined,
): Set<string> {
  const ids = new Set<string>();
  (entries ?? []).forEach((raw) => {
    const id = getCapabilityId(kind, raw);
    if (id) ids.add(id);
  });
  return ids;
}

// The backend identifier a Rewrite policy entry targets (its explicit `target`,
// falling back to the client-facing name/uri).
function rewriteEntryId(
  kind: CapabilityKind,
  entry: Record<string, unknown>,
): string | null {
  const target =
    typeof entry.target === "string" && entry.target.trim()
      ? entry.target.trim()
      : null;
  if (target) return target;
  return getCapabilityId(kind, entry);
}

// Converts stored per-environment capabilities into the shape returned by the
// fetch-server-info API, so an already-configured endpoint can be rendered and
// edited with the same components as a freshly fetched one.
export function capabilitiesToFetchedInfo(
  capabilities: MCPProxyCapabilities | undefined,
): MCPServerInfoFetchResponse {
  return {
    tools: capabilities?.tools ?? [],
    resources: capabilities?.resources ?? [],
    prompts: capabilities?.prompts ?? [],
  };
}

/**
 * Groups the per-environment blueprint blocks back into logical endpoints. Blocks
 * sharing the same upstream URL and auth header describe a single endpoint serving
 * several environments — the inverse of how the creation form expands one endpoint
 * into one block per environment. Auth values are never returned by the API, so the
 * reconstructed draft carries an empty `authValue` (treated as "keep existing").
 */
export function reconstructEndpointsFromEnvironments(
  environments: Record<string, MCPEnvironmentConfig>,
): EndpointDraft[] {
  const groups = new Map<string, EndpointDraft>();
  let nextId = 1;

  for (const [envId, config] of Object.entries(environments ?? {})) {
    const url = config.upstream?.url ?? "";
    const authHeader = config.upstream?.auth?.header ?? "";
    const key = `${url}\0${authHeader}`;

    const existing = groups.get(key);
    if (existing) {
      existing.environments.push(envId);
      continue;
    }

    groups.set(key, {
      id: String(nextId++),
      url,
      authHeader,
      authValue: "",
      environments: [envId],
      fetchedInfo: capabilitiesToFetchedInfo(config.capabilities),
    });
  }

  return Array.from(groups.values());
}

function buildUpstreamAuth(endpoint: EndpointDraft): UpstreamAuth | undefined {
  const header = endpoint.authHeader.trim();
  if (!header) return undefined;
  const value = endpoint.authValue.trim();
  // Omit the value when the user left the masked credential untouched, mirroring
  // the Connection tab: the backend keeps the stored value when none is supplied.
  return value
    ? { type: "api-key", header, value }
    : { type: "api-key", header };
}

/**
 * Removes Rewrite and Access Control policy entries that reference tools, resources
 * or prompts absent from `capabilities`. Other policy entries and policies are left
 * untouched. Returns `undefined` when no policies remain.
 */
export function prunePoliciesForCapabilities(
  policies: MCPProxyPolicy[] | undefined,
  capabilities: MCPProxyCapabilities | undefined,
): MCPProxyPolicy[] | undefined {
  if (!policies || policies.length === 0) return policies;

  const validIds: Record<CapabilityKind, Set<string>> = {
    tool: collectCapabilityIds("tool", capabilities?.tools),
    resource: collectCapabilityIds("resource", capabilities?.resources),
    prompt: collectCapabilityIds("prompt", capabilities?.prompts),
  };
  const sectionKey: Record<CapabilityKind, string> = {
    tool: "tools",
    resource: "resources",
    prompt: "prompts",
  };
  const kinds: CapabilityKind[] = ["tool", "resource", "prompt"];

  const next: MCPProxyPolicy[] = [];
  for (const policy of policies) {
    if (policy.name === REWRITE_POLICY_NAME) {
      const pruned = pruneRewritePolicy(policy, validIds, sectionKey, kinds);
      if (pruned) next.push(pruned);
    } else if (policy.name === ACL_POLICY_NAME) {
      const pruned = pruneAclPolicy(policy, validIds, sectionKey, kinds);
      if (pruned) next.push(pruned);
    } else {
      next.push(policy);
    }
  }

  return next.length > 0 ? next : undefined;
}

function pruneRewritePolicy(
  policy: MCPProxyPolicy,
  validIds: Record<CapabilityKind, Set<string>>,
  sectionKey: Record<CapabilityKind, string>,
  kinds: CapabilityKind[],
): MCPProxyPolicy | null {
  const params = (policy.params ?? {}) as Record<string, unknown>;
  const nextParams: Record<string, unknown> = { ...params };
  let hasAny = false;

  for (const kind of kinds) {
    const entries = params[sectionKey[kind]];
    if (!Array.isArray(entries)) continue;
    const kept = (entries as Record<string, unknown>[]).filter((entry) => {
      const id = rewriteEntryId(kind, entry);
      return id != null && validIds[kind].has(id);
    });
    if (kept.length > 0) {
      nextParams[sectionKey[kind]] = kept;
      hasAny = true;
    } else {
      delete nextParams[sectionKey[kind]];
    }
  }

  if (!hasAny) return null;
  return { ...policy, params: nextParams };
}

function pruneAclPolicy(
  policy: MCPProxyPolicy,
  validIds: Record<CapabilityKind, Set<string>>,
  sectionKey: Record<CapabilityKind, string>,
  kinds: CapabilityKind[],
): MCPProxyPolicy | null {
  const params = (policy.params ?? {}) as Record<string, unknown>;
  const nextParams: Record<string, unknown> = { ...params };
  let hasAny = false;

  for (const kind of kinds) {
    const section = params[sectionKey[kind]] as
      | Record<string, unknown>
      | undefined;
    if (!section) continue;
    // Drop the whole section when the new capabilities have nothing of this kind.
    if (validIds[kind].size === 0) {
      delete nextParams[sectionKey[kind]];
      continue;
    }
    const exceptions = Array.isArray(section.exceptions)
      ? (section.exceptions as unknown[]).filter(
          (entry): entry is string =>
            typeof entry === "string" && validIds[kind].has(entry.trim()),
        )
      : [];
    nextParams[sectionKey[kind]] = { ...section, exceptions };
    hasAny = true;
  }

  if (!hasAny) return null;
  return { ...policy, params: nextParams };
}

/**
 * Rebuilds the proxy's per-environment blueprint map from the edited endpoint list.
 * For an environment that already had a block, everything is preserved except the
 * upstream URL/auth and the capabilities (which follow the endpoint's fetched info),
 * with Rewrite/ACL entries pruned to the surviving capabilities. Environments no
 * longer served by any endpoint are dropped, leaving them unconfigured.
 */
export function buildEnvironmentsMap(
  endpoints: EndpointDraft[],
  existing: Record<string, MCPEnvironmentConfig>,
): Record<string, MCPEnvironmentConfig> {
  const result: Record<string, MCPEnvironmentConfig> = {};

  for (const endpoint of endpoints) {
    const auth = buildUpstreamAuth(endpoint);
    const capabilities: MCPProxyCapabilities = {
      tools: endpoint.fetchedInfo.tools,
      resources: endpoint.fetchedInfo.resources,
      prompts: endpoint.fetchedInfo.prompts,
    };

    for (const envId of endpoint.environments) {
      const prev = existing[envId];
      if (prev) {
        result[envId] = {
          ...prev,
          upstream: { ...prev.upstream, url: endpoint.url, auth },
          capabilities,
          policies: prunePoliciesForCapabilities(prev.policies, capabilities),
        };
      } else {
        result[envId] = {
          upstream: { url: endpoint.url, auth },
          capabilities,
          security: DEFAULT_ENDPOINT_SECURITY,
        };
      }
    }
  }

  return result;
}
