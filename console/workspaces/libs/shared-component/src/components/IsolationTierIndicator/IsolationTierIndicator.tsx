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

import type { ComponentType } from "react";
import { Box, Chip, Tooltip } from "@wso2/oxygen-ui";
import { Shield, ShieldCheck, ShieldPlus } from "@wso2/oxygen-ui-icons-react";

export type IsolationTierValue = "runc" | "gvisor" | "kata";

export interface IsolationTierMeta {
  value: IsolationTierValue;
  /** Sandboxing tier rank: 1 (runc) → 3 (kata). */
  tier: 1 | 2 | 3;
  /** Human name of the underlying runtime, e.g. "gVisor". */
  runtimeLabel: string;
  /** Compact label for chips/columns, e.g. "Sandboxing T2". */
  shortLabel: string;
  /** Full label for tooltips, e.g. "Sandboxing Tier 2 — gVisor". */
  fullLabel: string;
  color: "default" | "info" | "success";
  /** Theme color path for tinting the bare shield icon, e.g. "info.main". */
  iconColor: string;
  icon: ComponentType<{ size?: string | number }>;
}

const TIER_META: Record<IsolationTierValue, IsolationTierMeta> = {
  runc: {
    value: "runc",
    tier: 1,
    runtimeLabel: "runc (default)",
    shortLabel: "Sandboxing T1",
    fullLabel: "Sandboxing Tier 1 — runc (default)",
    color: "default",
    iconColor: "text.secondary",
    icon: Shield,
  },
  gvisor: {
    value: "gvisor",
    tier: 2,
    runtimeLabel: "gVisor",
    shortLabel: "Sandboxing T2",
    fullLabel: "Sandboxing Tier 2 — gVisor",
    color: "info",
    iconColor: "info.main",
    icon: ShieldPlus,
  },
  kata: {
    value: "kata",
    tier: 3,
    runtimeLabel: "Kata Containers",
    shortLabel: "Sandboxing T3",
    fullLabel: "Sandboxing Tier 3 — Kata Containers",
    color: "success",
    iconColor: "success.main",
    icon: ShieldCheck,
  },
};

/** Unrecognized/absent tiers resolve to runc, the platform default. */
export function getIsolationTierMeta(tier?: string): IsolationTierMeta {
  return TIER_META[tier as IsolationTierValue] ?? TIER_META.runc;
}

interface IsolationTierBadgeProps {
  tier?: string;
  size?: number;
}

/**
 * Bare shield icon with a "Sandboxing Tier N — <runtime>" tooltip.
 * Used next to environment names where a chip would be too heavy.
 */
export function IsolationTierBadge({ tier, size = 18 }: IsolationTierBadgeProps) {
  const meta = getIsolationTierMeta(tier);
  const IconComponent = meta.icon;
  return (
    <Tooltip title={meta.fullLabel}>
      <Box display="inline-flex" alignItems="center" sx={{ color: meta.iconColor }}>
        <IconComponent size={size} />
      </Box>
    </Tooltip>
  );
}

interface IsolationTierChipProps {
  tier?: string;
}

/**
 * "Sandboxing TN" chip with the tier's shield icon; the tooltip names the
 * concrete runtime (runc / gVisor / Kata Containers).
 */
export function IsolationTierChip({ tier }: IsolationTierChipProps) {
  const meta = getIsolationTierMeta(tier);
  const IconComponent = meta.icon;
  return (
    <Tooltip title={meta.runtimeLabel}>
      <Chip
        icon={<IconComponent size={14} />}
        label={meta.shortLabel}
        size="small"
        variant="outlined"
        color={meta.color}
        sx={{ cursor: "default" }}
      />
    </Tooltip>
  );
}
