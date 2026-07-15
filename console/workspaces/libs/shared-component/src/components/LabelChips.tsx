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

import { useLayoutEffect, useRef, useState } from "react";
import { Box, Chip, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";

interface LabelChipsProps {
  labels?: Record<string, string>;
  size?: "small" | "medium";
}

const CHIP_GAP_PX = 4;
// Approximate width of a "+N" overflow chip (its digit count barely moves
// this), reserved so the chip we decide to show doesn't itself get pushed
// out of view by the overflow chip appearing after it.
const OVERFLOW_CHIP_WIDTH_PX = 46;

export const LabelChips = ({ labels, size = "small" }: LabelChipsProps) => {
  const entries = Object.entries(labels ?? {});
  const containerRef = useRef<HTMLDivElement | null>(null);
  const measureRefs = useRef<(HTMLDivElement | null)[]>([]);
  const [visibleCount, setVisibleCount] = useState(entries.length);

  useLayoutEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const recalculate = () => {
      const available = container.clientWidth;
      if (available <= 0) return;

      let used = 0;
      let count = 0;
      for (let i = 0; i < entries.length; i++) {
        const width = measureRefs.current[i]?.getBoundingClientRect().width ?? 0;
        const gap = count > 0 ? CHIP_GAP_PX : 0;
        const hasMoreAfterThis = entries.length - (i + 1) > 0;
        const overflowReserve = hasMoreAfterThis ? OVERFLOW_CHIP_WIDTH_PX + CHIP_GAP_PX : 0;
        // Always keep at least one chip visible (if there's room for
        // anything at all) so a single long label doesn't collapse the row
        // down to nothing but a "+N" chip.
        if (used + gap + width + overflowReserve > available && count > 0) break;
        used += gap + width;
        count += 1;
      }
      setVisibleCount(count || (entries.length > 0 ? 1 : 0));
    };

    recalculate();

    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(recalculate);
    observer.observe(container);
    return () => observer.disconnect();
    // Re-measure whenever the label set itself changes; `entries` is derived
    // fresh from `labels` every render, so depending on `labels` here keeps
    // this in sync without recreating the effect on every unrelated render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [labels]);

  if (entries.length === 0) return null;

  const visible = entries.slice(0, visibleCount);
  const hidden = entries.slice(visibleCount);

  return (
    <Box
      ref={containerRef}
      sx={{
        display: "flex",
        flexWrap: "nowrap",
        alignItems: "center",
        gap: `${CHIP_GAP_PX}px`,
        overflow: "hidden",
        minWidth: 0,
        flex: "1 1 0%",
      }}
    >
      {/*
        Off-screen measuring pass: every chip renders once, invisibly, purely
        so its natural width can be read — that's what decides how many fit
        on the visible line above.
      */}
      <Box
        aria-hidden
        sx={{
          position: "absolute",
          top: -9999,
          left: -9999,
          visibility: "hidden",
          display: "flex",
          gap: `${CHIP_GAP_PX}px`,
          pointerEvents: "none",
        }}
      >
        {entries.map(([key, value], i) => (
          <Box
            key={key}
            ref={(el: HTMLDivElement | null) => {
              measureRefs.current[i] = el;
            }}
          >
            <Chip label={value ? `${key}: ${value}` : key} size={size} variant="outlined" />
          </Box>
        ))}
      </Box>

      {visible.map(([key, value]) => (
        <Chip
          key={key}
          label={value ? `${key}: ${value}` : key}
          size={size}
          variant="outlined"
          sx={{ flexShrink: 0 }}
        />
      ))}
      {hidden.length > 0 && (
        <Tooltip
          title={
            <Stack spacing={0.25}>
              {hidden.map(([key, value]) => (
                <Typography key={key} variant="caption">
                  {value ? `${key}: ${value}` : key}
                </Typography>
              ))}
            </Stack>
          }
          placement="top"
        >
          <Chip label={`+${hidden.length}`} size={size} variant="outlined" sx={{ flexShrink: 0 }} />
        </Tooltip>
      )}
    </Box>
  );
};
