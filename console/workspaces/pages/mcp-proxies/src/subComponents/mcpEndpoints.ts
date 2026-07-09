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
  MCPProxyCapabilities,
  MCPProxyEndpoint,
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
 * Converts a native backend endpoint into the form's editable draft (1:1). The draft's
 * client `id` carries the endpoint handle so the save path can preserve the endpoint's
 * identity. Auth values are never returned by the API, so the draft carries an empty
 * `authValue` (treated as "keep existing"). Its environment list is the flat set of bound
 * environment UUIDs.
 */
export function endpointToDraft(endpoint: MCPProxyEndpoint): EndpointDraft {
  return {
    id: endpoint.id,
    name: endpoint.name ?? "",
    url: endpoint.upstream?.main?.url ?? "",
    authHeader: endpoint.upstream?.main?.auth?.header ?? "",
    authValue: "",
    environments: (endpoint.environments ?? []).map(
      (env) => env.environmentUuid,
    ),
    fetchedInfo: capabilitiesToFetchedInfo(endpoint.capabilities),
  };
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
 * Derives an endpoint handle (id) from its name, falling back to the URL host and finally
 * a positional `endpoint-N`. The result is slugified to the `[a-z0-9-]` handle charset.
 */
export function deriveEndpointHandle(
  draft: Pick<EndpointDraft, "name" | "url">,
  index: number,
): string {
  const fromName = slugify(draft.name ?? "");
  if (fromName) return fromName;
  const fromUrl = slugify(hostFromUrl(draft.url));
  if (fromUrl) return fromUrl;
  return `endpoint-${index + 1}`;
}

/**
 * Converts an edited draft into a native backend endpoint (1:1). When an `existing`
 * endpoint is supplied (edit path), its policies, security and tool-scope bindings are
 * preserved — policies pruned to the endpoint's surviving capabilities — and its handle is
 * reused. Capabilities follow the draft's fetched info. The endpoint's environment
 * bindings are the draft's flat env-UUID list; `deploymentStatus` is response-only and
 * never sent.
 */
export function draftToEndpoint(
  draft: EndpointDraft,
  index: number,
  existing?: MCPProxyEndpoint,
): MCPProxyEndpoint {
  const auth = buildUpstreamAuth(draft);
  const capabilities: MCPProxyCapabilities = {
    tools: draft.fetchedInfo.tools,
    resources: draft.fetchedInfo.resources,
    prompts: draft.fetchedInfo.prompts,
  };
  const name = (draft.name ?? "").trim();

  return {
    id: existing?.id ?? deriveEndpointHandle(draft, index),
    name: name || undefined,
    upstream: { ...existing?.upstream, main: { url: draft.url, auth } },
    capabilities,
    policies: prunePoliciesForCapabilities(existing?.policies, capabilities),
    security: existing?.security ?? DEFAULT_ENDPOINT_SECURITY,
    toolScopeBindings: existing?.toolScopeBindings,
    environments: draft.environments.map((environmentUuid) => ({
      environmentUuid,
    })),
  };
}

function slugify(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function hostFromUrl(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "";
  }
}
