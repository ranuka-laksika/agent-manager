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

import { Box, Chip, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";

interface LabelChipsProps {
  labels?: Record<string, string>;
  /** Show at most this many chips; the rest collapse into a "+N" chip whose
   * tooltip lists them (default 3). */
  maxVisible?: number;
  size?: "small" | "medium";
}

/** Renders a labels map as compact, read-only `key: value` chips, capping how
 * many show inline so a resource with many labels doesn't blow out the
 * layout — the rest are listed in a tooltip on the "+N" chip. */
export const LabelChips = ({
  labels,
  maxVisible = 3,
  size = "small",
}: LabelChipsProps) => {
  const entries = Object.entries(labels ?? {});
  if (entries.length === 0) return null;

  const visible = entries.slice(0, maxVisible);
  const hidden = entries.slice(maxVisible);

  return (
    <Box display="flex" flexWrap="wrap" gap={0.5} alignItems="center">
      {visible.map(([key, value]) => (
        <Chip
          key={key}
          label={value ? `${key}: ${value}` : key}
          size={size}
          variant="outlined"
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
          <Chip label={`+${hidden.length}`} size={size} variant="outlined" />
        </Tooltip>
      )}
    </Box>
  );
};
