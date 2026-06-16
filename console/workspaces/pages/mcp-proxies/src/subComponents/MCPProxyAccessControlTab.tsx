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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AccessControlPanel,
  type AccessControlItem,
  type AccessControlMode,
  type AccessControlStatus,
} from "@agent-management-platform/shared-component";
import { useMCPPoliciesCatalog } from "@agent-management-platform/api-client";
import type {
  MCPProxy,
  MCPProxyPolicy,
} from "@agent-management-platform/types";
import { Alert, Stack, Typography } from "@wso2/oxygen-ui";
import { ACL_POLICY_NAME } from "../constants";

type CapabilityKind = "tool" | "resource" | "prompt";

const KIND_CHIP_LABEL: Record<CapabilityKind, string> = {
  tool: "Tool",
  resource: "Resource",
  prompt: "Prompt",
};

function getCapabilityIdentifier(
  kind: CapabilityKind,
  raw: Record<string, unknown> | undefined,
): string | null {
  if (!raw) return null;
  const value = kind === "resource" ? raw.uri ?? raw.name : raw.name ?? raw.uri;
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length ? trimmed : null;
}

function getCapabilityDescription(
  raw: Record<string, unknown> | undefined,
): string | undefined {
  if (!raw) return undefined;
  const value = raw.description ?? raw.title;
  return typeof value === "string" && value.trim() ? value : undefined;
}

function makeItemKey(kind: CapabilityKind, identifier: string): string {
  return `${kind}::${identifier}`;
}

type ParsedAcl = {
  mode: AccessControlMode;
  exceptionKeys: string[];
};

function parseExistingAclPolicy(
  policy: MCPProxyPolicy | undefined,
): ParsedAcl {
  if (!policy?.params) {
    return { mode: "allow", exceptionKeys: [] };
  }
  const params = policy.params as Record<string, unknown>;
  const sections: CapabilityKind[] = ["tool", "resource", "prompt"];
  const sectionKey: Record<CapabilityKind, string> = {
    tool: "tools",
    resource: "resources",
    prompt: "prompts",
  };
  let resolvedMode: AccessControlMode | null = null;
  const exceptionKeys: string[] = [];
  for (const kind of sections) {
    const section = params[sectionKey[kind]] as
      | Record<string, unknown>
      | undefined;
    if (!section) continue;
    const rawMode = String(section.mode ?? "").toLowerCase();
    if (resolvedMode === null && (rawMode === "allow" || rawMode === "deny")) {
      resolvedMode = rawMode;
    }
    const exceptions = section.exceptions;
    if (Array.isArray(exceptions)) {
      for (const entry of exceptions) {
        if (typeof entry === "string" && entry.trim()) {
          exceptionKeys.push(makeItemKey(kind, entry.trim()));
        }
      }
    }
  }
  return {
    mode: resolvedMode ?? "allow",
    exceptionKeys,
  };
}

function buildAclPolicyParams(
  mode: AccessControlMode,
  exceptionKeys: string[],
  hasCapabilities: Record<CapabilityKind, boolean>,
): Record<string, unknown> {
  const sectionKey: Record<CapabilityKind, string> = {
    tool: "tools",
    resource: "resources",
    prompt: "prompts",
  };
  const exceptionsByKind: Record<CapabilityKind, string[]> = {
    tool: [],
    resource: [],
    prompt: [],
  };
  for (const key of exceptionKeys) {
    const separator = key.indexOf("::");
    if (separator < 0) continue;
    const kind = key.slice(0, separator) as CapabilityKind;
    const identifier = key.slice(separator + 2);
    if (!identifier) continue;
    if (kind in exceptionsByKind) {
      exceptionsByKind[kind].push(identifier);
    }
  }
  const params: Record<string, unknown> = {};
  (["tool", "resource", "prompt"] as CapabilityKind[]).forEach((kind) => {
    if (!hasCapabilities[kind]) return;
    params[sectionKey[kind]] = {
      mode,
      exceptions: exceptionsByKind[kind],
    };
  });
  return params;
}

export type MCPProxyAccessControlTabProps = {
  proxy: MCPProxy | null | undefined;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPProxy>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxyAccessControlTab({
  proxy,
  orgName,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxyAccessControlTabProps) {
  const lastSavedRef = useRef<{
    mode: AccessControlMode;
    exceptionKeys: string[];
  } | null>(null);

  const [mode, setMode] = useState<AccessControlMode>("allow");
  const [exceptionKeys, setExceptionKeys] = useState<string[]>([]);
  const [status, setStatus] = useState<AccessControlStatus | null>(null);

  const { data: catalog, isLoading: isCatalogLoading } = useMCPPoliciesCatalog(
    orgName,
  );
  const availableAclPolicy = useMemo(
    () => catalog?.data?.find((p) => p.name === ACL_POLICY_NAME),
    [catalog],
  );

  const items = useMemo<AccessControlItem[]>(() => {
    const capabilities = proxy?.capabilities;
    if (!capabilities) return [];
    const result: AccessControlItem[] = [];
    const kinds: Array<{ kind: CapabilityKind; entries?: Record<string, unknown>[] }> = [
      { kind: "tool", entries: capabilities.tools },
      { kind: "resource", entries: capabilities.resources },
      { kind: "prompt", entries: capabilities.prompts },
    ];
    for (const { kind, entries } of kinds) {
      (entries ?? []).forEach((raw) => {
        const identifier = getCapabilityIdentifier(kind, raw);
        if (!identifier) return;
        result.push({
          key: makeItemKey(kind, identifier),
          method: KIND_CHIP_LABEL[kind],
          path: identifier,
          summary: getCapabilityDescription(raw),
        });
      });
    }
    return result;
  }, [proxy?.capabilities]);

  const capabilitiesPresent = useMemo<Record<CapabilityKind, boolean>>(
    () => ({
      tool: Boolean(proxy?.capabilities?.tools?.length),
      resource: Boolean(proxy?.capabilities?.resources?.length),
      prompt: Boolean(proxy?.capabilities?.prompts?.length),
    }),
    [
      proxy?.capabilities?.tools,
      proxy?.capabilities?.resources,
      proxy?.capabilities?.prompts,
    ],
  );

  const hasAnyCapability =
    capabilitiesPresent.tool ||
    capabilitiesPresent.resource ||
    capabilitiesPresent.prompt;

  useEffect(() => {
    if (!proxy) return;
    const existing = proxy.policies?.find((p) => p.name === ACL_POLICY_NAME);
    const parsed = parseExistingAclPolicy(existing);
    setMode(parsed.mode);
    setExceptionKeys(parsed.exceptionKeys);
    lastSavedRef.current = parsed;
  }, [proxy]);

  const isDirty = useMemo(() => {
    const saved = lastSavedRef.current;
    if (!saved) return false;
    const currentKeys = [...exceptionKeys].sort().join(" ");
    const savedKeys = [...saved.exceptionKeys].sort().join(" ");
    return mode !== saved.mode || currentKeys !== savedKeys;
  }, [mode, exceptionKeys]);

  const handleSave = useCallback(async () => {
    if (!proxy) return;
    if (!availableAclPolicy) {
      setStatus({
        message:
          "Access control policy is not available on the active gateway.",
        severity: "error",
      });
      return;
    }
    if (!hasAnyCapability) {
      setStatus({
        message:
          "This proxy has no tools, resources, or prompts to apply access control to.",
        severity: "error",
      });
      return;
    }
    const params = buildAclPolicyParams(
      mode,
      exceptionKeys,
      capabilitiesPresent,
    );
    const newPolicy: MCPProxyPolicy = {
      name: ACL_POLICY_NAME,
      version: availableAclPolicy.version,
      displayName: availableAclPolicy.displayName,
      params,
    };
    const existingPolicies = proxy.policies ?? [];
    const existingIndex = existingPolicies.findIndex(
      (p) => p.name === ACL_POLICY_NAME,
    );
    const nextPolicies =
      existingIndex >= 0
        ? existingPolicies.map((p, i) => (i === existingIndex ? newPolicy : p))
        : [...existingPolicies, newPolicy];

    try {
      await onUpdate({ policies: nextPolicies });
      lastSavedRef.current = { mode, exceptionKeys: [...exceptionKeys] };
      setStatus({
        message: "Access control updated successfully.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update access control.",
        severity: "error",
      });
    }
  }, [
    proxy,
    availableAclPolicy,
    hasAnyCapability,
    mode,
    exceptionKeys,
    capabilitiesPresent,
    onUpdate,
  ]);

  const handleDiscard = useCallback(() => {
    const saved = lastSavedRef.current;
    if (saved) {
      setMode(saved.mode);
      setExceptionKeys([...saved.exceptionKeys]);
    }
    setStatus(null);
  }, []);

  if (!isLoading && proxy && !hasAnyCapability) {
    return (
      <Stack
        alignItems="center"
        justifyContent="center"
        spacing={1}
        sx={{ minHeight: 200, textAlign: "center" }}
      >
        <Typography variant="subtitle1" fontWeight={600}>
          No Capabilities Available
        </Typography>
        <Typography variant="body2" color="text.secondary">
          This MCP proxy has no tools, resources, or prompts. Access control
          rules require at least one capability.
        </Typography>
      </Stack>
    );
  }

  return (
    <Stack spacing={2}>
      {!isCatalogLoading && !availableAclPolicy && (
        <Alert severity="warning">
          The access control policy ({ACL_POLICY_NAME}) is not reported as
          available by the active gateway. Saving is disabled.
        </Alert>
      )}
      <AccessControlPanel
        items={items}
        mode={mode}
        onModeChange={setMode}
        exceptionKeys={exceptionKeys}
        onExceptionKeysChange={setExceptionKeys}
        isLoading={isLoading || isCatalogLoading}
        isSaving={isUpdating}
        isDirty={isDirty && Boolean(availableAclPolicy)}
        onSave={() => void handleSave()}
        onDiscard={handleDiscard}
        status={status}
        onClearStatus={() => setStatus(null)}
        availableEmptyTitle="No available capabilities"
        availableEmptyDescription="Tools, resources, and prompts will appear here once the proxy reports them."
      />
    </Stack>
  );
}
